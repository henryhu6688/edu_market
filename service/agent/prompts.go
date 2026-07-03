package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"edu_market/database"
	"edu_market/model"
)

// ============ Session State 数据结构 ============

// SessionState 会话任务状态快照。
// 由引擎在每轮 Tool 执行后自动更新，注入 Prompt State Block 中通知 LLM 当前进度。
type SessionState struct {
	Task        string       `json:"task"`        // 当前任务描述
	Completed   []StepRecord `json:"completed"`   // 已完成步骤
	Failed      []StepRecord `json:"failed"`      // 失败步骤
	Gaps        []string     `json:"gaps"`        // 当前信息缺口
	Deliverable bool         `json:"deliverable"` // 是否可以交付
	ToDo        []string     `json:"to_do"`       // 待完成步骤
	Facts       []FactItem   `json:"facts"`       // 事实层
	Hypotheses  []FactItem   `json:"hypotheses"`  // 假设层
	Discarded   []FactItem   `json:"discarded"`   // 废弃层
	Context     ContextData  `json:"context"`     // 业务上下文
}

// ContextData 业务上下文数据。
type ContextData struct {
	UserHasAccess  bool        `json:"user_has_access"`        // 用户是否有资料访问权（由 Tool 结果写入，非 focus_id 推测）
	Candidates     []Candidate `json:"candidates,omitempty"`   // 候选资料列表
	FocusID        uint        `json:"focus_id"`               // 当前焦点资料ID
	CardSent       bool        `json:"card_sent"`              // 是否已发送购买卡片
	MaterialsViewed []uint     `json:"materials_viewed"`       // 浏览过的资料ID
}

// Candidate 候选资料。
type Candidate struct {
	ID    uint    `json:"id"`
	Title string  `json:"title"`
	Price float64 `json:"price"`
}

// FactItem 上下文分层中的一条数据项。
// 三层共用此结构，Basis 为可选扩展字段。
type FactItem struct {
	Content string `json:"content"`               // 数据内容（自然语言，截取 150 字以内）
	Source  string `json:"source"`                // 来源标识（tool名+参数 或 rag:chunk_id 或 user:said 或 llm:inferred）
	Basis   string `json:"basis,omitempty"`       // hypothesis：confidence值；discarded：废弃原因
}

// ============ 6 模块 Prompt Block ============

// basePersonaBlock 基础角色定义
const basePersonaBlock = `你是 edu_market 的智能助手，负责三件事：
- 导购：帮用户找到合适的学习资料并完成购买
- 助教：用已购资料的文档内容解答用户疑问
- 客服：处理订单查询、退款、售后问题

当前处于「{{mode}}」模式，请专注当前模式的任务。`

// shoppingModeBlock 导购模式策略
const shoppingModeBlock = `【导购模式】
你是书店导购。

策略：
  1. 用户没说方向 → 先问需求（方向/水平/预算），别直接搜
  2. 用户说了方向 → search_materials 搜索 → 挑 1-2 个最匹配的推荐
  3. 用户对某资料感兴趣 → get_material_detail（含目录、评价、访问权限）
  4. 用户表达购买意向 → purchase
  5. 发了卡片后 → 等用户决策，不强推其他资料
  6. 用户想了解内容但没买 → search_documents（系统自动限制返回preview），基于介绍引导购买

禁止：
  - 一上来就甩资料列表
  - 用户拒绝后继续推销同一资料
  - 深入讲解文档内容（告知买后可详细讲解即可）
  - purchase 是发卡唯一方式，不调 = 用户看不到卡片
  - 说了"已发送卡片"但没调 purchase → 立即补调`

// tutoringModeBlock 助教模式策略
const tutoringModeBlock = `【助教模式】
用户已拥有当前资料的完整访问权（已购买或为发布者）。你是课程助教，用资料全文内容详细回答问题。

策略：
  1. 用户提到资料名/章节 → get_material_detail 确认
  2. 用户问知识点 → search_documents 检索
  3. 用文档原文详细回答，注明章节来源，不用有所保留
  4. 搜不到 → 诚实地告知"资料中没有涉及该内容"
  5. 禁止提及"购买""付费""试读"等概念——用户已有完整访问权

禁止：
  - 建议用户购买或提示未购买
  - 答疑时推荐其他资料（除非用户主动问）
  - 编造文档中没有的内容
  - 管订单和退款`

