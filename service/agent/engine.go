package agent

import (
	"bytes"
	"context"
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

// TraceHandler 诊断回调：每步关键事件都会触发，用于调测和 REPL
type TraceHandler func(event TraceEvent)

// TraceEvent 诊断事件
type TraceEvent struct {
	Step  string                 `json:"step"`
	Round int                    `json:"round,omitempty"`
	Data  map[string]interface{} `json:"data"`
}

// agentChatMsg LLM 消息格式（OpenAI 兼容）
type agentChatMsg struct {
	Role             string         `json:"role"`
	Content          string         `json:"content,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	ToolCalls        []toolCallItem `json:"tool_calls,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
}

type toolCallItem struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function toolCallFunc `json:"function"`
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
	FinishReason string       `json:"finish_reason"`
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
	traceHandler TraceHandler
}

// NewAgentEngine 创建引擎实例
func NewAgentEngine() *AgentEngine {
	cfg := config.App.Agent
	return &AgentEngine{
		maxRounds:    cfg.MaxToolRounds,
		contextLimit: cfg.ContextMaxMsg,
	}
}

// SetTraceHandler 设置诊断回调（可选，仅调测用）
func (e *AgentEngine) SetTraceHandler(h TraceHandler) {
	e.traceHandler = h
}

func (e *AgentEngine) trace(evt TraceEvent) {
	if e.traceHandler != nil {
		e.traceHandler(evt)
	}
}

// ============ 主循环 ============

