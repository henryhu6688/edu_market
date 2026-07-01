package agent

import (
	"encoding/json"
	"fmt"

	"edu_market/database"
	"edu_market/model"
)

// ============ Level 1：精确重复熔断 ============

// CircuitBreaker 熔断器，防止 LLM 对同一 Tool 反复调用。
// 仅检测紧邻上一轮，跨轮重复由 SemanticLoopDetector 处理。
type CircuitBreaker struct {
	lastToolName string
	lastToolArgs string
}

// Check 检查是否触发 Level 1 精确重复熔断。
// toolName 和 argsJSON 与上一轮完全相同 → blocked=true。
// 如果 Tool 声明了 AllowRepeat()==true（如购买卡片），则跳过检测。
func (cb *CircuitBreaker) Check(tool Tool, toolName, argsJSON string) (blocked bool, reason string) {
	if tool != nil && tool.AllowRepeat() {
		return false, ""
	}
	if toolName == cb.lastToolName && argsJSON == cb.lastToolArgs {
		return true, "重复调用：与上一轮完全相同的工具和参数，请调整策略或直接回答用户"
	}
	return false, ""
}

// Record 记录本轮调用，供下一轮比对。
func (cb *CircuitBreaker) Record(toolName, argsJSON string) {
	cb.lastToolName = toolName
	cb.lastToolArgs = argsJSON
}

// ============ Level 2：语义回路检测 ============

// SemanticLoopDetector 语义回路检测器。
// 保留最近 3 轮 Tool 结果，用 Jaccard 相似度判断是否陷入回路。
type SemanticLoopDetector struct {
	recentResults []string
}

// Feed 将本轮 Tool 结果喂入检测器。
// 保留最近 3 轮结果（各截取 200 字），Jaccard 相似度判定。
func (d *SemanticLoopDetector) Feed(content string) (blocked bool, reason string) {
	truncated := TruncateRunes(content, 200)
	d.recentResults = append(d.recentResults, truncated)
	if len(d.recentResults) < 3 {
		return false, ""
	}
	if len(d.recentResults) > 3 {
		d.recentResults = d.recentResults[1:]
	}

	simAB := jaccard(d.recentResults[0], d.recentResults[1])
	simBC := jaccard(d.recentResults[1], d.recentResults[2])
	simAC := jaccard(d.recentResults[0], d.recentResults[2])

	if simAB > 0.8 && simBC > 0.8 && simAC > 0.8 {
		return true, "检测到语义回路：最近三轮查询结果高度重复，无新信息。请基于现有信息直接回答用户。"
	}
	return false, ""
}

// jaccard 计算两段文本的 Jaccard 相似度（基于 bigram）。
func jaccard(a, b string) float64 {
	setA := extractBigrams(a)
	setB := extractBigrams(b)
	intersection := 0
	for k := range setA {
		if setB[k] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// extractBigrams 提取 bigram 词组集合。
func extractBigrams(text string) map[string]bool {
	runes := []rune(text)
	set := make(map[string]bool)
	for i := 0; i < len(runes)-1; i++ {
		set[string(runes[i:i+2])] = true
	}
	return set
}

// ============ 工具边界：调用预算 ============

// ToolBudget Tool 调用预算。每轮对话每个 Tool 独立计数。
type ToolBudget struct {
	limits map[string]int
	counts map[string]int
}

// NewToolBudget 创建调用预算（各 Tool 上限见 spec 2.2.4）。
func NewToolBudget() *ToolBudget {
	return &ToolBudget{
		limits: map[string]int{
			ToolPurchase:          3,
			ToolSearchDocuments:   10,
			ToolSearchMaterials:   5,
			ToolGetMaterialDetail: 10,
				ToolMyMaterials:       5,
			ToolGetOrders:         5,
			ToolSearchFAQ:         5,
		},
		counts: make(map[string]int),
	}
}

// Spend 消耗一次调用配额，超限返回 error。
func (b *ToolBudget) Spend(toolName string) error {
	if b.counts[toolName] >= b.limits[toolName] {
		return fmt.Errorf("工具 %s 调用次数已达上限（%d次），请基于已有结果回答", toolName, b.limits[toolName])
	}
	b.counts[toolName]++
	return nil
}

// ============ 状态机：模式判定 ============

// ResolveMode 根据本轮实际执行的 Tool 类型判定当前模式。
// 不依赖用户消息语义——只看实际执行了什么 Tool。
// 返回 "shopping" | "tutoring" | "support" | ""（保持当前）。
func ResolveMode(session *model.Session, executedTools []string) string {
	if len(executedTools) == 0 {
		return session.Mode
	}
	for _, t := range executedTools {
		switch {
		case t == ToolGetOrders:
			return "support"
		case t == ToolSearchDocuments || t == ToolGetMaterialDetail:
			if checkHasAccess(session.UserID, getFocusMaterialID(session)) {
				return "tutoring"
			}
			return "shopping"
		case t == ToolPurchase:
			return "shopping"
		// search_materials / my_materials / search_faq 是中立的，不改变模式
		}
	}
	return session.Mode
}

// checkHasAccess 检查用户是否有权访问指定资料的全文内容。
// 两种情形视为有权限：(1) 已购买且支付成功 (2) 用户即资料发布者。
func checkHasAccess(userID, materialID uint) bool {
	if materialID == 0 {
		return false
	}
	// 发布者本人 → 天然有权限
	var mat model.Material
	if database.DB.Select("user_id").First(&mat, materialID).Error == nil && mat.UserID == userID {
		return true
	}
	// 已购买 → 有权限
	var count int64
	database.DB.Model(&model.Order{}).
		Where("user_id = ? AND course_id = ? AND status = ?", userID, materialID, "paid").
		Count(&count)
	return count > 0
}

// getFocusMaterialID 从 Session.State 中提取焦点资料ID。
// 使用原生 JSON 解析避免循环依赖（SessionState 定义在 prompts.go）。
func getFocusMaterialID(session *model.Session) uint {
	if session.State == "" {
		return 0
	}
	var raw struct {
		Context struct {
			FocusID uint `json:"focus_id"`
		} `json:"context"`
	}
	if json.Unmarshal([]byte(session.State), &raw) == nil {
		return raw.Context.FocusID
	}
	return 0
}
