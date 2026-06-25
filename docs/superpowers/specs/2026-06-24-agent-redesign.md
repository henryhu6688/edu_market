# Agent 系统重构设计

## 一、目录结构

```
当前文件 → 新位置：
  agent_engine.go    → service/agent/engine.go
  agent_prompts.go   → service/agent/prompts.go
  agent_tools.go     → service/agent/tools.go
  agent_rag.go       → service/agent/rag.go（不变）
  agent_service.go   → service/agent/service.go
  agent_router.go    → 删除（只剩废弃的 CleanTransferMarkers）

新增文件：
  service/agent/safety.go    ← 熔断器 + 工具边界 + 状态机 + 错误恢复
  service/agent/memory.go    ← 用户画像读写 + 上下文组装
  service/agent/quality.go   ← 硬字段自动修正

模型变更：
  model/session.go      ← 加 Mode / State 字段
  model/user_memory.go   ← 新增：长期记忆表
```

---

## 二、防翻车六道防线

### 2.1 循环控制（三级熔断）

#### Level 1：精确重复检测

**触发时机：** LLM 要调 Tool，执行前。

**判断逻辑：** Tool 名相同 且 argsJSON 字符串完全相同。只比较紧邻上一轮。中间隔了别的 Tool 不管，交给 Level 2。

```go
type CircuitBreaker struct {
    lastToolName string
    lastToolArgs string
}

func (cb *CircuitBreaker) Check(toolName, argsJSON string) (blocked bool, reason string) {
    if toolName == cb.lastToolName && argsJSON == cb.lastToolArgs {
        return true, "重复调用：与上一轮完全相同的工具和参数，请调整策略或直接回答用户"
    }
    return false, ""
}

func (cb *CircuitBreaker) Record(toolName, argsJSON string) {
    cb.lastToolName = toolName
    cb.lastToolArgs = argsJSON
}
```

**拦截后行为：** 不执行 Tool，返回 `ToolResult{Success: false, Content: reason}`。LLM 看到失败自然调整。

#### Level 2：语义回路检测

**触发时机：** Tool 执行后，把结果喂入检测器。

**判断逻辑：** 保留最近 3 轮结果（各截取前 200 字），用 Jaccard 相似度（基于 bigram）计算。三对相似度均 > 0.8 视为回路。

```go
type SemanticLoopDetector struct {
    recentResults []string // 保留最近 3 条，每条截取 200 字
}

func (d *SemanticLoopDetector) Feed(content string) (blocked bool, reason string) {
    d.recentResults = append(d.recentResults, truncate(content, 200))
    if len(d.recentResults) < 3 { return false, "" }
    if len(d.recentResults) > 3 { d.recentResults = d.recentResults[1:] }

    simAB := jaccard(d.recentResults[0], d.recentResults[1])
    simBC := jaccard(d.recentResults[1], d.recentResults[2])
    simAC := jaccard(d.recentResults[0], d.recentResults[2])

    if simAB > 0.8 && simBC > 0.8 && simAC > 0.8 {
        return true, "检测到语义回路：最近三轮查询结果高度重复，无新信息。请基于现有信息直接回答用户。"
    }
    return false, ""
}
```

**拦截后行为：** 注入一条 system 消息到 LLM 上下文中。不阻断本轮，下一轮 LLM 看到后会停止。

#### Level 3：硬上限

**触发时机：** 每轮循环入口。

```go
if round >= maxRounds { // 默认 10，由 config.App.Agent.MaxToolRounds 控制
    history = append(history, agentChatMsg{
        Role: "system",
        Content: "已达到最大对话步数限制。你必须立即基于已有信息回答用户问题，不要再尝试调用任何工具。如果现有信息不足以回答，请诚实告知用户。",
    })
    goto streamFinalAnswer
}
```

**行为：** 跳过后续 Tool Calling 循环，直接进入流式最终回答。

---

### 2.2 工具边界

#### 2.2.1 Tool 接口

```go
type Tool interface {
    Definition() ToolDef
    AllowedModes() []string           // 允许在哪些模式下调用
    ValidateArgs(argsJSON string) error // 参数校验，不信任 LLM
    Execute(userID uint, argsJSON string) ToolResult
    Describe(argsJSON string, result ToolResult) string // 返回描述文本，用于 State Block 的 completed 列表
}
```

#### 2.2.2 各 Tool 的允许模式

| Tool | shopping | tutoring | support |
|------|:---:|:---:|:---:|
| query_materials | ✅ | ✅ | — |
| get_material_detail | ✅ | ✅ | — |
| get_reviews | ✅ | ✅ | — |
| get_categories | ✅ | ✅ | — |
| trigger_purchase_offer | ✅ | — | — |
| search_documents | ✅ | ✅ | — |
| query_orders | — | — | ✅ |
| get_order_detail | — | — | ✅ |
| search_faq | ✅ | ✅ | ✅ |

**检查逻辑：**

```go
func (e *AgentEngine) checkToolMode(tool Tool, sessionMode string) error {
    for _, m := range tool.AllowedModes() {
        if m == sessionMode { return nil }
    }
    return fmt.Errorf("当前模式（%s）不允许使用此工具", sessionMode)
}
```

**第一轮 mode="" 时不检查**，9 个 Tool 全开放。

#### 2.2.3 参数校验示例