// Run 执行 Agent 循环，通过 sseHandler 输出事件。
// 集成六道防线：L1 精确重复 / L2 语义回路 / L3 硬上限 / 模式白名单 / 参数校验 / 调用预算。
func (e *AgentEngine) Run(
	session *model.Session,
	userMsg string,
	tools map[string]Tool,
	systemPrompt string,
	sseHandler SSEHandler,
	requestID string,
) error {
	slog.Info("Agent 开始", "request_id", requestID, "session_id", session.ID, "mode", session.Mode, "question", TruncateRunes(userMsg, 80))

	// ========== 0. 初始化安全组件 ==========
	breaker := &CircuitBreaker{}
	loopDetector := &SemanticLoopDetector{}
	budget := NewToolBudget()
	corrector := &HardFieldCorrector{}

	// ========== 1. 存用户消息 ==========
	userMessage := &model.Message{
		SessionID: session.ID,
		Role:      model.RoleUser,
		Content:   userMsg,
	}
	if err := database.DB.Create(userMessage).Error; err != nil {
		return fmt.Errorf("保存用户消息失败: %w", err)
	}

	// ========== 2. 初始化 State（新会话）==========
	if session.State == "" {
		session.State = e.initState(userMsg)
	}

	// ========== 3. 组装 Prompt 并加载上下文 ==========
	prompt := systemPrompt
	if prompt == "" {
		prompt = e.buildPrompt(session)
	}
	history := []agentChatMsg{
		{Role: "system", Content: prompt},
	}
	history = append(history, e.loadRecentMessages(session.ID)...)

	e.trace(TraceEvent{
		Step: "context_loaded",
		Data: map[string]interface{}{
			"session_id":     session.ID,
			"messages_count": len(history),
			"system_prompt":  prompt,
		},
	})
	toolNames := make([]string, 0, len(tools))
	for name := range tools {
		toolNames = append(toolNames, name)
	}
	slog.Info("Agent 上下文就绪", "request_id", requestID, "session_id", session.ID, "history_msgs", len(history), "tools", toolNames)

	// ========== 4. Tool Calling 循环 ==========
	openAITools := toolDefsToOpenAI(tools)
	if len(openAITools) == 0 {
		openAITools = nil
	}

	for round := 0; round < e.maxRounds; round++ {
		// Level 3: 硬上限 — 最后一轮跳过 tool calling，直接最终回答
		if round >= e.maxRounds-1 {
			history = append(history, agentChatMsg{
				Role:    "system",
				Content: "已达到最大对话步数限制。你必须立即基于已有信息回答用户问题，不要再尝试调用任何工具。如果现有信息不足以回答，请诚实告知用户。",
			})
			return e.streamFinalAnswer(session, history, sseHandler, corrector, requestID)
		}

		// ----- 调 LLM（非流式，带 tools）-----
		e.trace(TraceEvent{
			Step:  "llm_request",
			Round: round + 1,
			Data: map[string]interface{}{
				"model":       config.App.AI.Model,
				"messages":    messagesToTrace(history),
				"tools_count": len(openAITools),
				"stream":      false,
			},
		})
		slog.Info("Round 开始", "request_id", requestID, "round", round+1, "history_msgs", len(history), "tools_available", len(openAITools))

		llmStart := time.Now()
		resp, err := e.callLLMWithRetry(history, openAITools)
		if err != nil {
			sseHandler("error", `{"message":"AI 服务暂时不可用，请稍后重试"}`)
			return err
		}

		if len(resp.Choices) == 0 {
			sseHandler("error", `{"message":"AI 未返回有效回答"}`)
			return fmt.Errorf("LLM 返回空 choices")
		}

		choice := resp.Choices[0]

		e.trace(TraceEvent{
			Step:  "llm_response",
			Round: round + 1,
			Data: map[string]interface{}{
				"finish_reason":  choice.FinishReason,
				"has_tool_calls": len(choice.Message.ToolCalls) > 0,
				"tool_calls":     toolCallsToTrace(choice.Message.ToolCalls),
				"content":        choice.Message.Content,
				"tokens":         resp.Usage.TotalTokens,
			},
		})
		slog.Info("LLM 响应", "request_id", requestID, "round", round+1, "finish", choice.FinishReason, "tool_calls", len(choice.Message.ToolCalls), "content_len", len([]rune(choice.Message.Content)), "tokens", resp.Usage.TotalTokens, "llm_ms", time.Since(llmStart).Milliseconds())

		// ----- 没有 Tool Call → 最终回答 -----
		if len(choice.Message.ToolCalls) == 0 {
			if choice.Message.Content != "" {
				return e.finalizeAnswer(session, choice.Message.Content, sseHandler, corrector, resp.Usage.TotalTokens, requestID)
			}
			return e.streamFinalAnswer(session, history, sseHandler, corrector, requestID)
		}

		// ----- 有 Tool Calls → 执行工具 -----
		var executedTools []string
		var roundMsgs []agentChatMsg

		for _, tc := range choice.Message.ToolCalls {
			toolName := tc.Function.Name
			argsJSON := tc.Function.Arguments
			tool, toolExists := tools[toolName]

			// Level 1: 精确重复
			if blocked, reason := breaker.Check(tool, toolName, argsJSON); blocked {
				result := ToolResult{Success: false, Content: reason, Source: "blocked", ErrorCode: "TOOL_BLOCKED", Recoverable: false, RecommendedAction: "tell_user_boundary"}
				e.storeToolMessagesDB(session.ID, tc, toolName, result)
				roundMsgs = append(roundMsgs,
					agentChatMsg{
						Role:             "assistant",
						ReasoningContent: choice.Message.ReasoningContent,
						Content:          choice.Message.Content,
						ToolCalls:        []toolCallItem{{ID: tc.ID, Type: "function", Function: toolCallFunc{Name: toolName, Arguments: argsJSON}}},
					},
					agentChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
				)
				slog.Warn("L1 熔断", "request_id", requestID, "tool", toolName)
				continue
			}
			breaker.Record(toolName, argsJSON)

			// 工具不存在
			if !toolExists {
				result := ToolResult{Success: false, Content: fmt.Sprintf("未知工具: %s，请换用其他可用工具", toolName), Source: "error", ErrorCode: "UNKNOWN_TOOL", Recoverable: true, RecommendedAction: "try_alternative_tool"}
				e.storeToolMessagesDB(session.ID, tc, toolName, result)
				roundMsgs = append(roundMsgs,
					agentChatMsg{
						Role:             "assistant",
						ReasoningContent: choice.Message.ReasoningContent,
						Content:          choice.Message.Content,
						ToolCalls:        []toolCallItem{{ID: tc.ID, Type: "function", Function: toolCallFunc{Name: toolName, Arguments: argsJSON}}},
					},
					agentChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
				)
				continue
			}

			// 模式白名单（第一轮 mode="" 跳过）
			if err := e.checkToolMode(tool, session.Mode); err != nil {
				result := ToolResult{Success: false, Content: err.Error(), Source: "blocked", ErrorCode: "ACCESS_DENIED", Recoverable: false, RecommendedAction: "tell_user_boundary"}
				slog.Warn("模式白名单拦截", "request_id", requestID, "tool", toolName, "mode", session.Mode, "error", err)
				e.storeToolMessagesDB(session.ID, tc, toolName, result)
				roundMsgs = append(roundMsgs,
					agentChatMsg{
						Role:             "assistant",
						ReasoningContent: choice.Message.ReasoningContent,
						Content:          choice.Message.Content,
						ToolCalls:        []toolCallItem{{ID: tc.ID, Type: "function", Function: toolCallFunc{Name: toolName, Arguments: argsJSON}}},
					},
					agentChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
				)
				continue
			}

			// 参数校验
			if err := tool.ValidateArgs(argsJSON); err != nil {
				result := ToolResult{Success: false, Content: "参数错误: " + err.Error(), Source: "blocked", ErrorCode: "INVALID_ARGUMENT", Recoverable: true, RecommendedAction: "fix_arguments_and_retry"}
				slog.Warn("参数校验拦截", "request_id", requestID, "tool", toolName, "args", argsJSON, "error", err)
				e.storeToolMessagesDB(session.ID, tc, toolName, result)
				roundMsgs = append(roundMsgs,
					agentChatMsg{
						Role:             "assistant",
						ReasoningContent: choice.Message.ReasoningContent,
						Content:          choice.Message.Content,
						ToolCalls:        []toolCallItem{{ID: tc.ID, Type: "function", Function: toolCallFunc{Name: toolName, Arguments: argsJSON}}},
					},
					agentChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
				)
				continue
			}

			// 调用预算
			if err := budget.Spend(toolName); err != nil {
				result := ToolResult{Success: false, Content: err.Error(), Source: "blocked", ErrorCode: "BUDGET_EXCEEDED", Recoverable: false, RecommendedAction: "tell_user_try_later"}
				slog.Warn("调用预算耗尽", "request_id", requestID, "tool", toolName, "error", err)
				e.storeToolMessagesDB(session.ID, tc, toolName, result)
				roundMsgs = append(roundMsgs,
					agentChatMsg{
						Role:             "assistant",
						ReasoningContent: choice.Message.ReasoningContent,
						Content:          choice.Message.Content,
						ToolCalls:        []toolCallItem{{ID: tc.ID, Type: "function", Function: toolCallFunc{Name: toolName, Arguments: argsJSON}}},
					},
					agentChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
				)
				continue
			}

			// 执行 Tool
			toolIdx := len(executedTools) + 1
			toolTotal := len(choice.Message.ToolCalls)
			sseHandler("thinking", fmt.Sprintf(`{"tool":"%s","status":"executing"}`, toolName))
			toolStart := time.Now()
			result := tool.Execute(session.UserID, argsJSON)
			toolMs := time.Since(toolStart).Milliseconds()

			// 存 DB
			e.storeToolMessagesDB(session.ID, tc, toolName, result)

			resultPreview := TruncateRunes(result.Content, 200)
			logAttrs := []any{
			"request_id", requestID,
			"tool", fmt.Sprintf("[%d/%d] %s", toolIdx, toolTotal, toolName),
			"ok", result.Success,
			"len", len(result.Content),
			"ms", toolMs,
			"preview", resultPreview,
		}
		if !result.Success {
			logAttrs = append(logAttrs, "error_code", result.ErrorCode, "recoverable", result.Recoverable)
		}
		slog.Info("Tool ✓", logAttrs...)

			// Level 2: 语义回路
			if blocked, reason := loopDetector.Feed(result.Content); blocked {
				history = append(history, agentChatMsg{Role: "system", Content: reason})
				slog.Warn("L2 语义回路触发", "request_id", requestID, "reason", reason)
			}

			// 上下文分层
			e.updateFactsAndHypotheses(session, toolName, argsJSON, result)
			// 更新 State
			e.updateTaskState(session, tool, toolName, argsJSON, result)

			executedTools = append(executedTools, toolName)

			// action 检测（用原始 result，__action 标记还在）
			if strings.Contains(result.Content, `"__action"`) {
				e.handleAction(sseHandler, session, tc, result)
			}

			// 本轮消息：清理 __action 内部标记，避免泄漏到 LLM 上下文让它误判
			llmResult := cleanActionResult(result.Content)

			roundMsgs = append(roundMsgs,
				agentChatMsg{
					Role:             "assistant",
					ReasoningContent: choice.Message.ReasoningContent,
					Content:          choice.Message.Content,
					ToolCalls:        []toolCallItem{{ID: tc.ID, Type: "function", Function: toolCallFunc{Name: toolName, Arguments: argsJSON}}},
				},
				agentChatMsg{Role: "tool", Content: llmResult, ToolCallID: tc.ID},
			)
		}

		// 更新模式 + 保存 Session
		session.Mode = ResolveMode(session, executedTools)
		e.saveSession(session)

		history = append(history, roundMsgs...)
		slog.Info("Round 结束", "request_id", requestID, "round", round+1, "tools_executed", len(executedTools), "new_mode", session.Mode, "next_round", round+2)
	}

	// 超过最大轮数
	sseHandler("error", `{"message":"抱歉，这个问题比较复杂，请联系人工客服"}`)
	return nil
}

