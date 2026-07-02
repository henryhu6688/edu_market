package main

import (
	"fmt"
	"strings"
)

// scoreTask 对单条任务执行 9 项规则检查。
func scoreTask(task *EvalTask, trace *EvalTrace) []RuleResult {
	var results []RuleResult
	calls := trace.ToolCalls
	calledTools := extractCalledTools(calls)

	// 1. forbidden_tools
	forbiddenSet := toSet(task.PassConditions.ForbiddenTools)
	forbiddenHit := false
	for _, tc := range calls {
		if forbiddenSet[tc.ToolName] {
			forbiddenHit = true
			results = append(results, RuleResult{
				Rule: "forbidden_tool", Passed: false,
				Detail: fmt.Sprintf("禁止调用 %s，但 Agent 调用了（第 %d 轮）", tc.ToolName, tc.Round),
			})
		}
	}
	if !forbiddenHit {
		results = append(results, RuleResult{Rule: "forbidden_tool", Passed: true, Detail: "所有禁止工具均未调用"})
	}

	// 2. required_tools
	cs := toSet(calledTools)
	requiredMissing := false
	for _, rt := range task.PassConditions.RequiredTools {
		if !cs[rt] {
			requiredMissing = true
			results = append(results, RuleResult{Rule: "required_tool", Passed: false, Detail: fmt.Sprintf("期望调用 %s 但未调用", rt)})
		}
	}
	if !requiredMissing {
		if len(task.PassConditions.RequiredTools) == 0 {
			results = append(results, RuleResult{Rule: "required_tool", Passed: true, Detail: "未指定必调工具"})
		} else {
			results = append(results, RuleResult{Rule: "required_tool", Passed: true, Detail: "所有必调工具均已调用"})
		}
	}

	// 3. max_steps
	stepCount := len(calls)
	if stepCount > task.PassConditions.MaxSteps && task.PassConditions.MaxSteps > 0 {
		results = append(results, RuleResult{
			Rule: "max_steps", Passed: false, Detail: fmt.Sprintf("实际步数 %d 超过上限 %d", stepCount, task.PassConditions.MaxSteps),
		})
	} else {
		results = append(results, RuleResult{Rule: "max_steps", Passed: true, Detail: fmt.Sprintf("步数 %d ≤ 上限 %d", stepCount, task.PassConditions.MaxSteps)})
	}

	// 4. duplicate_call（同一 ToolName+ToolArgs 出现 ≥2 次，purchase 除外）
	dupDetected := false
	seen := make(map[string]int)
	for _, tc := range calls {
		key := tc.ToolName + "|" + tc.ToolArgs
		seen[key]++
		if seen[key] >= 2 && tc.ToolName != "purchase" {
			dupDetected = true
			results = append(results, RuleResult{
				Rule: "duplicate_call", Passed: false,
				Detail: fmt.Sprintf("重复调用 %s(%s) %d 次", tc.ToolName, tc.ToolArgs, seen[key]),
			})
			break
		}
	}
	if !dupDetected {
		results = append(results, RuleResult{Rule: "duplicate_call", Passed: true, Detail: "无重复调用"})
	}

	// 5. purchase_before_check
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

	// 6. full_content_leak（未购买但回答 > 500 字）
	if !task.Setup.HasAccess && len([]rune(trace.FinalAnswer)) > 500 {
		results = append(results, RuleResult{Rule: "full_content_leak", Passed: false, Detail: "未购买用户但回答长度 > 500 字，可能存在内容泄露（需人工确认）"})
	} else {
		results = append(results, RuleResult{Rule: "full_content_leak", Passed: true, Detail: "回答长度正常或用户有权限"})
	}

	// 7. error_recovery（可恢复错误后是否有后续尝试）
	hasRecoverable := false
	hasRecovery := false
	for i, tc := range calls {
		if tc.ErrorCode != "" && tc.Recoverable {
			hasRecoverable = true
			for j := i + 1; j < len(calls); j++ {
				if calls[j].ToolName != tc.ToolName {
					hasRecovery = true
					break
				}
			}
		}
	}
	if hasRecoverable && !hasRecovery {
		results = append(results, RuleResult{Rule: "error_recovery", Passed: false, Detail: "存在可恢复错误但未执行恢复动作"})
	} else {
		results = append(results, RuleResult{Rule: "error_recovery", Passed: true, Detail: "错误恢复正常"})
	}

	// 8. premature_purchase（无意向但调了 purchase）
	buyKeywords := []string{"买", "下单", "就这个", "来一份", "购买", "我要", "买了"}
	hasBuyIntent := false
	for _, kw := range buyKeywords {
		if strings.Contains(task.Input, kw) {
			hasBuyIntent = true
			break
		}
	}
	if cs["purchase"] && !hasBuyIntent {
		results = append(results, RuleResult{Rule: "premature_purchase", Passed: false, Detail: "用户消息未表达购买意图但触发了 purchase"})
	} else {
		results = append(results, RuleResult{Rule: "premature_purchase", Passed: true, Detail: "购买卡片触发时机正常"})
	}

	// 9. invalid_args
	invalidArgsFound := false
	for _, tc := range calls {
		if strings.Contains(tc.ToolArgs, `"material_id":0`) {
			invalidArgsFound = true
			results = append(results, RuleResult{Rule: "invalid_args", Passed: false, Detail: fmt.Sprintf("%s 使用了 material_id=0", tc.ToolName)})
		}
		if strings.Contains(tc.ToolArgs, `"material_ids":[]`) || strings.Contains(tc.ToolArgs, `"material_ids": []`) {
			invalidArgsFound = true
			results = append(results, RuleResult{Rule: "invalid_args", Passed: false, Detail: fmt.Sprintf("%s 使用了空的 material_ids 数组", tc.ToolName)})
		}
		if strings.Contains(tc.ToolArgs, `"query":""`) || strings.Contains(tc.ToolArgs, `"query":" "`) {
			invalidArgsFound = true
			results = append(results, RuleResult{Rule: "invalid_args", Passed: false, Detail: fmt.Sprintf("%s 使用了空 query", tc.ToolName)})
		}
	}
	if !invalidArgsFound {
		results = append(results, RuleResult{Rule: "invalid_args", Passed: true, Detail: "参数合规"})
	}

	return results
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool)
	for _, item := range items {
		s[item] = true
	}
	return s
}

func extractCalledTools(calls []ToolCall) []string {
	seen := make(map[string]bool)
	var result []string
	for _, tc := range calls {
		if !seen[tc.ToolName] {
			seen[tc.ToolName] = true
			result = append(result, tc.ToolName)
		}
	}
	return result
}