```go
// trigger_purchase_offer
func (t triggerPurchaseOfferTool) ValidateArgs(argsJSON string) error {
    var args struct{ MaterialID uint }
    json.Unmarshal([]byte(argsJSON), &args)
    if args.MaterialID == 0 { return errors.New("material_id 不能为空") }
    var count int64
    database.DB.Model(&model.Material{}).Where("id = ? AND status = ?", args.MaterialID, "published").Count(&count)
    if count == 0 { return fmt.Errorf("资料 #%d 不存在或已下架", args.MaterialID) }
    return nil
}

// search_documents
func (t searchMaterialsTool) ValidateArgs(argsJSON string) error {
    var args struct{ MaterialID uint; Query string }
    json.Unmarshal([]byte(argsJSON), &args)
    if args.MaterialID == 0 { return errors.New("material_id 不能为空") }
    if strings.TrimSpace(args.Query) == "" { return errors.New("搜索关键词不能为空") }
    if len(args.Query) > 200 { return errors.New("搜索关键词过长，请精简到 200 字以内") }
    return nil
}
```

#### 2.2.4 调用预算

每轮对话每个 Tool 独立计数，超限拦截：

```go
type ToolBudget struct {
    limits map[string]int
    counts map[string]int
}

func NewToolBudget() *ToolBudget {
    return &ToolBudget{
        limits: map[string]int{
            "trigger_purchase_offer": 3,
            "search_documents":       10,
            "query_materials":        5,
            "get_material_detail":    10,
            "get_reviews":            5,
            "get_categories":         3,
            "query_orders":           3,
            "get_order_detail":       5,
            "search_faq":             5,
        },
        counts: make(map[string]int),
    }
}

func (b *ToolBudget) Spend(toolName string) error {
    if b.counts[toolName] >= b.limits[toolName] {
        return fmt.Errorf("工具 %s 调用次数已达上限（%d次），请基于已有结果回答", toolName, b.limits[toolName])
    }
    b.counts[toolName]++
    return nil
}
```

---

### 2.3 状态管理（三模式）

#### 三个模式

| 模式 | 值 | 职责 |
|------|-----|------|
| 导购 | `shopping` | 帮用户搜索资料、查看详情和评价、引导购买 |
| 助教 | `tutoring` | 用已购资料的文档内容回答用户的疑问 |
| 客服 | `support` | 处理订单查询、退款等售后问题 |

#### 模式切换规则

**由引擎根据本轮实际执行的 Tool 类型判定，不靠 LLM。**

```go
func resolveMode(session *model.Session, executedTools []string) string {
    // 本轮没调任何 Tool → 保持当前模式不变
    if len(executedTools) == 0 {
        return session.Mode
    }

    for _, t := range executedTools {
        switch {
        case t == "query_orders" || t == "get_order_detail":
            return "support"
        case t == "search_documents":
            if checkPurchased(session.UserID, getFocusMaterialID(session)) {
                return "tutoring"
            }
            return "shopping" // 没买，导购模式下的预购答疑
        case t == "query_materials" || t == "get_categories" || t == "get_reviews" ||
             t == "get_material_detail" || t == "trigger_purchase_offer":
            return "shopping"
        }
    }
    return session.Mode
}
```

**关键规则：**
- 模式按轮解析，不粘——每轮独立判定，Tool 执行完就切
- 中途穿插已买资料问题 → search_documents 被执行 → 切 tutoring → 答完再问别的 → 切回去
- 多 Tool 同时调 → 按上述顺序匹配第一个命中的
- 第一轮 mode="" → 不检查白名单，9 个 Tool 全开放

#### 每个模式的 Prompt（见第五章 Mode Block）

---

### 2.4 上下文分层

**三条信息只在确定事件驱动下流转。引擎不做 NLP，不猜用户意图。**

```
三层：
  fact       — 来自 Tool 返回值 / 用户明确说的话 / RAG 精确匹配（confidence=high）
  hypothesis — 来自 RAG 语义匹配（confidence=medium/low）
  discarded  — 同 source 被新 Tool 结果覆盖、用户主动切换资料方向时的旧结果
```

#### 来源到层级的映射

| 来源 | 进入层级 | 何时移除 |
|------|:---:|------|
| Tool 执行返回的数据 | fact | 同 source 新数据到达 → 移入 discarded |
| RAG 检索 confidence=high | fact | 同上 |
| RAG 检索 confidence=medium/low | hypothesis | 同 source 新数据到达 → 移入 discarded |
| 新 Tool 调用同 source 返回了新数据 | fact | — |
| — | — | 旧版本 → discarded |
| 用户切换到另一个资料 | — | 旧 focus 的 fact/hypothesis → 移入 discarded |

#### 引擎每轮更新逻辑

