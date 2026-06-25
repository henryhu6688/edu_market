package agent

import (
	"fmt"
	"strings"

	"edu_market/database"
	"edu_market/model"
)

// allowedKeys L3 Key 白名单。
var allowedKeys = map[string]bool{
	"knowledge_level":        true,
	"interest_tags":          true,
	"preferred_price_range":  true,
	"purchased_material_ids": true,
}

// loadUserMemories 加载用户所有活跃长期记忆。
func loadUserMemories(userID uint) []model.UserMemory {
	var memories []model.UserMemory
	database.DB.Where("user_id = ? AND status = 'active'", userID).Find(&memories)
	return memories
}

// SaveUserMemory 写入一条长期记忆到 L3，自动过筛。
// 白名单校验 → 值格式校验 → 去重+冲突合并 → 写入。
func SaveUserMemory(userID uint, key, value, source string, confidence float64) error {
	// 1. Key 白名单
	if !allowedKeys[key] {
		return nil
	}

	// 2. Value 校验
	if err := validateMemoryValue(key, value); err != nil {
		return err
	}

	// 3. 去重 + 冲突合并
	var existing model.UserMemory
	result := database.DB.Where("user_id = ? AND mem_key = ?", userID, key).First(&existing)
	if result.Error == nil {
		// 旧值 confidence 更高 → 不覆盖
		if existing.Confidence > confidence {
			return nil
		}
		// 更新
		return database.DB.Model(&existing).Updates(map[string]interface{}{
			"mem_value": value, "source": source, "confidence": confidence, "status": "active",
		}).Error
	}

	// 4. 新写入
	return database.DB.Create(&model.UserMemory{
		UserID: userID, MemKey: key, MemValue: value,
		Source: source, Confidence: confidence, Status: "active",
	}).Error
}

// validateMemoryValue 校验记忆值的格式合法性。
func validateMemoryValue(key, value string) error {
	switch key {
	case "knowledge_level":
		if value != "beginner" && value != "intermediate" && value != "advanced" {
			return fmt.Errorf("invalid knowledge_level: %s", value)
		}
	}
	return nil
}

// computeToDo 根据模式、已完成步骤、业务上下文计算待办事项列表。
func computeToDo(mode string, completed []string, ctx ContextData) []string {
	var todos []string
	switch mode {
	case "shopping":
		if !containsStr(completed, "发购买卡片") && ctx.FocusID > 0 {
			todos = append(todos, "询问用户是否购买 → 调 trigger_purchase_offer")
		}
		if !containsStr(completed, "查看详情") && ctx.FocusID == 0 && len(ctx.Candidates) > 0 {
			todos = append(todos, "引导用户查看具体资料详情")
		}
	case "support":
		if !containsStr(completed, "查FAQ") {
			todos = append(todos, "如当前信息无法解决，查 search_faq")
		}
	}
	return todos
}

// containsStr 检查字符串切片中是否有字符串包含指定子串。
func containsStr(slice []string, substr string) bool {
	for _, s := range slice {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
