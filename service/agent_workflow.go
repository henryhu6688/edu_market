package service

import (
	"strings"

	"edu_market/database"
	"edu_market/model"
)

// Intent 意图类型
const (
	IntentPurchase  = "purchase"
	IntentAfterSale = "aftersale"
	IntentConsult   = "consult"
	IntentChat      = "chat"
)

// ClassifyIntent 意图分类（关键词优先 → LLM 快判兜底）
func ClassifyIntent(question string) string {
	msgLower := strings.ToLower(question)

	// 第一层：关键词精确匹配（只保留最明确的词组）
	buyWords := []string{"我要买", "帮我买", "帮我下单", "想买", "买哪个", "买这个", "下单"}
	afterSaleWords := []string{"我要退款", "申请退款", "申请退", "支付失败", "我的订单", "投诉", "申诉", "退款", "退货", "发货"}
	consultWords := []string{"有没有", "推荐", "学什么", "入门", "进阶", "哪个好", "想学", "适合我", "感兴趣", "介绍一下", "了解", "看看", "有什么", "资料"}

	if matchAny(msgLower, buyWords) {
		return IntentPurchase
	}
	if matchAny(msgLower, afterSaleWords) {
		return IntentAfterSale
	}
	if matchAny(msgLower, consultWords) {
		return IntentConsult
	}

	// 关键词不命中 → 让 Agent 自己判断，不再二阶段 LLM
	return ""
}

// CheckPurchaseStatus 判断是否已购买（Workflow 固定——代码查库，不靠 LLM）
func CheckPurchaseStatus(userID, materialID uint) bool {
	var count int64
	database.DB.Model(&model.Order{}).
		Where("user_id = ? AND course_id = ? AND status = ?", userID, materialID, "paid").
		Count(&count)
	return count > 0
}

func matchAny(msg string, words []string) bool {
	for _, w := range words {
		if strings.Contains(msg, w) {
			return true
		}
	}
	return false
}
