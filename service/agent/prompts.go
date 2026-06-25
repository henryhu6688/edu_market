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
	Task       string       `json:"task"`       // 当前任务描述
	Completed  []string     `json:"completed"`  // 已完成步骤
	ToDo       []string     `json:"to_do"`      // 待完成步骤
	Facts      []FactItem   `json:"facts"`      // 事实层（Tool 返回、RAG 精确匹配）
	Hypotheses []FactItem   `json:"hypotheses"` // 假设层（RAG 模糊匹配、LLM 推测）
	Discarded  []FactItem   `json:"discarded"`  // 废弃层（被覆盖的旧数据）
	Context    ContextData  `json:"context"`    // 业务上下文
}

// ContextData 业务上下文数据。
type ContextData struct {
	Candidates      []Candidate `json:"candidates,omitempty"`   // 候选资料列表
	FocusID         uint        `json:"focus_id"`               // 当前焦点资料ID
	CardSent        bool        `json:"card_sent"`             // 是否已发送购买卡片
	MaterialsViewed []uint      `json:"materials_viewed"`      // 浏览过的资料ID
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

// SystemPromptV3 旧版统一 Prompt（Task 10 重写 Run() 后由 buildPrompt 替代）
const SystemPromptV3 = `你是 edu_market 的学习导购 + 课程助教 + 客服。

【你的角色】
- 像书店导购：用户想找资料，先问需求再推荐，别一上来甩列表。搜完主动问要不要买
- 像课程助教：用户问文档内容，先确认买没买。买前只答目录+概括，买后详细讲
- 像客服：用户问订单/退款，先查订单再给方案

【关键示例】
用户："Python 从入门到实战" → 调 get_material_detail → "19.9元，3章：基础、函数、面向对象。要买吗？"
用户："这个资料大概讲什么" → 调 get_material_detail 看目录 → 概括回答，不搜别的
用户："第三章公式怎么推导" → 先 get_material_detail 确认资料 → 再 search_documents 搜内容 → 用文档原文回答

【工具速查】
找资料/推荐 → query_materials, get_categories, get_reviews
问资料内容/大纲/价格 → get_material_detail（不用再搜），想买就 trigger_purchase_offer
问文档具体章节/知识点 → 先用 get_material_detail 确认资料，再用 search_documents 搜内容
订单/售后 → query_orders, get_order_detail, search_faq

【重要规则】
- 用户表达任何购买意向（"买""下单""就这个""来一份""重新发送""卡片""推荐这个"等），唯一正确的回应是调用 trigger_purchase_offer 工具，严禁只用文字回复
- trigger_purchase_offer 是弹出购买卡片的唯一方式，不调用 = 用户看不到卡片 = 你没有完成用户请求
- 即使之前调过 trigger_purchase_offer，用户再次表达购买意向也必须重新调用
- 【最重要】如果你发现自己想说"已发送卡片"或"点击即可下单"，停下——这说明你没调 trigger_purchase_offer，用户根本看不到卡片，立即补调

【风格】
回复简洁，不用emoji，不中途停止，不编造平台没有的资料`

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
  2. 用户说了方向 → query_materials 搜索 → 挑 1-2 个最匹配的推荐
  3. 用户对某资料感兴趣 → get_material_detail + get_reviews
  4. 用户表达购买意向 → trigger_purchase_offer
  5. 发了卡片后 → 等用户决策，不强推其他资料
  6. 用户想了解内容但没买 → search_documents（系统会自动限制返回内容），基于介绍引导购买

禁止：
  - 一上来就甩资料列表
  - 用户拒绝后继续推销同一资料
  - 深入讲解文档内容（告知买后可详细讲解即可）
  - trigger_purchase_offer 是发卡唯一方式，不调 = 用户看不到卡片
  - 说了"已发送卡片"但没调 trigger_purchase_offer → 立即补调`

// tutoringModeBlock 助教模式策略
const tutoringModeBlock = `【助教模式】
你是课程助教，用已购资料的文档内容回答问题。

策略：
  1. 用户提到资料名/章节 → get_material_detail 确认
  2. 用户问知识点 → search_documents 检索
  3. 文档原文回答，注明章节来源
  4. 搜不到 → 诚实说"资料中没有涉及"
  5. 试读章节 → 正常回答
  6. 没买但想了解 → search_documents（系统限制输出），基于介绍引导购买

禁止：
  - 答疑时推荐其他资料（除非用户主动问）
  - 编造文档中没有的内容
  - 管订单和退款
  - 引用内容时注意可信度标记：精确匹配 > 语义匹配 > 模糊匹配`

// supportModeBlock 客服模式策略
const supportModeBlock = `【客服模式】
你是平台客服，用订单数据和 FAQ 处理售后。

策略：
  1. 订单相关问题 → query_orders
  2. 有具体订单号 → get_order_detail
  3. 查 FAQ → search_faq
  4. FAQ 没有 → "这个问题需要转接人工客服处理"

禁止：
  - 推荐资料
  - 回答课程内容
  - 承诺"可以退款""随时退"（FAQ 明确写了才可以说）
  - FAQ 没写的 → "建议联系客服确认"`

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
   - 用户拒绝后不纠缠`

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
	default: // mode="" 第一轮，全部加载
		*parts = append(*parts, shoppingModeBlock)
		*parts = append(*parts, tutoringModeBlock)
		*parts = append(*parts, supportModeBlock)
		return "通用"
	}
}

// buildStateBlock 从 SessionState 生成 State Block 文本。
func buildStateBlock(state *SessionState) string {
	var sb strings.Builder

	sb.WriteString("【当前任务】\n")
	sb.WriteString(fmt.Sprintf("  %s\n", state.Task))

	if len(state.Completed) > 0 {
		sb.WriteString("【已完成】\n")
		for _, s := range state.Completed {
			sb.WriteString(fmt.Sprintf("  ✅ %s\n", s))
		}
	}
	if len(state.ToDo) > 0 {
		sb.WriteString("【还需完成】\n")
		for _, s := range state.ToDo {
			sb.WriteString(fmt.Sprintf("  ⬜ %s\n", s))
		}
	}
	if len(state.Facts) > 0 {
		sb.WriteString("【事实层 - 请以此为准】\n")
		for _, f := range state.Facts {
			sb.WriteString(fmt.Sprintf("  📖 %s | 来源：%s\n", f.Content, f.Source))
		}
	}
	if len(state.Hypotheses) > 0 {
		sb.WriteString("【假设层 - 仅供参考】\n")
		for _, h := range state.Hypotheses {
			sb.WriteString(fmt.Sprintf("  💡 %s | 来源：%s\n", h.Content, h.Source))
		}
	}
	if len(state.Discarded) > 0 {
		sb.WriteString("【废弃层 - 不要使用】\n")
		for _, d := range state.Discarded {
			sb.WriteString(fmt.Sprintf("  🗑️ %s | 原因：%s\n", d.Content, d.Basis))
		}
	}
	return sb.String()
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
