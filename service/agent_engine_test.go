package service

import (
	"testing"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// setupEngineTest 创建测试用户和会话
func setupEngineTest(t *testing.T) (*model.User, *model.Session) {
	t.Helper()
	user := &model.User{Username: "engine_test_user", Role: "student"}
	if err := database.DB.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	session := &model.Session{
		UserID:    user.ID,
		AgentType: model.AgentCustomerService,
		Status:    model.SessionActive,
		Title:     "测试会话",
	}
	if err := database.DB.Create(session).Error; err != nil {
		t.Fatalf("创建测试会话失败: %v", err)
	}
	return user, session
}

func TestEngineConfig(t *testing.T) {
	engine := NewAgentEngine()
	if engine.maxRounds <= 0 {
		t.Error("maxRounds 应大于 0")
	}
	if engine.contextLimit <= 0 {
		t.Error("contextLimit 应大于 0")
	}
}

func TestEngineContext(t *testing.T) {
	cfg := config.App.Agent
	if cfg.ContextMaxMsg <= 0 {
		t.Skip("Agent 配置未加载")
	}
	engine := NewAgentEngine()
	if engine.contextLimit != cfg.ContextMaxMsg {
		t.Errorf("contextLimit = %d, want %d", engine.contextLimit, cfg.ContextMaxMsg)
	}
}

func TestEngineMaxRounds(t *testing.T) {
	engine := NewAgentEngine()
	if engine.maxRounds != config.App.Agent.MaxToolRounds {
		t.Errorf("maxRounds = %d, want %d", engine.maxRounds, config.App.Agent.MaxToolRounds)
	}
}

func TestEngineLoadContext(t *testing.T) {
	user, session := setupEngineTest(t)

	// 创建测试消息
	msgs := []*model.Message{
		{SessionID: session.ID, Role: model.RoleUser, Content: "你好"},
		{SessionID: session.ID, Role: model.RoleAssistant, Content: "你好！有什么可以帮你的？"},
	}
	for _, m := range msgs {
		if err := database.DB.Create(m).Error; err != nil {
			t.Fatalf("创建测试消息失败: %v", err)
		}
	}

	engine := NewAgentEngine()
	history := engine.loadContext(session.ID, "你是一个测试助手")

	if len(history) < 2 {
		t.Errorf("loadContext 返回消息数 = %d, 期望 >= 2", len(history))
	}
	if history[0].Role != "system" {
		t.Errorf("第一条消息 role = %s, 期望 system", history[0].Role)
	}
	if history[0].Content != "你是一个测试助手" {
		t.Errorf("system prompt = %s, 期望 '你是一个测试助手'", history[0].Content)
	}

	// 清理
	database.DB.Where("session_id = ?", session.ID).Delete(&model.Message{})
	database.DB.Delete(session)
	database.DB.Delete(user)
}
