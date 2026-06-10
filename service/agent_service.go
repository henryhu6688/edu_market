package service

import (
	"errors"
	"fmt"

	"edu_market/database"
	"edu_market/model"

	"gorm.io/gorm"
)

// AgentService Agent 总调度服务
type AgentService struct {
	engine *AgentEngine
}

// NewAgentService 创建 AgentService
func NewAgentService(engine *AgentEngine) *AgentService {
	return &AgentService{engine: engine}
}

// Chat 发起/继续 Agent 对话
func (s *AgentService) Chat(userID uint, sessionID *uint, question string, searchFunc SearchFunc, sseHandler SSEHandler) (*model.Session, error) {
	// 1. 获取或创建 Session
	var session *model.Session
	if sessionID != nil {
		session = &model.Session{}
		if err := database.DB.Where("id = ? AND user_id = ?", *sessionID, userID).First(session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("会话不存在")
			}
			return nil, err
		}
	} else {
		// 新会话：先路由，再创建
		agentType, _ := RouteIntent(question)
		if agentType == "" {
			agentType = model.AgentCustomerService // 默认客服
		}
		session = &model.Session{
			UserID:    userID,
			AgentType: agentType,
			Status:    model.SessionActive,
			Title:     truncateRunes(question, 30),
		}
		if err := database.DB.Create(session).Error; err != nil {
			return nil, fmt.Errorf("创建会话失败: %w", err)
		}
	}

	// 2. 构建 Prompt + Tools
	systemPrompt := GetAgentPrompt(session.AgentType)
	tools := buildToolSet(session.AgentType, searchFunc)

	// 3. 运行引擎
	if err := s.engine.Run(session, question, tools, systemPrompt, sseHandler); err != nil {
		return session, err
	}

	// 4. 检测 Agent 切换标记
	s.checkTransfer(session)

	return session, nil
}

// GetSessions 获取用户会话列表
func (s *AgentService) GetSessions(userID uint, page, pageSize int) ([]model.Session, int64, error) {
	page, pageSize = GetPagination(page, pageSize)
	var sessions []model.Session
	var total int64

	database.DB.Model(&model.Session{}).Where("user_id = ?", userID).Count(&total)
	if err := database.DB.Where("user_id = ?", userID).
		Order("updated_at DESC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&sessions).Error; err != nil {
		return nil, 0, err
	}
	return sessions, total, nil
}

// DeleteSession 关闭会话（软状态变更）
func (s *AgentService) DeleteSession(userID, sessionID uint) error {
	result := database.DB.Model(&model.Session{}).
		Where("id = ? AND user_id = ?", sessionID, userID).
		Update("status", model.SessionClosed)
	if result.RowsAffected == 0 {
		return errors.New("会话不存在")
	}
	return result.Error
}

// GetMessages 获取会话消息历史
func (s *AgentService) GetMessages(sessionID, userID uint, page, pageSize int) ([]model.Message, int64, error) {
	page, pageSize = GetPagination(page, pageSize)

	// 验权：session 属于该用户
	var session model.Session
	if err := database.DB.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		return nil, 0, errors.New("会话不存在")
	}

	var messages []model.Message
	var total int64

	database.DB.Model(&model.Message{}).Where("session_id = ?", sessionID).Count(&total)
	if err := database.DB.Where("session_id = ?", sessionID).
		Order("id ASC").Offset((page-1)*pageSize).Limit(pageSize).
		Find(&messages).Error; err != nil {
		return nil, 0, err
	}
	return messages, total, nil
}

// checkTransfer 检测最后一条 assistant 回答是否有切换标记
func (s *AgentService) checkTransfer(session *model.Session) {
	var msg model.Message
	if err := database.DB.Where("session_id = ? AND role = ?", session.ID, model.RoleAssistant).
		Order("id DESC").First(&msg).Error; err != nil {
		return
	}
	if should, targetAgent := DetectTransfer(msg.Content); should {
		// 清理回答中的切换标记
		cleaned := CleanTransferMarkers(msg.Content)
		database.DB.Model(&msg).Update("content", cleaned)
		// 更新 session 的 agent_type
		database.DB.Model(session).Update("agent_type", targetAgent)
	}
}

// truncateRunes 截取字符串前 n 个字符（Unicode 安全）
func truncateRunes(s string, maxLen int) string {
	// 去掉切换标记再截取
	s = CleanTransferMarkers(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
