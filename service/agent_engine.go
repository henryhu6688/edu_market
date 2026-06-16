package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
	"log"
	"log/slog"
)

// SSEHandler SSE 事件回调接口（由控制器实现）
type SSEHandler func(event string, data string) error

// agentChatMsg LLM 消息格式（OpenAI 兼容）
type agentChatMsg struct {
	Role             string         `json:"role"`
	Content          string         `json:"content,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ToolCalls        []toolCallItem `json:"tool_calls,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
}

type toolCallItem struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function toolCallFunc       `json:"function"`
}

type toolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// llmRequest LLM 请求体
type llmRequest struct {
	Model     string                   `json:"model"`
	Messages  []agentChatMsg           `json:"messages"`
	Stream    bool                     `json:"stream"`
	Tools     []map[string]interface{} `json:"tools,omitempty"`
	MaxTokens int                      `json:"max_tokens,omitempty"`
}

// llmChoice LLM 响应 choice（非流式）
type llmChoice struct {
	Message      agentChatMsg `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// llmUsage 用量
type llmUsage struct {
	TotalTokens int `json:"total_tokens"`
}

// llmResponse 非流式 LLM 响应
type llmResponse struct {
	Choices []llmChoice `json:"choices"`
	Usage   llmUsage    `json:"usage"`
}

// AgentEngine Agent 引擎
type AgentEngine struct {
	maxRounds    int
	contextLimit int
}

// NewAgentEngine 创建引擎实例
func NewAgentEngine() *AgentEngine {
	cfg := config.App.Agent
	return &AgentEngine{
		maxRounds:    cfg.MaxToolRounds,
		contextLimit: cfg.ContextMaxMsg,
	}
}

// Run 执行 Agent 循环，通过 sseHandler 输出事件
func (e *AgentEngine) Run(
	session *model.Session,
	userMsg string,
	tools map[string]Tool,
	systemPrompt string,
	sseHandler SSEHandler,
	requestID string,
) error {
	slog.Info("Agent 开始", "request_id", requestID, "session_id", session.ID, "question_preview", truncateRunes(userMsg, 50))

	// 1. 存用户消息
	userMessage := &model.Message{
		SessionID: session.ID,
		Role:      model.RoleUser,
		Content:   userMsg,
	}
	if err := database.DB.Create(userMessage).Error; err != nil {
		return fmt.Errorf("保存用户消息失败: %w", err)
	}

	// 2. 加载上下文（含 system prompt）
	history := e.loadContext(session.ID, systemPrompt)

	// 3. Tool Calling 循环
	var lastTool string
	var sameToolCount int
	openAITools := toolDefsToOpenAI(tools)
	if len(openAITools) == 0 {
		openAITools = nil
	}

	for round := 0; round < e.maxRounds; round++ {
		// 调 LLM（先非流式，需要 tool call 时不用流式）
		resp, err := e.callLLM(history, openAITools)
		if err != nil {
			sseHandler("error", `{"message":"AI 服务暂时不可用，请稍后再试"}`)
			return err
		}

		if len(resp.Choices) == 0 {
			sseHandler("error", `{"message":"AI 未返回有效回答"}`)
			return fmt.Errorf("LLM 返回空 choices")
		}

		choice := resp.Choices[0]

		// LLM 返回了 Tool Calls → 执行工具
		if len(choice.Message.ToolCalls) > 0 {
			// 收集本轮所有 tool call 的上下文消息
			var roundMsgs []agentChatMsg
			for _, tc := range choice.Message.ToolCalls {
				toolName := tc.Function.Name

			// 同一 tool 连续超过 2 次 → 强制终止
			if toolName == lastTool {
				sameToolCount++
			} else {
				lastTool = toolName
				sameToolCount = 1
			}
			if sameToolCount > 2 {
				// 跳过本次 tool 调用，不做任何操作，LLM 下一轮会看到空结果自然停止
				continue
			}
				// 通知前端
				sseHandler("thinking", fmt.Sprintf(`{"tool":"%s","status":"executing"}`, toolName))

				// 查找并执行工具
				tool, ok := tools[toolName]
				var result ToolResult
				if !ok {
					result = ToolResult{Success: false, Content: fmt.Sprintf("未知工具: %s", toolName)}
				} else {
					result = tool.Execute(session.UserID, tc.Function.Arguments)
				}
				resultPreview := result.Content
			if len([]rune(resultPreview)) > 200 {
				resultPreview = string([]rune(resultPreview)[:200]) + "..."
			}
			slog.Info("Agent Tool 执行", "request_id", requestID, "session_id", session.ID, "tool", toolName, "round", round+1, "result", resultPreview)

				// 检测 action 标记（如 trigger_purchase_offer）
				if strings.Contains(result.Content, `"__action"`) {
					var actionData map[string]interface{}
					if json.Unmarshal([]byte(result.Content), &actionData) == nil {
						if actionType, ok := actionData["__action"].(string); ok {
							actionJSON, _ := json.Marshal(map[string]interface{}{
								"type":    actionType,
								"payload": actionData,
							})
							sseHandler("action", string(actionJSON))

							// 存 action 消息到历史
							actionMsg := model.Message{
								SessionID: session.ID,
								Role:      model.RoleAssistant,
								Content:   fmt.Sprintf("已发送购买引导: %v", actionData["title"]),
							}
							database.DB.Create(&actionMsg)
							continue
						}
					}
				}

				// 存 assistant tool_calls 消息（DB 必须有，否则后续 loadContext 时报错）
				assistantToolMsg := model.Message{
					SessionID: session.ID,
					Role:      model.RoleAssistant,
					Content:   "", // tool_calls 消息不需要 content
					ToolCalls: model.ToolCalls{{CallID: tc.ID, Name: toolName, Arguments: tc.Function.Arguments}},
				}
				if err := database.DB.Create(&assistantToolMsg).Error; err != nil {
					return fmt.Errorf("保存 tool_calls 消息失败: %w", err)
				}

				// 存 tool result 消息（含结果和 call_id）
				toolMsg := model.Message{
					SessionID: session.ID,
					Role:      model.RoleTool,
					Content:   result.Content,
					ToolCalls: model.ToolCalls{{CallID: tc.ID, Name: toolName, Arguments: tc.Function.Arguments, Result: result.Content}},
				}
				if err := database.DB.Create(&toolMsg).Error; err != nil {
					return fmt.Errorf("保存工具消息失败: %w", err)
				}

				roundMsgs = append(roundMsgs,
					agentChatMsg{
						Role:             "assistant",
						ReasoningContent: choice.Message.ReasoningContent,
						Content:          choice.Message.Content,
						ToolCalls: []toolCallItem{{
							ID: tc.ID, Type: "function",
							Function: toolCallFunc{Name: toolName, Arguments: tc.Function.Arguments},
						}},
					},
					agentChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
				)
			}
			history = append(history, roundMsgs...)
			continue
		}

		// 没有 Tool Call → 用原生流式 SSE 输出最终回答
		answer, tokens, err := e.callLLMStream(history, openAITools, sseHandler)
		if err != nil {
			sseHandler("error", `{"message":"AI 服务暂时不可用，请稍后再试"}`)
			return err
		}
		displayAnswer := CleanTransferMarkers(answer)

		slog.Info("Agent 回复", "request_id", requestID, "session_id", session.ID, "answer_len", len([]rune(displayAnswer)), "answer_preview", truncateRunes(displayAnswer, 200))

		// 存 assistant message
		assistantMsg := &model.Message{
			SessionID: session.ID,
			Role:      model.RoleAssistant,
			Content:   displayAnswer,
			TokensUsed: tokens,
		}
		if err := database.DB.Create(assistantMsg).Error; err != nil {
			return fmt.Errorf("保存回答失败: %w", err)
		}

		sseHandler("done", fmt.Sprintf(`{"session_id":%d,"agent_type":"%s"}`, session.ID, session.AgentType))
		return nil
	}

	// 超过最大轮数
	sseHandler("error", `{"message":"抱歉，这个问题比较复杂，请联系人工客服"}`)
	return nil
}

// loadContext 从 messages 表加载上下文，system prompt 插入最前
func (e *AgentEngine) loadContext(sessionID uint, systemPrompt string) []agentChatMsg {
	history := []agentChatMsg{
		{Role: "system", Content: systemPrompt},
	}

	var dbMsgs []model.Message
	database.DB.Where("session_id = ?", sessionID).
		Order("id ASC").Limit(e.contextLimit).Find(&dbMsgs)

	for _, m := range dbMsgs {
		if m.Role == model.RoleTool {
			continue
		}
		if m.Role == model.RoleAssistant && len(m.ToolCalls) > 0 && m.Content == "" {
			continue
		}
		// deepseek-v4-pro: 助理消息没有 reasoning_content 则跳过（旧数据）
		if m.Role == model.RoleAssistant && m.ReasoningContent == "" {
			continue
		}
		msg := agentChatMsg{
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
		}
		history = append(history, msg)
	}

	return history
}

// callLLM 调用 LLM API（非流式，用于 Tool Calling 场景）
func (e *AgentEngine) callLLM(history []agentChatMsg, tools []map[string]interface{}) (*llmResponse, error) {
	reqBody := llmRequest{
		Model:     config.App.AI.Model,
		Messages:  history,
		Stream:    false,
		Tools:     tools,
		MaxTokens: 4096,
	}

	LLMSem.Acquire()
	defer LLMSem.Release()

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("请求编码失败: %w", err)
	}

	req, err := http.NewRequest("POST", config.App.AI.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AI服务请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取AI响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Agent LLM API 错误: status=%d body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("AI API 返回状态 %d: %s", resp.StatusCode, string(body))
	}

	var result llmResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("Agent LLM 解析失败: body=%s err=%v", string(body), err)
		return nil, fmt.Errorf("解析AI响应失败: %w", err)
	}
	return &result, nil
}

// streamAnswer 一次发送完整回复（不用逐字流式，避免前端丢字问题）
func (e *AgentEngine) streamAnswer(answer string, sseHandler SSEHandler) error {
	data, _ := json.Marshal(map[string]string{"content": answer})
	return sseHandler("delta", string(data))
}
// callLLMStream 真正的流式调用 LLM API（预留，后续替代 streamAnswer）
func (e *AgentEngine) callLLMStream(history []agentChatMsg, tools []map[string]interface{}, sseHandler SSEHandler) (string, int, error) {
	reqBody := llmRequest{
		Model:    config.App.AI.Model,
		Messages: history,
		Stream:   true,
		Tools:    tools,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, err
	}

	req, err := http.NewRequest("POST", config.App.AI.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	var fullContent strings.Builder
	totalTokens := 0

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *llmUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, c := range chunk.Choices {
			if c.Delta.Content != "" {
				fullContent.WriteString(c.Delta.Content)
				contentJSON, _ := json.Marshal(map[string]string{"content": c.Delta.Content})
				sseHandler("delta", string(contentJSON))
			}
		}
		if chunk.Usage != nil {
			totalTokens = chunk.Usage.TotalTokens
		}
	}

	return fullContent.String(), totalTokens, scanner.Err()
}