```go
// updateFactsAndHypotheses 上下文分层维护。
// 在每轮 Tool 执行后调用。
func (e *AgentEngine) updateFactsAndHypotheses(state *SessionState, toolName, argsJSON string, result ToolResult) {
    source := fmt.Sprintf("%s(%s)", toolName, argsJSON)

    // 1. Tool 返回的数据 → fact
    if result.Success {
        // 同 source 旧数据 → 移入 discarded
        e.expireOldFacts(state, source)

        state.Facts = append(state.Facts, FactItem{
            Content: truncate(result.Content, 150),
            Source:  source,
        })
    }

    // 2. RAG 结果 → 按 confidence 分流
    if toolName == "search_documents" && result.Success {
        chunks := parseChunks(result.Content)
        for _, c := range chunks {
            item := FactItem{
                Content: c.Content,
                Source:  fmt.Sprintf("rag:chunk_%d", c.ID),
            }
            if c.Confidence == "high" {
                state.Facts = append(state.Facts, item)
            } else {
                item.Basis = fmt.Sprintf("confidence=%s", c.Confidence)
                state.Hypotheses = append(state.Hypotheses, item)
            }
        }
    }
}

// expireOldFacts 将同 source 的旧 fact/hypothesis 移入 discarded。
func (e *AgentEngine) expireOldFacts(state *SessionState, source string) {
    for i, f := range state.Facts {
        if f.Source == source {
            state.Discarded = append(state.Discarded, FactItem{
                Content: f.Content, Source: f.Source, Basis: "被新数据覆盖",
            })
            state.Facts = append(state.Facts[:i], state.Facts[i+1:]...)
            break
        }
    }
    for i, h := range state.Hypotheses {
        if h.Source == source {
            state.Discarded = append(state.Discarded, FactItem{
                Content: h.Content, Source: h.Source, Basis: "被新数据覆盖",
            })
            state.Hypotheses = append(state.Hypotheses[:i], state.Hypotheses[i+1:]...)
            break
        }
    }
}
```

#### 注入 LLM 时的格式

LLM 上下文中，三层以明文区分：

```
【事实层 - 以下数据来自系统查询，请以此为准】
  📖 《Python 从入门到实战》| ¥19.90 | 3章 | 来源：get_material_detail(2)
  📋 订单 #20260624001 | ¥19.90 | 已支付 | 来源：query_orders

【假设层 - 以下内容未经确认，仅供参考】
  💡 用户偏好低价资料 | 来源：llm:inferred | 依据：浏览3次均<20元

【废弃层 - 以下信息已失效，不要使用】
  🗑️ 《Python 数据分析》¥29.9 | 来源：query_materials | 原因：被新搜索结果覆盖
```

---

### 2.5 权限控制

**不是检查"能不能调"而是控制"返回多少内容"。**

`search_documents.Execute()` 内部检查用户是否已购买该资料：

- 已购买 → 返回 `access: "full"` + 完整文档片段
- 未购买但试读章节 → 返回完整试读内容
- 未购买非试读 → 返回 `access: "restricted"` + 摘要 + 目录提示

```go
func (t searchMaterialsTool) Execute(userID uint, argsJSON string) ToolResult {
    materialID := parseMaterialID(argsJSON)
    chunks := search(materialID, parseQuery(argsJSON))

    if checkPurchased(userID, materialID) {
        return ToolResult{Success: true, Content: toJSON(map[string]interface{}{
            "access": "full",
            "chunks": formatChunks(chunks),
        })}
    }

    // 区分试读和非试读
    var preview, restricted []Chunk
    for _, c := range chunks {
        if c.IsFreePreview { preview = append(preview, c) }
        else { restricted = append(restricted, c) }
    }

    return ToolResult{Success: true, Content: toJSON(map[string]interface{}{
        "access": "restricted",
        "free_preview": preview,
        "summary": summarize(restricted),
        "outline": getOutline(materialID),
    })}
}
```

#### 可信度标记

search_documents 返回的每条 chunk 带 `confidence` 字段：

| confidence | 条件 | LLM 应如何引用 |
|------|------|------|
| high | keyword 命中 + 向量相似 > 0.85 | 可直接引用，可注明章节来源 |
| medium | 向量相似 0.5 ~ 0.85，keyword 未命中 | 可作为参考，不声称"原文说" |
| low | 仅 LIKE 模糊匹配 | 告知用户"可能不准确" |

---

### 2.6 错误恢复

#### Tool 执行失败分类

| 类别 | 场景 | 处理 |
|------|------|------|
| A 可重试 | 超时、网络抖动 | 重试 1 次，间隔 2s。仍失败 → 降为 C |
| B 可降级 | Redis 挂了、Embedding API 挂了 | search_documents：Redis KNN → MySQL 内存 → MySQL LIKE |
| C 不可恢复 | 参数非法、资料不存在、无权限 | 返回明确错误给 LLM，LLM 自然告知用户 |

#### LLM API 调用失败

```
callLLM 超时/返回非 200：
  → 重试 1 次（3 秒后）
  → 仍失败 → SSE error 事件："AI 服务暂时不可用，请稍后重试"
  → 不再重试，不让用户等
```

#### ToolResult Source 标记

```go
type ToolResult struct {
    Success bool
    Content string
    Source  string // "primary" | "fallback_l1" | "fallback_l2" | "error" | "blocked"
}
```

Source 用于日志记录和可信度调整，不影响 LLM 看到的 Content。

---

## 三、Session State

### Session 模型新增字段

```go
type Session struct {
    // 现有字段不变...
    ID        uint
    UserID    uint
    AgentType string
    Title     string
    Status    string

    // 新增
    Mode     string `gorm:"type:varchar(20);default:''" json:"mode"`
    // "" | "shopping" | "tutoring" | "support"

    State    string `gorm:"type:text" json:"state"`
    // JSON，结构见下
}
```

### State JSON 结构

```json
{
  "task": "帮用户找 20 元以内的 Python 入门课",
  "completed": [
    "搜索「Python」→ 找到 3 门资料",
    "查看《Python 从入门到实战》详情"
  ],
  "to_do": ["确认用户水平", "发购买卡片"],
  "facts": [
    {"content": "《Python 从入门到实战》¥19.90，3章，4.8分", "source": "get_material_detail(2)"}
  ],
  "hypotheses": [
    {"content": "用户偏好低价资料", "source": "llm:inferred", "basis": "浏览3次均<20元"}
  ],
  "discarded": [
    {"content": "《Python 数据分析》¥29.9", "source": "query_materials", "basis": "用户切换方向"}
  ],
  "context": {
    "candidates": [{"id": 2, "title": "Python 从入门到实战", "price": 19.9}],
    "focus_id": 2,
    "card_sent": false,
    "materials_viewed": [2, 5]
  }
}
```