// streamFinalAnswer 最终兜底：LLM 返回空 content 时才额外调一次拿回答。
func (e *AgentEngine) streamFinalAnswer(session *model.Session, history []agentChatMsg, sseHandler SSEHandler, corrector *HardFieldCorrector, requestID string) error {
	history = append(history, agentChatMsg{
		Role:    "system",
		Content: "请基于以上信息简洁回答用户。只输出回答内容。",
	})

	finalResp, err := e.callLLMWithRetry(history, nil)
	if err != nil {
		sseHandler("error", `{"message":"AI 服务暂时不可用，请稍后重试"}`)
		return err
	}
	return e.finalizeAnswer(session, finalResp.Choices[0].Message.Content, sseHandler, corrector, finalResp.Usage.TotalTokens, requestID)
}

// finalizeAnswer quality 修正 → 流式输出 → 存 DB → done 事件。
func (e *AgentEngine) finalizeAnswer(session *model.Session, fullAnswer string, sseHandler SSEHandler, corrector *HardFieldCorrector, tokens int, requestID string) error {
	facts := e.getFacts(session.State)
	fullAnswer = corrector.Correct(fullAnswer, facts)

	e.streamAnswer(fullAnswer, func(delta string) {
		sseHandler("delta", formatDelta(delta))
	})

	displayAnswer := CleanTransferMarkers(fullAnswer)
	slog.Info("Agent 回复", "request_id", requestID, "session_id", session.ID, "len", len([]rune(displayAnswer)), "preview", TruncateRunes(displayAnswer, 200))

	e.storeAssistantMessageDB(session.ID, fullAnswer, tokens)
	sseHandler("done", fmt.Sprintf(`{"session_id":%d,"agent_type":"%s"}`, session.ID, session.AgentType))
	return nil
}