// supportModeBlock 客服模式策略
const supportModeBlock = `【客服模式】
你是平台客服，用订单数据和 FAQ 处理售后。

策略：
  1. 订单相关问题 → get_orders（不传 order_no=列表，传了=详情）
  2. 查 FAQ → search_faq
  3. FAQ 没有 → "这个问题需要转接人工客服处理"

禁止：
  - 推荐资料
  - 回答课程内容
  - 承诺"可以退款""随时退"（FAQ 明确写了才可以说）
  - FAQ 没写的 → "建议联系客服确认"`

// undeterminedModeBlock 识别模式（首轮专用）
// 此时系统尚未确定用户意图和访问权限，不做购买/不购买的假设。
const undeterminedModeBlock = `【识别模式】
你是平台助手。当前尚未确定用户意图和访问权限——系统会根据你调用的工具自动判断。

策略：
  1. 先理解用户在问什么——是找资料、问内容、还是查订单？
  2. 用户问资料/学习内容：
	   - 找有没有某方向的课（「有没有Python的」）→ search_materials
	   - 我的资料（「我买了什么」「我发了哪些」）→ my_materials
	   - 问知识点/概念/技术问题（「闭包怎么用」「函数定义」）→ search_documents
  3. 用户问订单/售后 → get_orders / search_faq
  4. 不要做任何购买/不购买的假设——系统会自动判断用户权限并切换模式

禁止：
  - 在系统确定模式前建议购买或假设用户未购买
  - 在系统确定模式前假设用户已拥有访问权
  - 发送购买卡片（purchase）`

// rulesBlock 不可违反的核心规则
const rulesBlock = `【核心规则 - 不可违反】

1. 数据准确性
   - 资料价格、章数、评分以【事实层】数据为准
   - 【事实层】没有的数据不要编造
   - 搜到的内容带可信度（高/中/低），优先引用"高"，"中""低"不声称原文

2. 工具使用
   - 同一 tool 同一参数不重复调
   - 连续 2 次无结果 → 换策略
   - 返回"调用次数上限""重复调用" → 立即基于现有信息回答

3. 边界兜底
   - 不确定的政策不说"可以""支持""保证"
   - 退款/售后 → FAQ 有就用，没有就引导客服
   - 超出能力 → "这个我帮不了，建议联系客服"
   - 搜不到 → 诚实说

4. 用户关系
   - 不替用户做购买决策
   - 用户拒绝后不纠缠

【回答规则 - 引用约束】
1. 只基于上面【参考资料】回答，资料中没有的内容必须说"资料中未涉及"
2. 每条论断必须标注来源：[《文档名》> 章节]
3. 禁止使用你自身的参数知识补充资料中没有的内容
4. 参考资料中可信度为"低"的只能作为参考，不能作为主要依据
5. 无法回答时直接说"这个问题在资料中没有找到相关内容"，不要猜测`

// styleBlock 回复格式要求
const styleBlock = `【回复格式】

1. 长度：不超过 3 段，每段不超过 3 句
2. 推荐资料：
   推荐《资料名》- ¥XX.XX
   亮点：一句话概括
   （挑 1-2 个最匹配的）
3. 解释知识：一句话结论 → 展开 → 注明章节
4. 不用：打招呼、客套话、emoji
5. 兜底："抱歉，这个问题建议联系平台客服处理。"`

// ============ Prompt 拼装 ============

// buildPrompt 按 6 模块顺序拼装 System Prompt。
// 1. Base Persona → 2. Mode Block → 3. State Block → 4. User Context → 5. Rules → 6. Style
func (e *AgentEngine) buildPrompt(session *model.Session) string {
	var parts []string

	// 1. Base Persona
	parts = append(parts, basePersonaBlock)

	// 2. Mode Block
	modeName := e.appendModeBlock(&parts, session.Mode)

	// 3. State Block
	if session.State != "" {
		var state SessionState
		if json.Unmarshal([]byte(session.State), &state) == nil {
			parts = append(parts, buildStateBlock(&state))
		}
	}

	// 4. User Context（按 mode 筛选注入字段）
	userBlock := buildUserContextBlock(session.UserID, session.Mode)
	if userBlock != "" {
		parts = append(parts, userBlock)
	}

	// 5. Rules
	parts = append(parts, rulesBlock)

	// 6. Style
	parts = append(parts, styleBlock)

	result := strings.Join(parts, "\n\n")
	result = strings.ReplaceAll(result, "{{mode}}", modeName)
	return result
}

// appendModeBlock 根据 mode 追加对应的 Mode Block，返回中文模式名。
func (e *AgentEngine) appendModeBlock(parts *[]string, mode string) string {
	switch mode {
	case "shopping":
		*parts = append(*parts, shoppingModeBlock)
		return "导购"
	case "tutoring":
		*parts = append(*parts, tutoringModeBlock)
		return "学习助教"
	case "support":
		*parts = append(*parts, supportModeBlock)
		return "客服"
	default: // mode="" 第一轮：识别模式，不做权限假设
		*parts = append(*parts, undeterminedModeBlock)
		return "识别"
	}
}