### FactItem 结构

```go
// FactItem 上下文分层中的一条数据。
// 三层都有统一的 Content/Source 字段。
// Basis 为可选：hypothesis 存 confidence，discarded 存原因。
type FactItem struct {
    Content string `json:"content"`           // 数据内容（自然语言，截取 150 字以内）
    Source  string `json:"source"`            // 来源标识（tool名+参数 或 rag:chunk_id 或 user:said 或 llm:inferred）
    Basis   string `json:"basis,omitempty"`   // hypothesis：confidence值；discarded：废弃原因
}
```

### 各字段的写入时机

| 字段 | 写入者 | 写入时机 |
|------|--------|----------|
| mode | `resolveMode()` | 每轮 Tool 执行后 |
| task | 引擎初始化 | 新会话时直接用用户第一句话（不调 LLM） |
| completed | `updateState()` | Tool 执行成功 → 调 `tool.Describe()` 生成描述追加 |
| to_do | `computeToDo()` | completed 或 context 变更时重新计算 |
| facts | `updateFactsAndHypotheses()` | Tool 执行成功 → 追加；RAG high → 追加 |
| hypotheses | `updateFactsAndHypotheses()` | RAG medium/low → 追加；LLM 推测 → 追加 |
| discarded | `expireOldFacts()` | 同 source 旧数据被新数据覆盖 |
| context | `updateState()` | Tool 返回结构化数据时填入（focus_id、candidates、card_sent 等） |

### computeToDo 逻辑

```go
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
    case "tutoring":
        // 没有固定的 to_do，用户问什么答什么
    case "support":
        if !containsStr(completed, "查FAQ") {
            todos = append(todos, "如当前信息无法解决，查 search_faq")
        }
    }
    return todos
}
```

---

## 四、记忆架构

### 记忆类型（三类）

| 类型 | 存什么 | 存储位置 | 生命周期 |
|------|--------|----------|----------|
| 用户画像 | 知识水平、兴趣标签、价格偏好、已购资料 | user_memories 表 | 长期 |
| 会话状态 | task / completed / to_do / focus_id | Session.State | 单次会话 |
| 上下文窗口 | 最近 15 条消息 + facts/hypotheses/discarded | 引擎每轮拼装 | 单次请求 |

### L3：长期记忆（user_memories 表）

#### 表结构

```sql
CREATE TABLE user_memories (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    mem_key VARCHAR(100) NOT NULL,
    mem_value TEXT,
    source VARCHAR(50) DEFAULT 'explicit',
    -- 'explicit'：用户明确说的
    -- 'behavior'：行为事实（购买、下单）
    -- 'inferred'：从行为推断（暂存，confidence < 0.8 时不写入 L3）
    confidence DOUBLE DEFAULT 0.5,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE INDEX idx_user_mem_key (user_id, mem_key)
);
```

#### Key 白名单

| mem_key | 示例值 | 写入方式 |
|---------|--------|----------|
| `knowledge_level` | `"beginner"` | 用户明确说 → 事件驱动直接写 |
| `interest_tags` | `["Python","数据分析"]` | 同方向搜 3 次 → 事件驱动写 |
| `preferred_price_range` | `{"min":10,"max":30}` | 行为推断 → 事件驱动写 |
| `purchased_material_ids` | `[2,5,7]` | 购买后 → 事件驱动写 |

#### 写入规则

```
可以写（事件驱动，不通过压缩）：
  ✅ 用户明确说的属性（"我是零基础"）
     → source=explicit, confidence=1.0, 直接写入
  ✅ 购买行为（DB 有订单记录）
     → source=behavior, confidence=1.0, 监听购买事件写入
  ✅ 同方向搜 3 次以上
     → source=inferred, confidence=0.6, 写入 interest_tags
     → 同方向判定：两次搜索词 embedding 余弦相似度 > 0.7 视为同方向
     → 用同一 Embedding API（已存在，不新增依赖）
     → 搜索记录暂存引擎内存（session-level map），会话结束清，不持久化
  ✅ 用户持续浏览低价资料
     → source=inferred, confidence=0.5, 写入 preferred_price_range

不能写：
  ❌ LLM 推测的结论
  ❌ 用户拒绝过的推荐
  ❌ 不在白名单内的 key
  ❌ confidence < 0.5

写入前校验：
  - knowledge_level ∈ [beginner, intermediate, advanced]
  - interest_tags 是 string 数组
  - preferred_price_range 包含有效 min/max 数字
  - 同 key 已有旧值且 confidence 更高 → 不覆盖
```

#### 检索规则

按模式筛选，不全量注入。`buildUserContextBlock(userID, mode)` 内部实现：

```go
func buildUserContextBlock(userID uint, mode string) string {
    memories := loadUserMemories(userID)
    if len(memories) == 0 { return "" }

    var lines []string
    lines = append(lines, "【用户画像】")

    // shopping：全量
    // tutoring：只取水平和已购
    // support：只取已购
    for _, m := range memories {
        switch mode {
        case "shopping":
            // 全部注入
        case "tutoring":
            if m.Key != "knowledge_level" && m.Key != "purchased_material_ids" {
                continue
            }
        case "support":
            if m.Key != "purchased_material_ids" {
                continue
            }
        }
        lines = append(lines, formatMemoryLine(m))
    }
    return strings.Join(lines, "\n")
}
```

