package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"edu_market/database"
	"edu_market/model"
	"edu_market/service/agent"
)

// runTask 执行单条评估任务，返回完整 trace。
// evalUserID 是评估专用测试用户，searchFunc 复用 RAG。
// verbose=true 时实时打印每轮 LLM 请求/响应/Tool 执行到 stdout。
func runTask(task *EvalTask, evalUserID uint, svc *agent.AgentService, searchFunc agent.SearchFunc, requestID string, verbose bool) (*EvalTrace, error) {
	trace := &EvalTrace{TaskID: task.ID}

	// 1. 如任务有预设上下文（噪音类），先在 DB 中创建历史消息
	var sessionID *uint
	if len(task.ContextHistory) > 0 {
		s := &model.Session{
			UserID: evalUserID, AgentType: "agent", Status: model.SessionActive,
			Title: task.ID,
		}
		if err := database.DB.Create(s).Error; err != nil {
			return nil, fmt.Errorf("创建会话失败: %w", err)
		}
		sid := s.ID
		sessionID = &sid
		for _, cm := range task.ContextHistory {
			msg := &model.Message{
				SessionID: s.ID,
				Role:      cm.Role,
				Content:   cm.Content,
			}
			if err := database.DB.Create(msg).Error; err != nil {
				return nil, fmt.Errorf("创建上下文消息失败: %w", err)
			}
		}
	}

	// 2. 创建新的 AgentEngine + 设置 trace 收集器
	engine := agent.NewAgentEngine()
	var mu sync.Mutex
	engine.SetTraceHandler(func(evt agent.TraceEvent) {
		mu.Lock()
		defer mu.Unlock()

		step := TraceStep{
			Round: evt.Round,
			Step:  evt.Step,
		}

		switch evt.Step {
		case "llm_response":
			if fc, ok := evt.Data["finish_reason"].(string); ok {
				step.FinishReason = fc
			}
			if tok, ok := evt.Data["tokens"].(int); ok {
				step.TokensUsed = tok
			} else if tok, ok := evt.Data["tokens"].(float64); ok {
				step.TokensUsed = int(tok)
			}
			if content, ok := evt.Data["content"].(string); ok {
				step.Content = content
			}
			if tcs, ok := evt.Data["tool_calls"].([]interface{}); ok && len(tcs) > 0 {
				for _, tc := range tcs {
					if tcm, ok := tc.(map[string]interface{}); ok {
						if fn, ok := tcm["function"].(map[string]interface{}); ok {
							if name, ok := fn["name"].(string); ok {
								step.ToolName = name
							}
							if args, ok := fn["arguments"].(string); ok {
								step.ToolArgs = args
							}
						}
					}
				}
			}
		case "tool_result":
			if tn, ok := evt.Data["tool"].(string); ok {
				step.ToolName = tn
			}
			if ec, ok := evt.Data["error_code"].(string); ok {
				step.ErrorCode = ec
			}
			if rc, ok := evt.Data["recoverable"].(bool); ok {
				step.Recoverable = rc
			}
		}

		trace.Steps = append(trace.Steps, step)

		// verbose: 实时打印
		if verbose {
			switch evt.Step {
			case "llm_request":
				fmt.Printf("  [第%d轮] 🤖 LLM 请求 → model=%v tools=%v\n", evt.Round, evt.Data["model"], evt.Data["tools_count"])
			case "llm_response":
				if tcs, ok := evt.Data["tool_calls"].([]interface{}); ok && len(tcs) > 0 {
					for _, tc := range tcs {
						if tcm, ok := tc.(map[string]interface{}); ok {
							if fn, ok := tcm["function"].(map[string]interface{}); ok {
								argStr, _ := fn["arguments"].(string)
								fmt.Printf("  [第%d轮] 🔧 调 %s(%s)\n", evt.Round, fn["name"], truncateStr(argStr, 120))
							}
						}
					}
				} else if c, ok := evt.Data["content"].(string); ok && c != "" {
					fmt.Printf("  [第%d轮] 💬 文本: %s\n", evt.Round, truncateStr(c, 100))
				}
			case "tool_result":
				success, _ := evt.Data["success"].(bool)
				tn, _ := evt.Data["tool"].(string)
				result, _ := evt.Data["result"].(string)
				icon := "✅"
				if !success {
					icon = "❌"
				}
				fmt.Printf("  [第%d轮] %s %s → %s\n", evt.Round, icon, tn, truncateStr(result, 150))
			}
		}
	})

	evalSvc := agent.NewAgentService(engine)

	// 3. 通过 SSEHandler 收集 final answer
	var finalAnswer string
	sseHandler := func(event, data string) error {
		if event == "delta" {
			var payload struct {
				Content string `json:"content"`
			}
			if json.Unmarshal([]byte(data), &payload) == nil {
				finalAnswer += payload.Content
			}
		}
		return nil
	}

	slog.Info("eval 开始执行任务", "task_id", task.ID, "category", task.Category)

	if verbose {
		fmt.Printf("\n━━━ %s [%s] ━━━\n输入: %s\n", task.ID, task.Category, task.Input)
	}

	session, err := evalSvc.Chat(evalUserID, sessionID, task.Input, searchFunc, sseHandler, requestID)
	if err != nil {
		trace.Error = err.Error()
		return trace, nil // 不返回 error——部分失败也要出 trace
	}

	trace.FinalAnswer = finalAnswer
	trace.TotalRounds = len(trace.Steps) / 2 // 大致：每个 round 产生 2 个 trace event
	if session != nil {
		trace.ModeSequence = append(trace.ModeSequence, session.Mode)
	}

	// 4. 计算总 tokens
	for _, s := range trace.Steps {
		trace.TotalTokens += s.TokensUsed
	}

	if verbose {
		fmt.Printf("  📋 完成: %d steps, %d tokens, 回答 %d 字\n",
			len(trace.Steps), trace.TotalTokens, len([]rune(finalAnswer)))
	}

	return trace, nil
}

// truncateStr 截断字符串到指定长度
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
