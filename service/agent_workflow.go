package service

import (
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
	consultWords := []string{"有没有", "推荐", "学什么", "入门", "进阶", "哪个好", "想学", "适合我"}

	if matchAny(msgLower, buyWords) {
		return IntentPurchase
	}
	if matchAny(msgLower, afterSaleWords) {
		return IntentAfterSale
	}
	if matchAny(msgLower, consultWords) {
		return IntentConsult
	}

	// 第二层：LLM 快判（关键词没命中时）
	return classifyByLLM(question)
}

// classifyByLLM 用轻量 LLM 调用判断意图
func classifyByLLM(question string) string {
	prompt := fmt.Sprintf(`判断这句话的意图，只回答一个词（purchase/aftersale/consult/chat）：

- purchase: 用户想买资料、下单、付钱
- aftersale: 退款、订单问题、支付问题、投诉
- consult: 咨询资料内容、推荐、学习方向、资料介绍
- chat: 闲聊、问候、无关话题、学习相关的自由提问

用户消息: %s
意图:`, question)

	reqBody := map[string]interface{}{
		"model":    config.App.AI.Model,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
		"stream":   false,
		"max_tokens": 10,
	}

	jsonBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", config.App.AI.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return IntentChat
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return IntentChat
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return IntentChat
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return IntentChat
	}

	if len(result.Choices) == 0 {
		return IntentChat
	}

	answer := strings.TrimSpace(strings.ToLower(result.Choices[0].Message.Content))

	// 解析 LLM 返回的意图词
	if strings.Contains(answer, "purchase") || strings.Contains(answer, "购买") {
		return IntentPurchase
	}
	if strings.Contains(answer, "aftersale") || strings.Contains(answer, "售后") {
		return IntentAfterSale
	}
	if strings.Contains(answer, "consult") || strings.Contains(answer, "咨询") {
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