#### 遗忘机制

记忆不是越多越好。长期记忆必须会忘，否则过期、冲突、噪音会毁掉检索质量。

六条遗忘规则：

```
1. 过期时间
   user_memories 表已有 updated_at 字段。
   超过 90 天未 updated 的记录 → 自动标记为过期，检索时跳过。
   不真删——万一用户又回来，重新激活（confidence 降 0.2）。

2. 置信度降权
   每次检索时检查：
   - 30 天未 updated → confidence 减 0.2
   - 60 天未 updated → confidence 减 0.4
   - 90 天未 updated → 跳过
   降权后 confidence < 0.3 → 删除。

3. 冲突合并
   同 key 存在多条记录时：
   - 比较 updated_at，保留最新的
   - 旧记录 confidence > 新记录 → 不覆盖，两条都保留但旧记录标记 conflict
   - 用户改口（source=explicit）→ 无条件覆盖旧值

4. 更新时间
   updated_at 仅在写入新数据时更新。
   过期判断完全由定时任务扫表处理，请求路径不做额外 DB 写。

5. 用户主动删除
   用户说"忘了这个""别记这个""删除我的记忆" → 按 key 精确删除。
   用户说"全忘了" → 该用户全部 user_memories 删除。

6. 低价值清理
   定时任务（每周一次）：
   - confidence < 0.3 → 删除
   - 90 天未 updated → 删除
   - interest_tags 中 60 天未被再次提及的标签 → 移除
```

对应到表结构，新增一个字段：

```sql
ALTER TABLE user_memories ADD COLUMN status VARCHAR(20) DEFAULT 'active';
-- 'active'：正常使用
-- 'expired'：已过期，检索时跳过
-- 'conflict'：冲突，保留不删但检索时跳过
```

#### 更新规则

| 事件 | 操作 |
|------|------|
| 用户明确说新属性 | 写入，source=explicit，confidence=1.0 |
| 用户改口 | 无条件覆盖旧值，confidence=1.0 |
| 新购买 | purchased_material_ids 追加 |
| 同方向搜 3 次 | interest_tags 追加标签 |
| 60 天未提及的标签 | 从 interest_tags 移除 |
| 30 天未 updated | confidence 减 0.2 |
| 90 天未 updated | 标记 expired |
| confidence < 0.3 | 定时任务删除 |
| 用户说"别再推 XX" | interest_tags 移除对应标签 |
| 用户要求删除 | 按 key 精确删除 |

### L1：上下文组装

```
agentChatMsg{Role: "system"}    ← buildPrompt 返回 6 模块拼接
    ↓
agentChatMsg{Role: "user"}      ← 消息 1
agentChatMsg{Role: "assistant"} ← 消息 2
...                             ← 最近 15 条，ID ASC
```

Session.State 中的 facts/hypotheses/discarded 通过 State Block 注入 Prompt（见第五章）。不在 System Prompt 之外另加注入点。

### loadRecentMessages

加载最近 15 条消息给 LLM。不做压缩：

```go
func (e *AgentEngine) loadRecentMessages(sessionID uint) []agentChatMsg {
    // 1. 最近 15 条（DESC → 反转 ASC）
    var dbMsgs []model.Message
    database.DB.Where("session_id = ?", sessionID).
        Order("id DESC").Limit(15).Find(&dbMsgs)
    for i, j := 0, len(dbMsgs)-1; i < j; i, j = i+1, j-1 {
        dbMsgs[i], dbMsgs[j] = dbMsgs[j], dbMsgs[i]
    }

    // 2. 过滤 action 卡片 + 还原 tool_calls 格式
    var msgs []agentChatMsg
    for _, m := range dbMsgs {
        if m.Role == "assistant" && strings.Contains(m.Content, `"purchase_offer"`) {
            continue
        }
        msg := agentChatMsg{Role: m.Role, Content: m.Content, ReasoningContent: m.ReasoningContent}
        if m.Role == "assistant" && len(m.ToolCalls) > 0 {
            msg.ToolCalls = restoreToolCalls(m.ToolCalls)
        }
        if m.Role == "tool" && len(m.ToolCalls) > 0 {
            msg.ToolCallID = m.ToolCalls[0].CallID
        }
        msgs = append(msgs, msg)
    }
    return msgs
}
```

Session 模型变更已在第三章定义，不重复。

---

## 五、Prompt 模块化

### 拼装顺序

```
1. Base Persona    ← 固定，永远在
2. Mode Block      ← 按 session.Mode 选择（第一轮 mode="" 时三个全加）
3. State Block     ← 从 session.State JSON 动态生成
4. User Context    ← 从 user_memories 表加载，无数据则跳过
5. Rules           ← 固定，永远在
6. Style           ← 固定，永远在
```

### 5.1 Base Persona

```
你是 edu_market 的智能助手，负责三件事：
- 导购：帮用户找到合适的学习资料并完成购买
- 助教：用已购资料的文档内容解答用户疑问
- 客服：处理订单查询、退款、售后问题

当前处于「{{mode}}」模式，请专注当前模式的任务。
```
`{{mode}}` 替换为对应中文。

### 5.2 Mode Block（三个）

**shopping：**
```
【导购模式】
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
  - 说了"已发送卡片"但没调 trigger_purchase_offer → 立即补调
```

