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

// ClassifyIntent 意图分类（Workflow 固定步骤）
func ClassifyIntent(question string) string {
	msgLower := strings.ToLower(question)

	buyWords := []string{"买", "购买", "下单", "怎么收费", "多少钱"}
	afterSaleWords := []string{"退款", "订单", "支付失败", "投诉", "申诉", "退", "付", "发货"}
	consultWords := []string{"有没有", "内容", "目录", "讲什么", "适合", "推荐", "学什么", "资料", "课程", "哪", "入门", "进阶", "可以", "好"}

	if matchAny(msgLower, buyWords) {
		return IntentPurchase
	}
	if matchAny(msgLower, afterSaleWords) {
		return IntentAfterSale
	}
	if matchAny(msgLower, consultWords) {
		return IntentConsult
	}
	return IntentChat
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