// buildStateBlock 从 SessionState 生成 State Block 文本，含治理信号。
func buildStateBlock(state *SessionState) string {
	var sb strings.Builder

	sb.WriteString("/* ── 任务状态 ── */\n")
	sb.WriteString(fmt.Sprintf("当前目标: %s\n", state.Task))

	if len(state.Completed) > 0 {
		sb.WriteString("已完成:\n")
		for _, r := range state.Completed {
			sb.WriteString(fmt.Sprintf("  ✅ %s\n", r.Action))
		}
	}
	if len(state.Failed) > 0 {
		sb.WriteString("已失败:\n")
		for _, r := range state.Failed {
			sb.WriteString(fmt.Sprintf("  ❌ %s → %s\n", r.Action, r.Error))
		}
	}
	if len(state.Gaps) > 0 {
		sb.WriteString("缺口:\n")
		for _, g := range state.Gaps {
			sb.WriteString(fmt.Sprintf("  ⚠️ %s\n", g))
		}
	}
	label := "否 → 继续获取信息"
	if state.Deliverable {
		label = "是 → 立即回答用户"
	}
	sb.WriteString(fmt.Sprintf("可以交付: %s\n", label))

	if len(state.ToDo) > 0 {
		sb.WriteString("待办:\n")
		for _, s := range state.ToDo {
			sb.WriteString(fmt.Sprintf("  ⬜ %s\n", s))
		}
	}
	if len(state.Facts) > 0 {
		sb.WriteString("\n/* ── 事实层 ── */\n")
		for _, f := range state.Facts {
			sb.WriteString(fmt.Sprintf("  📖 %s | 来源：%s\n", f.Content, f.Source))
		}
	}
	if len(state.Hypotheses) > 0 {
		sb.WriteString("/* ── 假设层 ── */\n")
		for _, h := range state.Hypotheses {
			sb.WriteString(fmt.Sprintf("  💡 %s | 来源：%s\n", h.Content, h.Source))
		}
	}
	if len(state.Discarded) > 0 {
		sb.WriteString("/* ── 废弃层 ── */\n")
		for _, d := range state.Discarded {
			sb.WriteString(fmt.Sprintf("  🗑️ %s | 原因：%s\n", d.Content, d.Basis))
		}
	}
	return sb.String()
}

// assessState 每轮 Tool 执行后校验任务状态，标注缺口和交付条件。
// 引擎不替代 LLM 做决策——只标注位置信息，LLM 据此自主规划。
func (e *AgentEngine) assessState(state *SessionState) {
	hasCompleted := len(state.Completed) > 0
	hasOpenFailures := false
	for _, f := range state.Failed {
		if f.Error != "" {
			hasOpenFailures = true
			break
		}
	}
	state.Deliverable = hasCompleted && len(state.Gaps) == 0 && !hasOpenFailures
}

// buildUserContextBlock 从 user_memories 表加载用户画像，按 mode 筛选注入字段。
func buildUserContextBlock(userID uint, mode string) string {
	var memories []model.UserMemory
	database.DB.Where("user_id = ? AND status = 'active'", userID).Find(&memories)
	if len(memories) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "【用户画像】")

	for _, m := range memories {
		switch mode {
		case "tutoring":
			if m.MemKey != "knowledge_level" && m.MemKey != "purchased_material_ids" {
				continue
			}
		case "support":
			if m.MemKey != "purchased_material_ids" {
				continue
			}
		} // shopping: 全量注入

		lines = append(lines, formatMemoryLine(m))
	}
	return strings.Join(lines, "\n")
}

// formatMemoryLine 格式化单条记忆为提示行。
func formatMemoryLine(m model.UserMemory) string {
	switch m.MemKey {
	case "knowledge_level":
		return fmt.Sprintf("  水平：%s（可信度：%.0f%%）", m.MemValue, m.Confidence*100)
	case "interest_tags":
		return fmt.Sprintf("  兴趣：%s（可信度：%.0f%%）", m.MemValue, m.Confidence*100)
	case "preferred_price_range":
		return fmt.Sprintf("  预算偏好：%s（可信度：%.0f%%）", m.MemValue, m.Confidence*100)
	case "purchased_material_ids":
		return fmt.Sprintf("  已购资料ID：%s", m.MemValue)
	}
	return fmt.Sprintf("  %s: %s", m.MemKey, m.MemValue)
}