**tutoring：**
```
【助教模式】
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
  - 引用内容时注意可信度标记：精确匹配 > 语义匹配 > 模糊匹配
```

**support：**
```
【客服模式】
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
  - FAQ 没写的 → "建议联系客服确认"
```

### 5.3 State Block

从 `session.State` JSON 动态生成：

```go
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
```

生成效果示例：
```
【当前任务】
  帮用户找 20 元以内的 Python 入门课
【已完成】
  ✅ 搜索「Python」→ 找到 3 门资料
  ✅ 查看《Python 从入门到实战》详情
【还需完成】
  ⬜ 询问用户是否购买 → 调 trigger_purchase_offer
【事实层 - 请以此为准】
  📖 《Python 从入门到实战》¥19.90，3章，4.8分 | 来源：get_material_detail(2)
```

### 5.4 User Context Block

无数据则跳过。有数据时：

```
【用户画像】
  兴趣：Python、数据分析（可信度：中）
  水平：零基础（可信度：高）
  预算偏好：10-30 元（可信度：低）
  已购资料：📖《Python 从入门到实战》
```

### 5.5 Rules Block

```
【核心规则 - 不可违反】

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
```

### 5.6 Style Block

```
【回复格式】

1. 长度：不超过 3 段，每段不超过 3 句
2. 推荐资料：
   推荐《资料名》- ¥XX.XX
   亮点：一句话概括
   （挑 1-2 个最匹配的）
3. 解释知识：一句话结论 → 展开 → 注明章节
4. 不用：打招呼、客套话、emoji
5. 兜底："抱歉，这个问题建议联系平台客服处理。"
```

---

## 六、quality.go：硬字段自动修正

### 处理范围

**只修正：** 价格、金额、日期——从 facts 可精确比对的数据。

**不处理：** 主观评价、承诺性表述、知识点真假。

### 修正流程

```
每个 LLM 返回完整回答后（非流式轮）：
  → quality.Correct() 修正硬字段
  → 修正后内容：存 DB / 拼入下一轮 history / 模拟流式发 SSE
  → 打日志记录修正
```

Tool Calling 轮：非流式，拿到完整回答 → 修正 → 存 DB + 拼入 history。
最终回答轮：非流式，拿到完整回答 → 修正 → 模拟流式逐字发 SSE → 存 DB。

### 实现

```go
type HardFieldCorrector struct{}

func (c *HardFieldCorrector) Correct(answer string, facts []FactItem) string {
    correctValues := extractHardFields(facts)
    claimedValues := extractHardFieldsFromText(answer)

    result := answer
    for _, claimed := range claimedValues {
        if correct, ok := correctValues[claimed.field]; ok && correct != claimed.value {
            result = strings.Replace(result, claimed.raw, correct, 1)
            slog.Warn("quality: 硬字段修正",
                "field", claimed.field,
                "claimed", claimed.value,
                "corrected", correct,
            )
        }
    }
    return result
}
```

---

## 七、引擎执行流程

### 完整 Run() 循环