// ============ LLM 调用 ============

// callLLMWithRetry 调用 LLM API（非流式），带 1 次重试。
func (e *AgentEngine) callLLMWithRetry(history []agentChatMsg, tools []map[string]interface{}) (*llmResponse, error) {
	resp, err := e.callLLM(history, tools)
	if err != nil {
		slog.Warn("LLM 调用失败，3秒后重试", "error", err)
		time.Sleep(3 * time.Second)
		resp, err = e.callLLM(history, tools)
		if err != nil {
			slog.Error("LLM 调用失败（已重试）", "error", err)
			return nil, err
		}
	}
	return resp, nil
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

	llmRate.Wait(context.Background())

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

// ============ 上下文加载 ============

// loadRecentMessages 从 messages 表加载最近 N 条消息给 LLM。
// DESC 取最新 → 反转为 ASC（时间顺序）。
func (e *AgentEngine) loadRecentMessages(sessionID uint) []agentChatMsg {
	// 1. 最近 N 条（DESC 取最新 → 反转为 ASC）
	var dbMsgs []model.Message
	database.DB.Where("session_id = ?", sessionID).
		Order("id DESC").Limit(e.contextLimit).Find(&dbMsgs)
	for i, j := 0, len(dbMsgs)-1; i < j; i, j = i+1, j-1 {
		dbMsgs[i], dbMsgs[j] = dbMsgs[j], dbMsgs[i]
	}

	// 2. 过滤 action 卡片 + 还原 tool_calls 格式
	var msgs []agentChatMsg
	for _, m := range dbMsgs {
		if m.Role == model.RoleAssistant && strings.Contains(m.Content, `"purchase_offer"`) {
			continue
		}
		msg := agentChatMsg{
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
		}
		if m.Role == model.RoleAssistant && len(m.ToolCalls) > 0 {
			msg.ToolCalls = restoreToolCallsItem(m.ToolCalls)
		}
		if m.Role == model.RoleTool && len(m.ToolCalls) > 0 {
			msg.ToolCallID = m.ToolCalls[0].CallID
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

// restoreToolCallsItem 还原 tool_calls 数组。
func restoreToolCallsItem(tcs model.ToolCalls) []toolCallItem {
	result := make([]toolCallItem, len(tcs))
	for i, tc := range tcs {
		result[i] = toolCallItem{
			ID:       tc.CallID,
			Type:     "function",
			Function: toolCallFunc{Name: tc.Name, Arguments: tc.Arguments},
		}
	}
	return result
}

// ============ 上下文分层管理 ============

// updateFactsAndHypotheses 上下文分层维护。每轮 Tool 执行后调用。
func (e *AgentEngine) updateFactsAndHypotheses(session *model.Session, toolName, argsJSON string, result ToolResult) {
	if !result.Success {
		return
	}

	var state SessionState
	if err := json.Unmarshal([]byte(session.State), &state); err != nil {
		return
	}

	source := fmt.Sprintf("%s(%s)", toolName, argsJSON)

	// 同 source 旧数据 → 移入 discarded
	e.expireOldFacts(&state, source)

	// 新数据 → fact
	state.Facts = append(state.Facts, FactItem{
		Content: TruncateRunes(result.Content, 150),
		Source:  source,
	})

	b, _ := json.Marshal(state)
	session.State = string(b)
}

// expireOldFacts 将同 source 的旧 fact/hypothesis 移入 discarded。
func (e *AgentEngine) expireOldFacts(state *SessionState, source string) {
	for i, f := range state.Facts {
		if f.Source == source {
			state.Discarded = append(state.Discarded, FactItem{
				Content: f.Content, Source: f.Source, Basis: "被新数据覆盖",
			})
			state.Facts = append(state.Facts[:i], state.Facts[i+1:]...)
			break
		}
	}
	for i, h := range state.Hypotheses {
		if h.Source == source {
			state.Discarded = append(state.Discarded, FactItem{
				Content: h.Content, Source: h.Source, Basis: "被新数据覆盖",
			})
			state.Hypotheses = append(state.Hypotheses[:i], state.Hypotheses[i+1:]...)
			break
		}
	}
}

// updateTaskState 更新进度状态：completed / context / to_do。
func (e *AgentEngine) updateTaskState(session *model.Session, tool Tool, toolName, argsJSON string, result ToolResult) {
	if !result.Success {
		return
	}

	var state SessionState
	if err := json.Unmarshal([]byte(session.State), &state); err != nil {
		return
	}

	// 追加 completed
	desc := tool.Describe(argsJSON, result)
	state.Completed = append(state.Completed, desc)

	// 更新业务上下文
	switch toolName {
	case "get_material_detail":
		var args struct{ MaterialID uint }
		json.Unmarshal([]byte(argsJSON), &args)
		state.Context.FocusID = args.MaterialID
		state.Context.MaterialsViewed = appendUnique(state.Context.MaterialsViewed, args.MaterialID)
	case "trigger_purchase_offer":
		state.Context.CardSent = true
	case "query_materials":
		state.Context.Candidates = extractCandidates(result.Content)
	}

	// 重新计算 to_do
	mode := session.Mode
	if mode == "" {
		mode = ResolveMode(session, []string{toolName})
	}
	state.ToDo = computeToDo(mode, state.Completed, state.Context)

	b, _ := json.Marshal(state)
	session.State = string(b)
}

// initState 初始化会话 State（task = userMsg，不调 LLM）。
func (e *AgentEngine) initState(userMsg string) string {
	state := SessionState{
		Task:    userMsg,
		Context: ContextData{},
	}
	b, _ := json.Marshal(state)
	return string(b)
}

// getFacts 从 Session.State JSON 中提取 facts 数组，供 quality.go 使用。
func (e *AgentEngine) getFacts(stateJSON string) []FactItem {
	if stateJSON == "" {
		return nil
	}
	var state SessionState
	if json.Unmarshal([]byte(stateJSON), &state) != nil {
		return nil
	}
	return state.Facts
}

// ============ Session 持久化 ============

// saveSession 持久化 Session（State + Mode）。
func (e *AgentEngine) saveSession(session *model.Session) {
	database.DB.Model(session).Updates(map[string]interface{}{
		"mode":  session.Mode,
		"state": session.State,
	})
}

// ============ 消息存储 ============

// storeToolMessagesDB 存 assistant tool_calls 消息 + tool result 消息到 DB。
func (e *AgentEngine) storeToolMessagesDB(sessionID uint, tc toolCallItem, toolName string, result ToolResult) {
	assistantToolMsg := model.Message{
		SessionID: sessionID,
		Role:      model.RoleAssistant,
		Content:   "",
		ToolCalls: model.ToolCalls{{CallID: tc.ID, Name: toolName, Arguments: tc.Function.Arguments}},
	}
	database.DB.Create(&assistantToolMsg)

	toolMsg := model.Message{
		SessionID: sessionID,
		Role:      model.RoleTool,
		Content:   result.Content,
		ToolCalls: model.ToolCalls{{CallID: tc.ID, Name: toolName, Arguments: tc.Function.Arguments, Result: result.Content}},
	}
	database.DB.Create(&toolMsg)
}

// storeAssistantMessageDB 存 assistant 最终回答消息到 DB。
func (e *AgentEngine) storeAssistantMessageDB(sessionID uint, content string, tokens int) {
	assistantMsg := &model.Message{
		SessionID:  sessionID,
		Role:       model.RoleAssistant,
		Content:    content,
		TokensUsed: tokens,
	}
	database.DB.Create(assistantMsg)
}

// ============ 动作处理 ============

// handleAction 处理 tool 返回的购买卡片动作。
func (e *AgentEngine) handleAction(sseHandler SSEHandler, session *model.Session, tc toolCallItem, result ToolResult) {
	var actionData map[string]interface{}
	if json.Unmarshal([]byte(result.Content), &actionData) != nil {
		return
	}
	actionType, ok := actionData["__action"].(string)
	if !ok {
		return
	}
	actionJSON, _ := json.Marshal(map[string]interface{}{
		"type":    actionType,
		"payload": actionData,
	})
	sseHandler("action", string(actionJSON))

	dbActionMsg := model.Message{
		SessionID: session.ID,
		Role:      model.RoleAssistant,
		Content:   string(actionJSON),
		ToolCalls: model.ToolCalls{{CallID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments, Result: result.Content}},
	}
	database.DB.Create(&dbActionMsg)
	slog.Info("Agent 发送 action", "action_type", actionType)
}

// ============ 流式输出 ============

// streamAnswer 模拟流式输出：将完整回答逐字拆发 delta 事件。
func (e *AgentEngine) streamAnswer(answer string, onDelta func(string)) {
	runes := []rune(answer)
	for i := 0; i < len(runes); i += 3 {
		end := i + 3
		if end > len(runes) {
			end = len(runes)
		}
		onDelta(string(runes[i:end]))
	}
}

// cleanActionResult 清除 tool result 中的 __action 内部标记，避免泄漏到 LLM 上下文。
// LLM 看到 "需要处理某事" 会误以为卡片没发成功，从而重复调用。
func cleanActionResult(content string) string {
	if !strings.Contains(content, `"__action"`) {
		return content
	}
	// 用简单的 JSON 解析删除 __action 字段，保留其他字段
	var raw map[string]interface{}
	if json.Unmarshal([]byte(content), &raw) != nil {
		return content
	}
	delete(raw, "__action")
	b, _ := json.Marshal(raw)
	return string(b)
}

// formatDelta 格式化 delta SSE 数据。
func formatDelta(delta string) string {
	b, _ := json.Marshal(map[string]string{"content": delta})
	return string(b)
}

// ============ 辅助函数 ============

// extractCandidates 从 query_materials 返回 JSON 中提取候选资料列表。
func extractCandidates(content string) []Candidate {
	var materials []model.Material
	if json.Unmarshal([]byte(content), &materials) != nil {
		return nil
	}
	var candidates []Candidate
	for _, m := range materials {
		candidates = append(candidates, Candidate{ID: m.ID, Title: m.Title, Price: m.Price})
	}
	return candidates
}

// appendUnique 向 uint 切片追加不重复的元素。
func appendUnique(slice []uint, v uint) []uint {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

// ============ 诊断辅助函数 ============

// messagesToTrace 将内部消息数组转为可序列化的切片（供 trace 使用）
func messagesToTrace(msgs []agentChatMsg) []interface{} {
	result := make([]interface{}, len(msgs))
	for i, m := range msgs {
		item := map[string]interface{}{
			"role":    m.Role,
			"content": m.Content,
		}
		if m.ToolCallID != "" {
			item["tool_call_id"] = m.ToolCallID
		}
		if m.ReasoningContent != "" {
			item["reasoning_content"] = m.ReasoningContent
		}
		if len(m.ToolCalls) > 0 {
			tcs := make([]interface{}, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				tcs[j] = map[string]interface{}{
					"id":   tc.ID,
					"type": tc.Type,
					"function": map[string]interface{}{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					},
				}
			}
			item["tool_calls"] = tcs
		}
		result[i] = item
	}
	return result
}

// toolCallsToTrace 将 toolCallItem 切片转为可序列化的切片
func toolCallsToTrace(tcs []toolCallItem) []interface{} {
	result := make([]interface{}, len(tcs))
	for i, tc := range tcs {
		result[i] = map[string]interface{}{
			"id":   tc.ID,
			"type": tc.Type,
			"function": map[string]interface{}{
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			},
		}
	}
	return result
}
