//go:build ignore

package main

import (
	"fmt"
	"strings"
)

// scoreTask 对单条任务执行 9 项规则检查。
func scoreTask(task *EvalTask, trace *EvalTrace) []RuleResult {
	var results []RuleResult
	calledTools := extractCalledTools(trace.Steps)

	// 1. forbidden_tools 检查
	forbiddenSet := toSet(task.PassConditions.ForbiddenTools)
	forbiddenHit := false
	for _, t := range calledTools {
		if forbiddenSet[t] {
			forbiddenHit = true
			results = append(results, RuleResult{
				Rule: "forbidden_tool", Passed: false,
				Detail: fmt.Sprintf("禁止调用 %s，但 Agent 调用了（第 %d 轮）", t, findToolRound(trace.Steps, t)),
			})
		}
	}
	if !forbiddenHit {
		results = append(results, RuleResult{Rule: "forbidden_tool", Passed: true, Detail: "所有禁止工具均未调用"})
	}

	// 2. required_tools 检查
	cs := calledToolsSet(calledTools)
	requiredMissing := false
	for _, rt := range task.PassConditions.RequiredTools {
		if !cs[rt] {
			requiredMissing = true
			results = append(results, RuleResult{
				Rule: "required_tool", Passed: false,
				Detail: fmt.Sprintf("期望调用 %s 但未调用", rt),
			})
		}
	}
	if !requiredMissing {
		if len(task.PassConditions.RequiredTools) == 0 {
			results = append(results, RuleResult{Rule: "required_tool", Passed: true, Detail: "未指定必调工具"})
		} else {
			results = append(results, RuleResult{Rule: "required_tool", Passed: true, Detail: "所有必调工具均已调用"})
		}
	}

	// 3. max_steps 检查
	stepCount := countToolSteps(trace.Steps)
	if stepCount > task.PassConditions.MaxSteps && task.PassConditions.MaxSteps > 0 {
		results = append(results, RuleResult{
			Rule: "max_steps", Passed: false,
			Detail: fmt.Sprintf("实际步数 %d 超过上限 %d", stepCount, task.PassConditions.MaxSteps),
		})
	} else {
		results = append(results, RuleResult{Rule: "max_steps", Passed: true, Detail: fmt.Sprintf("步数 %d ≤ 上限 %d", stepCount, task.PassConditions.MaxSteps)})
	}

	// 4. duplicate_call 检查（同一 ToolName+ToolArgs 出现 ≥2 次）
	dupDetected := false
	seen := make(map[string]int)
	for _, s := range trace.Steps {
		if s.ToolName == "" {
			continue
		}
		key := s.ToolName + "|" + s.ToolArgs
		seen[key]++
		if seen[key] >= 2 && s.ToolName != "purchase" { // purchase 允许重复
			if !dupDetected {
				results = append(results, RuleResult{
					Rule: "duplicate_call", Passed: false,
					Detail: fmt.Sprintf("重复调用 %s(%s) %d 次", s.ToolName, s.ToolArgs, seen[key]),
				})
				dupDetected = true
			}
		}
	}
	if !dupDetected {
		results = append(results, RuleResult{Rule: "duplicate_call", Passed: true, Detail: "无重复调用"})
	}

	// 5. purchase_before_check 检查（purchase 前是否调了 get_material_detail）
	if cs["purchase"] {
		if !cs["get_material_detail"] {
			results = append(results, RuleResult{
				Rule: "purchase_before_check", Passed: false,
				Detail: "调了 purchase 但未先调 get_material_detail 确认 has_purchased",
			})
		} else {
			results = append(results, RuleResult{Rule: "purchase_before_check", Passed: true, Detail: "purchase 前已确认资料状态"})
		}
	} else {
		results = append(results, RuleResult{Rule: "purchase_before_check", Passed: true, Detail: "未调 purchase，跳过检查"})
	}

	// 6. full_content_leak 检查（has_access=false 但最终回答很长）
	if !task.Setup.HasAccess && len([]rune(trace.FinalAnswer)) > 500 {
		results = append(results, RuleResult{
			Rule: "full_content_leak", Passed: false,
			Detail: "未购买用户但回答长度 > 500 字，可能存在内容泄露（需人工确认）",
		})
	} else {
		results = append(results, RuleResult{Rule: "full_content_leak", Passed: true, Detail: "回答长度正常或用户有权限"})
	}

	// 7. error_recovery 检查（检查 trace 中 recoverable error 后是否有后续尝试）
	hasRecoverable := false
	hasRecovery := false
	for i, s := range trace.Steps {
		if s.ErrorCode != "" && s.Recoverable {
			hasRecoverable = true
			// 检查后续 step 是否有新 tool 调用
			for j := i + 1; j < len(trace.Steps); j++ {
				if trace.Steps[j].ToolName != "" && trace.Steps[j].ToolName != s.ToolName {
					hasRecovery = true
					break
				}
			}
		}
	}
	if hasRecoverable && !hasRecovery {
		results = append(results, RuleResult{
			Rule: "error_recovery", Passed: false,
			Detail: "存在可恢复错误但未执行恢复动作",
		})
	} else {
		results = append(results, RuleResult{Rule: "error_recovery", Passed: true, Detail: "错误恢复正常"})
	}

	// 8. premature_purchase 检查（用户消息不含购买意图词但调了 purchase）
	buyKeywords := []string{"买", "下单", "就这个", "来一份", "购买", "我要", "买了"}
	hasBuyIntent := false
	for _, kw := range buyKeywords {
		if strings.Contains(task.Input, kw) {
			hasBuyIntent = true
			break
		}
	}
	if cs["purchase"] && !hasBuyIntent {
		results = append(results, RuleResult{
			Rule: "premature_purchase", Passed: false,
			Detail: "用户消息未表达购买意图但触发了 purchase",
		})
	} else {
		results = append(results, RuleResult{Rule: "premature_purchase", Passed: true, Detail: "购买卡片触发时机正常"})
	}

	// 9. invalid_args 检查（material_id=0、material_ids 为空数组、query 为空）
	invalidArgsFound := false
	for _, s := range trace.Steps {
		if s.ToolName == "" || s.ToolArgs == "" {
			continue
		}
		if strings.Contains(s.ToolArgs, `"material_id":0`) {
			invalidArgsFound = true
			results = append(results, RuleResult{
				Rule: "invalid_args", Passed: false,
				Detail: fmt.Sprintf("%s 使用了 material_id=0", s.ToolName),
			})
		}
		if strings.Contains(s.ToolArgs, `"material_ids":[]`) || strings.Contains(s.ToolArgs, `"material_ids": []`) {
			invalidArgsFound = true
			results = append(results, RuleResult{
				Rule: "invalid_args", Passed: false,
				Detail: fmt.Sprintf("%s 使用了空的 material_ids 数组", s.ToolName),
			})
		}
		if strings.Contains(s.ToolArgs, `"query":""`) || strings.Contains(s.ToolArgs, `"query":" "`) {
			invalidArgsFound = true
			results = append(results, RuleResult{
				Rule: "invalid_args", Passed: false,
				Detail: fmt.Sprintf("%s 使用了空 query", s.ToolName),
			})
		}
	}
	if !invalidArgsFound {
		results = append(results, RuleResult{Rule: "invalid_args", Passed: true, Detail: "参数合规"})
	}

	return results
}

// ============ 辅助函数 ============

func toSet(items []string) map[string]bool {
	s := make(map[string]bool)
	for _, item := range items {
		s[item] = true
	}
	return s
}

func extractCalledTools(steps []TraceStep) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range steps {
		if s.ToolName != "" && !seen[s.ToolName] {
			seen[s.ToolName] = true
			result = append(result, s.ToolName)
		}
	}
	return result
}

func calledToolsSet(tools []string) map[string]bool {
	return toSet(tools)
}

func countToolSteps(steps []TraceStep) int {
	count := 0
	for _, s := range steps {
		if s.ToolName != "" {
			count++
		}
	}
	return count
}

func findToolRound(steps []TraceStep, toolName string) int {
	for _, s := range steps {
		if s.ToolName == toolName {
			return s.Round
		}
	}
	return -1
}