```go
func (e *AgentEngine) Run(session *model.Session, userMsg string,
    tools map[string]Tool, sseHandler SSEHandler, requestID string) error {

    // ========== 0. 初始化 ==========
    breaker := &CircuitBreaker{}
    loopDetector := &SemanticLoopDetector{}
    budget := NewToolBudget()
    corrector := &HardFieldCorrector{}

    // ========== 1. 存用户消息 ==========
    e.storeUserMessage(session.ID, userMsg)

    // ========== 2. 初始化 State（新会话）==========
    if session.State == "" {
        session.State = e.initState(userMsg) // task = userMsg
    }

    // ========== 3. 组装 Prompt 并加载上下文 ==========
    systemPrompt := e.buildPrompt(session)
    history := []agentChatMsg{
        {Role: "system", Content: systemPrompt},
    }
    history = append(history, e.loadRecentMessages(session.ID)...)

    openAITools := toolDefsToOpenAI(tools)

    // ========== 4. Tool Calling 循环 ==========
    for round := 0; round < e.maxRounds; round++ {

        // Level 3: 硬上限
        if round >= e.maxRounds-1 {
            history = append(history, agentChatMsg{
                Role: "system",
                Content: "已达最大对话步数。必须立即回答，不能再调用工具。",
            })
            goto streamFinalAnswer
        }

        // ----- 调 LLM（非流式，带 tools）-----
        resp, err := e.callLLM(history, openAITools)
        if err != nil {
            // 重试 1 次
            time.Sleep(3 * time.Second)
            resp, err = e.callLLM(history, openAITools)
            if err != nil {
                sseHandler("error", `{"message":"AI 服务暂时不可用，请稍后重试"}`)
                return err
            }
        }

        choice := resp.Choices[0]

        // ----- 有 Tool Calls → 执行 -----
        if len(choice.Message.ToolCalls) > 0 {
            var executedTools []string
            var roundMsgs []agentChatMsg

            for _, tc := range choice.Message.ToolCalls {
                toolName := tc.Function.Name
                tool := tools[toolName]

                // Level 1: 精确重复
                var result ToolResult
                if blocked, reason := breaker.Check(toolName, tc.Function.Arguments); blocked {
                    result = ToolResult{Success: false, Content: reason, Source: "blocked"}
                    goto recordResult
                }
                breaker.Record(toolName, tc.Function.Arguments)

                // 工具边界：模式白名单（第一轮 mode="" 跳过）
                if session.Mode != "" {
                    if err := e.checkToolMode(tool, session.Mode); err != nil {
                        result = ToolResult{Success: false, Content: err.Error(), Source: "blocked"}
                        goto recordResult
                    }
                }

                // 工具边界：参数校验
                if err := tool.ValidateArgs(tc.Function.Arguments); err != nil {
                    result = ToolResult{Success: false, Content: "参数错误: " + err.Error(), Source: "blocked"}
                    goto recordResult
                }

                // 工具边界：调用预算
                if err := budget.Spend(toolName); err != nil {
                    result = ToolResult{Success: false, Content: err.Error(), Source: "blocked"}
                    goto recordResult
                }

                // 执行 Tool
                sseHandler("thinking", fmt.Sprintf(`{"tool":"%s","status":"executing"}`, toolName))
                result = tool.Execute(session.UserID, tc.Function.Arguments)

            recordResult:
                // 存 DB
                e.storeToolMessages(session.ID, tc, toolName, result)

                // Level 2: 语义回路
                if blocked, reason := loopDetector.Feed(result.Content); blocked {
                    history = append(history, agentChatMsg{Role: "system", Content: reason})
                }

                // ★ 上下文分层：更新 facts / hypotheses / discarded
                e.updateFactsAndHypotheses(session, toolName, tc.Function.Arguments, result)

                // ★ 更新 State：completed / context / to_do
                e.updateTaskState(session, toolName, tc.Function.Arguments, result)

                executedTools = append(executedTools, toolName)

                // action 检测 → SSE
                if strings.Contains(result.Content, `"__action"`) {
                    e.handleAction(sseHandler, session, tc, result)
                }

                // 拼接本轮消息
                roundMsgs = append(roundMsgs,
                    agentChatMsg{Role: "assistant", ToolCalls: []toolCallItem{{ID: tc.ID, Type: "function", Function: toolCallFunc{Name: toolName, Arguments: tc.Function.Arguments}}}},
                    agentChatMsg{Role: "tool", Content: result.Content, ToolCallID: tc.ID},
                )
            }

            // ★ 更新模式（本轮 Tool 执行完后）
            session.Mode = resolveMode(session, executedTools)

            history = append(history, roundMsgs...)
            e.saveSession(session) // 持久化 State + Mode
            continue
        }

        // ----- 没有 Tool Call → 最终回答（非流式 + 模拟流式 SSE）-----
    streamFinalAnswer:
        history = append(history, agentChatMsg{
            Role: "system",
            Content: "请基于以上信息简洁回答用户。只输出回答内容。",
        })

        // 非流式拿到完整回答
        finalResp, err := e.callLLM(history, nil) // tools=nil
        if err != nil {
            sseHandler("error", `{"message":"生成回答失败"}`)
            return err
        }
        fullAnswer := finalResp.Choices[0].Message.Content

        // quality：完整回答在手，先修正再发
        facts := e.getFacts(session.State)
        fullAnswer = corrector.Correct(fullAnswer, facts)

        // 模拟流式逐字发 SSE
        e.streamAnswer(fullAnswer, func(delta string) {
            sseHandler("delta", formatDelta(delta))
        })

        e.storeAssistantMessage(session.ID, fullAnswer, finalResp.Usage.TotalTokens)
        sseHandler("done", formatDone(session))
        return nil
    }
    return nil
}
```

### buildPrompt 实现

```go
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
```

### loadRecentMessages 实现

```go
func (e *AgentEngine) loadRecentMessages(sessionID uint) []agentChatMsg {
    // 1. 最近 15 条（DESC 取最新 → 反转为 ASC）
    var dbMsgs []model.Message
    database.DB.Where("session_id = ?", sessionID).
        Order("id DESC").Limit(15).Find(&dbMsgs)
    for i, j := 0, len(dbMsgs)-1; i < j; i, j = i+1, j-1 {
        dbMsgs[i], dbMsgs[j] = dbMsgs[j], dbMsgs[i]
    }

    // 2. 过滤 action 卡片 + 还原 tool_calls 格式
    var msgs []agentChatMsg
    for _, m := range dbMsgs {
        if m.Role == "assistant" && strings.Contains(m.Content, `"purchase_offer"`) {
            continue
        }
        msg := agentChatMsg{Role: m.Role, Content: m.Content, ReasoningContent: m.ReasoningContent}
        if m.Role == "assistant" && len(m.ToolCalls) > 0 {
            msg.ToolCalls = restoreToolCalls(m.ToolCalls)
        }
        if m.Role == "tool" && len(m.ToolCalls) > 0 {
            msg.ToolCallID = m.ToolCalls[0].CallID
        }
        msgs = append(msgs, msg)
    }
    return msgs
}
```

---

## 八、实现路线

| 阶段 | 内容 | 产出文件 |
|------|------|----------|
| 1 | 目录重组 | 移动 6 个文件到 `service/agent/`，删 agent_router.go |
| 2 | 模型变更 | model/session.go + model/user_memory.go |
| 3 | tools.go 增强 | 加 AllowedModes / ValidateArgs / Describe 方法 |
| 4 | safety.go | CircuitBreaker + SemanticLoopDetector + ToolBudget + resolveMode + 错误恢复 |
| 5 | memory.go | buildPrompt + loadRecentMessages + L3 读写 |
| 6 | prompts.go | 6 个 Block 常量 + buildStateBlock + buildUserContextBlock |
| 7 | quality.go | HardFieldCorrector |
| 8 | engine.go | 集成所有组件，调整 Run() 循环 |
| 9 | controller | 适配新 import 路径 |
| 10 | 测试 | 熔断器 + 状态机 + quality 单元测试 |

