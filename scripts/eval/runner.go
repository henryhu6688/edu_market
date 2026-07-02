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
func runTask(task *EvalTask, evalUserID uint, svc *agent.AgentService, searchFunc agent.SearchFunc, requestID string, verbose bool) (*EvalTrace, error) {
	trace := &EvalTrace{TaskID: task.ID}

	// 1. 噪音类任务：预设上下文历史
	var sessionID *uint
	if len(task.ContextHistory) > 0 {
		s := &model.Session{
			UserID: evalUserID, AgentType: "agent", Status: model.SessionActive, Title: task.ID,
		}
		if err := database.DB.Create(s).Error; err != nil {
			return nil, fmt.Errorf("创建会话失败: %w", err)
		}
		sid := s.ID
		sessionID = &sid
		for _, cm := range task.ContextHistory {
			msg := &model.Message{SessionID: s.ID, Role: cm.Role, Content: cm.Content}
			if err := database.DB.Create(msg).Error; err != nil {
				return nil, fmt.Errorf("创建上下文消息失败: %w", err)
			}
		}
	}

	// 2. 创建 AgentEngine + 设置 trace 收集器
	engine := agent.NewAgentEngine()
	var mu sync.Mutex
	pending := map[string]*ToolCall{} // callID → 待填充结果的 tool call
	maxRound := 0

	engine.SetTraceHandler(func(evt agent.TraceEvent) {
		mu.Lock()
		defer mu.Unlock()

		if evt.Round > maxRound {
			maxRound = evt.Round
		}

		switch evt.Step {
		case "llm_response":
			tok := 0
			if t, ok := evt.Data["tokens"].(int); ok {
				tok = t
			} else if t, ok := evt.Data["tokens"].(float64); ok {
				tok = int(t)
			}
			trace.TotalTokens += tok

			if tcs, ok := evt.Data["tool_calls"].([]interface{}); ok && len(tcs) > 0 {
				// 有 tool_call：为每个创建 ToolCall 记录（pending 状态）
				for _, tc := range tcs {
					tcm, _ := tc.(map[string]interface{})
					fn, _ := tcm["function"].(map[string]interface{})
					name, _ := fn["name"].(string)
					args, _ := fn["arguments"].(string)

					callID := ""
					if id, ok := tcm["id"].(string); ok {
						callID = id
					}
					tc := &ToolCall{Round: evt.Round, ToolName: name, ToolArgs: args}
					trace.ToolCalls = append(trace.ToolCalls, *tc)
					// 用 callID 或 name 做 key 索引最后一个同名的
					key := callID
					if key == "" {
						key = name
					}
					pending[key] = &trace.ToolCalls[len(trace.ToolCalls)-1]
				}
			}

		case "tool_result":
			tn, _ := evt.Data["tool"].(string)
			args, _ := evt.Data["args"].(string)
			ec, _ := evt.Data["error_code"].(string)
			rc, _ := evt.Data["recoverable"].(bool)
			res, _ := evt.Data["result"].(string)

			// 找到匹配的 pending ToolCall 并填充结果
			for k, tc := range pending {
				if tc.ToolName == tn && tc.ToolArgs == args {
					tc.ErrorCode = ec
					tc.Recoverable = rc
					tc.ToolResult = res
					delete(pending, k)
					break
				}
			}
		}

		// verbose
		if verbose {
			switch evt.Step {
			case "llm_request":
				fmt.Printf("  [第%d轮] 🤖 LLM 请求 → model=%v tools=%v\n", evt.Round, evt.Data["model"], evt.Data["tools_count"])
			case "llm_response":
				if tcs, ok := evt.Data["tool_calls"].([]interface{}); ok && len(tcs) > 0 {
					for _, tc := range tcs {
						tcm, _ := tc.(map[string]interface{})
						fn, _ := tcm["function"].(map[string]interface{})
						argStr, _ := fn["arguments"].(string)
						fmt.Printf("  [第%d轮] 🔧 调 %s(%s)\n", evt.Round, fn["name"], truncateStr(argStr, 120))
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

	// 3. SSEHandler 收集最终回答
	var finalAnswer string
	sseHandler := func(event, data string) error {
		if event == "delta" {
			var payload struct{ Content string `json:"content"` }
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
		return trace, nil
	}

	trace.FinalAnswer = finalAnswer
	trace.TotalRounds = maxRound
	if session != nil {
		trace.ModeSequence = append(trace.ModeSequence, session.Mode)
	}

	if verbose {
		fmt.Printf("  📋 完成: %d 次工具调用, %d tokens, 回答 %d 字\n",
			len(trace.ToolCalls), trace.TotalTokens, len([]rune(finalAnswer)))
	}

	return trace, nil
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
