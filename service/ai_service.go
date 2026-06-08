package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"edu-market/config"
	"edu-market/database"
	"edu-market/model"
)

// AIService AI 对话服务
type AIService struct{}

// ChatRequest AI API 请求体
type chatRequest struct {
	Model    string       `json:"model"`
	Messages []chatMsg    `json:"messages"`
	Stream   bool         `json:"stream"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse AI API 响应体
type chatResponse struct {
	ID      string    `json:"id"`
	Choices []choice  `json:"choices"`
	Usage   usageInfo `json:"usage"`
}

type choice struct {
	Message chatMsg `json:"message"`
}

type usageInfo struct {
	TotalTokens int `json:"total_tokens"`
}

// Chat 发起 AI 对话
func (s *AIService) Chat(userID uint, question string) (*model.Conversation, error) {
	// 构建请求
	reqBody := chatRequest{
		Model:  config.App.AI.Model,
		Stream: false,
		Messages: []chatMsg{
			{Role: "system", Content: "你是一个学习助手，帮助用户解答学习相关的问题，提供专业、准确、易懂的回答。"},
			{Role: "user", Content: question},
		},
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.New("请求编码失败")
	}

	// 发送 HTTP 请求
	req, err := http.NewRequest("POST", config.App.AI.APIURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, errors.New("创建请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("AI服务请求失败: " + err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("读取AI响应失败")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("AI服务返回错误: " + string(body))
	}

	var aiResp chatResponse
	if err := json.Unmarshal(body, &aiResp); err != nil {
		return nil, errors.New("解析AI响应失败")
	}

	if len(aiResp.Choices) == 0 {
		return nil, errors.New("AI未返回有效回答")
	}

	// 保存对话记录
	conv := &model.Conversation{
		UserID:     userID,
		Question:   question,
		Answer:     aiResp.Choices[0].Message.Content,
		Model:      config.App.AI.Model,
		TokensUsed: aiResp.Usage.TotalTokens,
	}

	if err := database.DB.Create(conv).Error; err != nil {
		return nil, errors.New("保存对话记录失败")
	}

	return conv, nil
}

// GetHistory 获取对话历史
func (s *AIService) GetHistory(userID uint, page, pageSize int) ([]model.Conversation, int64, error) {
	var conversations []model.Conversation
	var total int64

	page, pageSize = GetPagination(page, pageSize)

	database.DB.Model(&model.Conversation{}).Where("user_id = ?", userID).Count(&total)
	if err := database.DB.Where("user_id = ?", userID).
		Offset((page - 1) * pageSize).Limit(pageSize).
		Order("created_at DESC").Find(&conversations).Error; err != nil {
		return nil, 0, err
	}

	return conversations, total, nil
}