---

## 九、不做的

- **不做 NLP / 关键词匹配**。用户意图判断交给 LLM，状态管理只响应确定事件。
- **不做流式 API 直接发送**。最终回答先非流式拿完整结果 → quality 修正 → 再模拟流式发 SSE。
- **不做多 Agent 场景路由**。单 Agent + 模式切换够用。
- **不做 Structured Output**。DeepSeek 兼容性不确定，与流式 SSE 冲突。
- **不做事后质量阻断**。准确率不够，误拦更差。

---

## 十、代码注释规范

### 包级注释

每个文件头部说明文件职责：

```go
// Package agent 提供 edu_market 智能助手引擎。
//
// 实现基于 LLM Tool Calling 的多轮对话 Agent，支持三种业务模式（导购/助教/客服），
// 具备熔断器、上下文分层、状态管理、长期记忆、权限控制等防护机制。
//
// 文件职责：
//   - engine.go    主循环，编排 LLM 调用与 Tool 执行
//   - prompts.go   6 模块 Prompt 拼装
//   - tools.go     Tool 定义、校验、调用预算
//   - rag.go       RAG 向量检索
//   - service.go   会话管理与对外接口
//   - safety.go    熔断器 + 工具边界 + 状态机 + 错误恢复
//   - memory.go    用户画像读写 + 上下文组装
//   - quality.go   硬字段自动修正
package agent
```

### 导出结构体

```go
// CircuitBreaker 熔断器，防止 LLM 对同一 Tool 反复调用。
// 仅检测紧邻上一轮，跨轮重复由 SemanticLoopDetector 处理。
type CircuitBreaker struct {
    lastToolName string
    lastToolArgs string
}

// SessionState 会话任务状态快照。
// 由引擎在每轮 Tool 执行后自动更新，注入 Prompt State Block 中通知 LLM 当前进度。
type SessionState struct {
    Task       string       // 当前任务描述
    Completed  []string     // 已完成步骤
    ToDo       []string     // 待完成步骤
    Facts      []FactItem   // 事实层（Tool 返回、RAG 精确匹配）
    Hypotheses []FactItem   // 假设层（RAG 模糊匹配、LLM 推测）
    Discarded  []FactItem   // 废弃层（被覆盖的旧数据）
    Context    ContextData  // 业务上下文
}

// FactItem 上下文分层中的一条数据项。
// 三层共用此结构，Basis 为可选扩展字段。
type FactItem struct {
    Content string `json:"content"`
    Source  string `json:"source"`
    Basis   string `json:"basis,omitempty"`
}
```

### 导出函数

```go
// Check 检查是否触发 Level 1 精确重复熔断。
// toolName 和 argsJSON 与上一轮完全相同 → blocked=true。
func (cb *CircuitBreaker) Check(toolName, argsJSON string) (blocked bool, reason string)

// Feed 将本轮 Tool 结果喂入 Level 2 语义回路检测器。
// 保留最近 3 轮结果（各截取 200 字），Jaccard 相似度判定。
func (d *SemanticLoopDetector) Feed(content string) (blocked bool, reason string)

// ResolveMode 根据本轮执行的 Tool 类型判定当前模式。
// 不依赖用户消息语义——只看实际执行了什么 Tool。
// 返回 "shopping" | "tutoring" | "support" | ""（保持当前）。
func ResolveMode(session *model.Session, executedTools []string) string

// Correct 对 LLM 回答中的硬字段作自动修正（价格/金额/日期/资料名）。
// 所有 LLM 回答均为非流式获取，修正后再发往前端。
func (c *HardFieldCorrector) Correct(answer string, facts []FactItem) string

// SaveUserMemory 写入一条长期记忆到 L3。
// 自动过筛：source/白名单/value校验/confidence门槛/去重。
func (m *MemoryManager) SaveUserMemory(userID uint, key, value, source string, confidence float64) error
```

---

## 十一、日志规范

### 日志级别

| 级别 | 场景 | 示例 |
|------|------|------|
| Info | 关键节点 | 会话开始/结束、模式切换、熔断触发、action 发送 |
| Debug | 调试细节 | 上下文组装、Tool 执行、LLM 请求/响应 |
| Warn | 异常不阻断 | quality 修正、降级路径、熔断触发 |
| Error | 需排查 | LLM 重试后仍失败、DB 写入失败 |

### 必须打的日志点

```
engine.go:
  Agent 开始/完成       (Info)
  上下文加载            (Debug)
  模式切换              (Info)
  每轮 LLM 请求/响应     (Debug)

safety.go:
  熔断 L1/L2/L3         (Warn)
  白名单拦截            (Warn)
  预算耗尽              (Warn)
  状态更新              (Debug)

memory.go:
  L3 写入               (Info)
  上下文组装(token分布) (Debug)

quality.go:
  硬字段修正            (Warn)

错误恢复:
  Tool 降级             (Warn)
  LLM 重试              (Warn)
  LLM 最终失败          (Error)
```

### 约定

- 全部用 `slog.Info/Debug/Warn/Error`，不用 `fmt.Println`
- 所有日志带 `"request_id"` 字段
- 涉及 session 的带 `"session_id"`，涉及 user 的带 `"user_id"`
- 敏感数据（API Key、密码、Token）绝对不出现在日志中
- 长文本用 `truncate(xxx, 200)` 截断
- 熔断触发用 Warn（预期行为），DB 写入失败用 Error（需要排查）
