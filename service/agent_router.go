package service

import (
	"strings"

	"edu_market/model"
)

// routeKeywords 关键词路由表
var routeKeywords = map[string][]string{
	model.AgentCustomerService: {
		"退款", "订单", "支付失败", "怎么买", "申诉", "客服", "联系", "投诉",
		"价格", "优惠券", "发货", "物流", "退货",
	},
	model.AgentCourseRecommend: {
		"推荐", "有什么课", "适合我", "哪个好", "入门", "进阶", "有没有",
		"零基础", "学什么", "想学", "选课", "哪个课程", "对比",
	},
	model.AgentQA: {
		"这个公式", "第三章", "解释一下", "为什么", "怎么做", "详细讲讲",
		"讲义", "课件", "这里为什么", "推导", "证明", "怎么理解",
	},
}

// RouteIntent 根据用户消息路由到对应的 Agent 类型
// 返回 agentType 和是否需要 LLM 二次判断
func RouteIntent(message string) (agentType string, needLLM bool) {
	msgLower := strings.ToLower(message)

	// 1. 统计关键词命中数
	scores := map[string]int{
		model.AgentCustomerService:  0,
		model.AgentCourseRecommend:  0,
		model.AgentQA:               0,
	}

	for agentType, keywords := range routeKeywords {
		for _, kw := range keywords {
			if strings.Contains(msgLower, kw) {
				scores[agentType]++
			}
		}
	}

	// 2. 确定最高分
	maxScore := 0
	maxAgent := ""
	for agentType, score := range scores {
		if score > maxScore {
			maxScore = score
			maxAgent = agentType
		}
	}

	// 3. 如果有明确最高分 → 直接路由
	if maxScore > 0 {
		// 检查是否有并列
		for agentType, score := range scores {
			if score == maxScore && agentType != maxAgent {
				return "", true // 并列 → LLM 判断
			}
		}
		return maxAgent, false
	}

	// 4. 无命中 → 需要 LLM 判断
	return "", true
}

// DetectTransfer 检测 LLM 回答中的 Agent 切换标记
func DetectTransfer(answer string) (shouldTransfer bool, targetAgent string) {
	if strings.Contains(answer, "[TRANSFER:qa]") {
		return true, model.AgentQA
	}
	if strings.Contains(answer, "[TRANSFER:course_recommend]") {
		return true, model.AgentCourseRecommend
	}
	if strings.Contains(answer, "[TRANSFER:customer_service]") {
		return true, model.AgentCustomerService
	}
	return false, ""
}

// CleanTransferMarkers 清除回答中的切换标记，返回干净的文本
func CleanTransferMarkers(answer string) string {
	answer = strings.ReplaceAll(answer, "[TRANSFER:qa]", "")
	answer = strings.ReplaceAll(answer, "[TRANSFER:course_recommend]", "")
	answer = strings.ReplaceAll(answer, "[TRANSFER:customer_service]", "")
	return strings.TrimSpace(answer)
}

// GetAgentPrompt 根据 Agent 类型返回对应的 System Prompt
func GetAgentPrompt(agentType string) string {
	switch agentType {
	case model.AgentCustomerService:
		return SystemPromptCustomerService
	case model.AgentCourseRecommend:
		return SystemPromptCourseRecommend
	case model.AgentQA:
		return SystemPromptQA
	default:
		return SystemPromptCustomerService
	}
}
