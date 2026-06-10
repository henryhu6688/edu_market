package service

import (
	"testing"

	"edu_market/database"
	"edu_market/model"
)

func setupAgentServiceTest(t *testing.T) (*model.User, *AgentService) {
	t.Helper()
	database.DB.Where("1=1").Delete(&model.Message{})
	database.DB.Where("1=1").Delete(&model.Session{})
	database.DB.Where("username LIKE ?", "svc_test_%").Delete(&model.User{})

	engine := NewAgentEngine()
	svc := NewAgentService(engine)

	username := "svc_test_" + t.Name()
	user := &model.User{Username: username, Role: "student"}
	if err := database.DB.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user, svc
}

func TestAgentService_GetSessions(t *testing.T) {
	user, svc := setupAgentServiceTest(t)

	for i := 0; i < 2; i++ {
		s := &model.Session{UserID: user.ID, AgentType: model.AgentCustomerService, Status: model.SessionActive, Title: "测试"}
		database.DB.Create(s)
	}

	sessions, total, err := svc.GetSessions(user.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetSessions 失败: %v", err)
	}
	if total < 2 {
		t.Errorf("total = %d, want >= 2", total)
	}
	if len(sessions) < 2 {
		t.Errorf("sessions count = %d, want >= 2", len(sessions))
	}
}

func TestAgentService_DeleteSession(t *testing.T) {
	user, svc := setupAgentServiceTest(t)

	s := &model.Session{UserID: user.ID, AgentType: model.AgentCustomerService, Status: model.SessionActive, Title: "删除测试"}
	database.DB.Create(s)

	if err := svc.DeleteSession(user.ID, s.ID); err != nil {
		t.Fatalf("DeleteSession 失败: %v", err)
	}

	var updated model.Session
	database.DB.First(&updated, s.ID)
	if updated.Status != model.SessionClosed {
		t.Errorf("status = %s, want %s", updated.Status, model.SessionClosed)
	}
}

func TestAgentService_DeleteSession_NotFound(t *testing.T) {
	user, svc := setupAgentServiceTest(t)

	err := svc.DeleteSession(user.ID, 99999)
	if err == nil {
		t.Error("删除不存在的会话应返回错误")
	}
}

func TestAgentService_DeleteSession_WrongUser(t *testing.T) {
	user, svc := setupAgentServiceTest(t)

	s := &model.Session{UserID: user.ID, AgentType: model.AgentCustomerService, Status: model.SessionActive, Title: "权限测试"}
	database.DB.Create(s)

	err := svc.DeleteSession(user.ID+999, s.ID)
	if err == nil {
		t.Error("删除其他用户的会话应返回错误")
	}
}

func TestAgentService_GetMessages(t *testing.T) {
	user, svc := setupAgentServiceTest(t)

	s := &model.Session{UserID: user.ID, AgentType: model.AgentCustomerService, Status: model.SessionActive, Title: "消息测试"}
	database.DB.Create(s)

	msgs := []*model.Message{
		{SessionID: s.ID, Role: model.RoleUser, Content: "你好"},
		{SessionID: s.ID, Role: model.RoleAssistant, Content: "你好！"},
	}
	for _, m := range msgs {
		database.DB.Create(m)
	}

	messages, total, err := svc.GetMessages(s.ID, user.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetMessages 失败: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(messages) != 2 {
		t.Errorf("messages count = %d, want 2", len(messages))
	}
}

func TestAgentService_GetMessages_WrongUser(t *testing.T) {
	user, svc := setupAgentServiceTest(t)

	s := &model.Session{UserID: user.ID, AgentType: model.AgentCustomerService, Status: model.SessionActive, Title: "权限测试"}
	database.DB.Create(s)

	_, _, err := svc.GetMessages(s.ID, user.ID+999, 1, 10)
	if err == nil {
		t.Error("查询其他用户的会话消息应返回错误")
	}
}
