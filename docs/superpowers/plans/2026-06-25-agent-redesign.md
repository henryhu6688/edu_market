# Agent 系统重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Agent 系统重构为确定性防护架构：六道防线 + 三模式切换 + 三层上下文 + 长期记忆 + 硬字段修正

**Architecture:** `service/agent/` 包，7 个单文件协作。AgentEngine.Run() 编排 Tool Calling 循环，safety.go 提供六道确定性防线，prompts.go 动态拼装 6 模块 Prompt，memory.go 管理 L3 长期记忆，quality.go 修正硬字段

**Tech Stack:** Go + Gin + GORM + MySQL + slog

## Global Constraints

- 所有 HTTP 响应走 `utils/response.go`，禁止 `c.JSON()`
- Service 层不碰 `gin.Context`，只返回 Go `error`
- 敏感数据不硬编码，用 `config.App.XXX` 读取
- 测试库独立 `edu_market_test`，TestMain 自动建库清空
- GORM 错误用 `errors.Is(err, gorm.ErrRecordNotFound)` 判断
- 导出函数/结构体必须有中文注释
- 所有日志用 `slog`，带 `request_id` 字段
- 不做关键词匹配、不做 NLP——引擎只响应确定事件

---

### Task 1: 目录重组 — 创建 `service/agent/` 包

**Files:**
- Create: `service/agent/rag.go` (move from service/agent_rag.go)
- Create: `service/agent/tools.go` (move from service/agent_tools.go)
- Create: `service/agent/service.go` (move from service/agent_service.go)
- Create: `service/agent/engine.go` (move from service/agent_engine.go)
- Create: `service/agent/prompts.go` (move from service/agent_prompts.go)
- Delete: `service/agent_rag.go`, `service/agent_tools.go`, `service/agent_service.go`, `service/agent_engine.go`, `service/agent_prompts.go`, `service/agent_router.go`

**Interfaces:**
- Produces: package `service/agent` with existing files relocated, no functional changes

- [ ] **Step 1: 创建目录并移动文件**

```bash
mkdir -p d:/Vscoding/edu_market/service/agent
cd d:/Vscoding/edu_market

# 移动文件并更新 package 声明为 agent
for f in agent_rag agent_tools agent_service agent_engine agent_prompts; do
  cp service/${f}.go service/agent/${f#agent_}.go
done
```

- [ ] **Step 2: 修改每个新文件的 package 声明**

将每个 `service/agent/*.go` 的 `package service` 改为 `package agent`。

- [ ] **Step 3: 删除旧文件**

```bash
rm service/agent_rag.go service/agent_tools.go service/agent_service.go service/agent_engine.go service/agent_prompts.go service/agent_router.go
```

- [ ] **Step 4: 编译检查**

```bash
cd d:/Vscoding/edu_market && go build ./...
```
Expected: 编译报错（import 路径未更新），这是预期行为，Task 2 修。

- [ ] **Step 5: Commit**

```bash
git add service/agent/ service/agent_*.go
git commit -m "refactor: 移动 Agent 文件到 service/agent/ 包"
```

---

### Task 2: 更新所有 import 路径

**Files:**
- Modify: `controller/agent_controller.go`
- Modify: `service/agent/service.go` (imports internal references)
- Modify: `service/agent/engine.go` (imports model/database)
- Modify: `main.go` 或 router 中引用了 service.AgentService 的地方

**Interfaces:**
- Consumes: Task 1 产生的新包结构
- Produces: 全项目编译通过，功能不变

- [ ] **Step 1: 查找所有引用 `service.` 的地方**

```bash
cd d:/Vscoding/edu_market
grep -rn "service\." --include="*.go" | grep -v "_test.go" | grep -v "service/agent/"
```

- [ ] **Step 2: 更新 controller/agent_controller.go**

```go
// 改前
import "edu_market/service"
// ...
svc *service.AgentService

// 改后
import "edu_market/service/agent"
// ...
svc *agent.AgentService
```

其他引用（`service.SearchFunc`、`service.GetPagination`）同样改为 `agent.SearchFunc`、`agent.GetPagination`。

- [ ] **Step 3: 更新 main.go 或 router 中的引用**

将所有 `service.NewAgentService` → `agent.NewAgentService`，`service.NewAgentEngine` → `agent.NewAgentEngine`，`service.InitRAG` 等所有 service 包引用更新为 agent 包。

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```
Expected: 编译通过，无报错。

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "fix: 更新 Agent 包 import 路径"
```

---

### Task 3: 模型变更 — Session + user_memories

**Files:**
- Modify: `model/session.go`
- Create: `model/user_memory.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `model.Session.Mode string` — "shopping"|"tutoring"|"support"|""
  - `model.Session.State string` — JSON 文本
  - `model.UserMemory` — L3 长期记忆模型
  - `model.UserMemory{}.TableName()` → "user_memories"

- [ ] **Step 1: 修改 Session 模型**

读 `model/session.go`，在 Status 字段下方新增：

```go
// Session 模型
type Session struct {
    // ... 现有字段不变 ...

    // Mode Agent 当前模式（shopping/tutoring/support/"" = 第一轮未判定）
    Mode  string `gorm:"type:varchar(20);default:''" json:"mode"`
    // State 会话任务状态 JSON（task/completed/to_do/facts/hypotheses/discarded/context）
    State string `gorm:"type:text" json:"state"`
}
```

- [ ] **Step 2: 创建 model/user_memory.go**

```go
package model

import "time"

// UserMemory 长期记忆模型（L3）
type UserMemory struct {
    ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
    UserID     uint      `gorm:"not null;index" json:"user_id"`
    MemKey     string    `gorm:"type:varchar(100);not null" json:"mem_key"`
    MemValue   string    `gorm:"type:text" json:"mem_value"`
    Source     string    `gorm:"type:varchar(50);default:explicit" json:"source"`
    Confidence float64   `gorm:"default:0.5" json:"confidence"`
    Status     string    `gorm:"type:varchar(20);default:active" json:"status"`
    CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt  time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (UserMemory) TableName() string {
    return "user_memories"
}
```

- [ ] **Step 3: 创建 migration SQL（或 AutoMigrate）**

在 `database/mysql.go` 的 `Init()` 函数中新增：

```go
database.DB.AutoMigrate(&model.UserMemory{})
```

Session 的 Mode/State 字段通过 GORM AutoMigrate 自动新增。

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add model/session.go model/user_memory.go
git commit -m "feat: Session 加 Mode/State 字段，新增 UserMemory 模型"
```

---

### Task 4: tools.go 增强 — Tool 接口扩展

**Files:**
- Modify: `service/agent/tools.go`

**Interfaces:**
- Consumes: Task 3 (模型已就绪)
- Produces:
  - `Tool` 接口新增 `AllowedModes() []string`、`ValidateArgs(argsJSON string) error`、`Describe(argsJSON string, result ToolResult) string`
  - 所有 9 个 Tool 实现新方法

- [ ] **Step 1: Tool 接口加方法**

```go
type Tool interface {
    Definition() ToolDef
    AllowedModes() []string
    ValidateArgs(argsJSON string) error
    Execute(userID uint, argsJSON string) ToolResult
    Describe(argsJSON string, result ToolResult) string
}
```

- [ ] **Step 2: 为每个 Tool 添加 AllowedModes()**

参照 spec 2.2.2 表格：

```go
func (t queryCoursesTool) AllowedModes() []string     { return []string{"shopping", "tutoring"} }
func (t getMaterialDetailTool) AllowedModes() []string { return []string{"shopping", "tutoring"} }
func (t getReviewsTool) AllowedModes() []string        { return []string{"shopping", "tutoring"} }
func (t getCategoriesTool) AllowedModes() []string     { return []string{"shopping", "tutoring"} }
func (t triggerPurchaseOfferTool) AllowedModes() []string { return []string{"shopping"} }
func (t searchMaterialsTool) AllowedModes() []string   { return []string{"shopping", "tutoring"} }
func (t queryOrdersTool) AllowedModes() []string       { return []string{"support"} }
func (t getOrderDetailTool) AllowedModes() []string    { return []string{"support"} }
func (t searchFAQTool) AllowedModes() []string         { return []string{"shopping", "tutoring", "support"} }
```

- [ ] **Step 3: 为每个 Tool 添加 ValidateArgs()**

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
    if len(args.Query) > 200 { return errors.New("搜索关键词过长") }
    return nil
}

// 其余 Tool 返回 nil（无参数或参数可空）
```

- [ ] **Step 4: 为每个 Tool 添加 Describe()**

```go
func (t queryCoursesTool) Describe(argsJSON, result string) string {
    var args struct{ Keyword string }
    json.Unmarshal([]byte(argsJSON), &args)
    return fmt.Sprintf("搜索「%s」→ 找到 %d 门资料", args.Keyword, countJSONItems(result))
}

func (t getMaterialDetailTool) Describe(argsJSON, result string) string {
    var d struct{ Title string }
    json.Unmarshal([]byte(result), &d)
    return fmt.Sprintf("查看《%s》详情", d.Title)
}

func (t triggerPurchaseOfferTool) Describe(argsJSON, result string) string {
    return "发送购买卡片"
}

func (t searchMaterialsTool) Describe(argsJSON, result string) string {
    var args struct{ Query string }
    json.Unmarshal([]byte(argsJSON), &args)
    return fmt.Sprintf("搜索文档「%s」→ 找到 %d 条", args.Query, countJSONItems(result))
}

func (t queryOrdersTool) Describe(argsJSON, result string) string {
    return fmt.Sprintf("查询用户订单 → 共 %d 笔", countJSONItems(result))
}

func (t getOrderDetailTool) Describe(argsJSON, result string) string {
    return "查看订单详情"
}

func (t searchFAQTool) Describe(argsJSON, result string) string {
    var args struct{ Query string }
    json.Unmarshal([]byte(argsJSON), &args)
    return fmt.Sprintf("搜索FAQ「%s」→ 找到 %d 条", args.Query, countJSONItems(result))
}

func (t getReviewsTool) Describe(argsJSON, result string) string {
    count := countJSONItems(result)
    return fmt.Sprintf("查看评价 → %d 条", count)
}

func (t getCategoriesTool) Describe(argsJSON, result string) string {
    return fmt.Sprintf("获取分类列表 → %d 个", countJSONItems(result))
}
```

- [ ] **Step 5: 编译验证**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add service/agent/tools.go
git commit -m "feat: Tool 接口加 AllowedModes/ValidateArgs/Describe 方法"
```

---

### Task 5: safety.go — 熔断器与工具边界

**Files:**
- Create: `service/agent/safety.go`

**Interfaces:**
- Consumes: Task 4 (Tool 接口)
- Produces:
  - `type CircuitBreaker` + `Check()` + `Record()`
  - `type SemanticLoopDetector` + `Feed()`
  - `type ToolBudget` + `NewToolBudget()` + `Spend()`
  - `func ResolveMode()`

- [ ] **Step 1: 创建文件骨架**

```go
package agent

import (
    "fmt"
    "strings"

    "edu_market/database"
    "edu_market/model"
)
```

- [ ] **Step 2: 实现 CircuitBreaker（Level 1 熔断）**

```go
// CircuitBreaker 精确重复熔断器，防止 LLM 对同一 Tool 反复调用。
// 仅检测紧邻上一轮，跨轮重复由 SemanticLoopDetector 处理。
type CircuitBreaker struct {
    lastToolName string
    lastToolArgs string
}

// Check 检查是否触发 Level 1 精确重复熔断。
func (cb *CircuitBreaker) Check(toolName, argsJSON string) (blocked bool, reason string) {
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
```

- [ ] **Step 3: 实现 SemanticLoopDetector（Level 2 熔断）**

```go
// SemanticLoopDetector 语义回路检测器。
// 保留最近 3 轮 Tool 结果，用 Jaccard 相似度判断是否陷入回路。
type SemanticLoopDetector struct {
    recentResults []string
}

// Feed 将本轮 Tool 结果喂入检测器。
func (d *SemanticLoopDetector) Feed(content string) (blocked bool, reason string) {
    truncated := truncateRunes(content, 200)
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
        if setB[k] { intersection++ }
    }
    union := len(setA) + len(setB) - intersection
    if union == 0 { return 0 }
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
```

- [ ] **Step 4: 实现 ToolBudget（调用预算）**

```go
// ToolBudget Tool 调用预算。每轮对话每个 Tool 独立计数。
type ToolBudget struct {
    limits map[string]int
    counts map[string]int
}

// NewToolBudget 创建调用预算（各 Tool 上限见 spec 2.2.4）。
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

// Spend 消耗一次调用配额，超限返回 error。
func (b *ToolBudget) Spend(toolName string) error {
    if b.counts[toolName] >= b.limits[toolName] {
        return fmt.Errorf("工具 %s 调用次数已达上限（%d次），请基于已有结果回答", toolName, b.limits[toolName])
    }
    b.counts[toolName]++
    return nil
}
```

- [ ] **Step 5: 实现 resolveMode（状态机）**

```go
// ResolveMode 根据本轮实际执行的 Tool 类型判定当前模式。
// 不依赖用户消息语义——只看实际执行了什么 Tool。
func ResolveMode(session *model.Session, executedTools []string) string {
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
            return "shopping"
        case t == "query_materials" || t == "get_categories" || t == "get_reviews" ||
             t == "get_material_detail" || t == "trigger_purchase_offer":
            return "shopping"
        }
    }
    return session.Mode
}

// checkPurchased 检查用户是否已购买指定资料。
func checkPurchased(userID, materialID uint) bool {
    var count int64
    database.DB.Model(&model.Order{}).
        Where("user_id = ? AND material_id = ? AND status = ?", userID, materialID, "paid").
        Count(&count)
    return count > 0
}

// getFocusMaterialID 从 Session.State 中提取焦点资料ID。
func getFocusMaterialID(session *model.Session) uint {
    if session.State == "" { return 0 }
    var state SessionState
    if json.Unmarshal([]byte(session.State), &state) == nil {
        return state.Context.FocusID
    }
    return 0
}
```

注：`SessionState` 和 `ContextData` 类型在 Task 7 的 prompts.go 中定义。此处先用 `interface{}` 或空结构体占位，Task 7 完成后更新。

- [ ] **Step 6: 编译验证**

```bash
go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add service/agent/safety.go
git commit -m "feat: 熔断器 + 工具边界 + 状态机 resolveMode"
```

---

### Task 6: prompts.go — 6 模块 Prompt 拼装

**Files:**
- Rewrite: `service/agent/prompts.go`

**Interfaces:**
- Consumes: model.Session, model.UserMemory (L3)
- Produces:
  - 6 个 Block 常量：`basePersonaBlock`, `shoppingModeBlock`, `tutoringModeBlock`, `supportModeBlock`, `rulesBlock`, `styleBlock`
  - `type SessionState` — State JSON 的 Go 结构体
  - `type ContextData` — context 子结构
  - `type FactItem` — 分层数据项
  - `func buildPrompt(session *model.Session) string`
  - `func buildStateBlock(state *SessionState) string`
  - `func buildUserContextBlock(userID uint, mode string) string`

- [ ] **Step 1: 定义 SessionState / ContextData / FactItem**

```go
// SessionState 会话任务状态快照。
type SessionState struct {
    Task       string      `json:"task"`
    Completed  []string    `json:"completed"`
    ToDo       []string    `json:"to_do"`
    Facts      []FactItem  `json:"facts"`
    Hypotheses []FactItem  `json:"hypotheses"`
    Discarded  []FactItem  `json:"discarded"`
    Context    ContextData `json:"context"`
}

// ContextData 业务上下文数据。
type ContextData struct {
    Candidates      []Candidate `json:"candidates,omitempty"`
    FocusID         uint        `json:"focus_id"`
    CardSent        bool        `json:"card_sent"`
    MaterialsViewed []uint      `json:"materials_viewed"`
}

// Candidate 候选资料。
type Candidate struct {
    ID    uint    `json:"id"`
    Title string  `json:"title"`
    Price float64 `json:"price"`
}

// FactItem 上下文分层数据项。
type FactItem struct {
    Content string `json:"content"`
    Source  string `json:"source"`
    Basis   string `json:"basis,omitempty"`
}
```

- [ ] **Step 2: 定义 6 个 Prompt Block 常量**

从 spec 第五章完整拷贝。每个 Block 为单独的 `const` 字符串。

```go
const basePersonaBlock = `你是 edu_market 的智能助手，负责三件事：
- 导购：帮用户找到合适的学习资料并完成购买
- 助教：用已购资料的文档内容解答用户疑问
- 客服：处理订单查询、退款、售后问题

当前处于「{{mode}}」模式，请专注当前模式的任务。`

const shoppingModeBlock = `【导购模式】
...` // 完整文本见 spec 5.2

const tutoringModeBlock = `【助教模式】
...` // 完整文本见 spec 5.2

const supportModeBlock = `【客服模式】
...` // 完整文本见 spec 5.2

const rulesBlock = `【核心规则 - 不可违反】
...` // 完整文本见 spec 5.5

const styleBlock = `【回复格式】
...` // 完整文本见 spec 5.6
```

- [ ] **Step 3: 实现 buildStateBlock**

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

- [ ] **Step 4: 实现 buildUserContextBlock**

```go
func buildUserContextBlock(userID uint, mode string) string {
    var memories []model.UserMemory
    database.DB.Where("user_id = ? AND status = 'active'", userID).Find(&memories)
    if len(memories) == 0 { return "" }
    
    var lines []string
    lines = append(lines, "【用户画像】")
    
    for _, m := range memories {
        switch mode {
        case "tutoring":
            if m.MemKey != "knowledge_level" && m.MemKey != "purchased_material_ids" { continue }
        case "support":
            if m.MemKey != "purchased_material_ids" { continue }
        } // shopping: 全量注入
        
        lines = append(lines, formatMemoryLine(m))
    }
    return strings.Join(lines, "\n")
}

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
```

- [ ] **Step 5: 实现 buildPrompt**

```go
func (e *AgentEngine) buildPrompt(session *model.Session) string {
    var parts []string
    
    parts = append(parts, basePersonaBlock)
    modeName := e.appendModeBlock(&parts, session.Mode)
    
    if session.State != "" {
        var state SessionState
        if json.Unmarshal([]byte(session.State), &state) == nil {
            parts = append(parts, buildStateBlock(&state))
        }
    }
    
    userBlock := buildUserContextBlock(session.UserID, session.Mode)
    if userBlock != "" {
        parts = append(parts, userBlock)
    }
    
    parts = append(parts, rulesBlock, styleBlock)
    
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
    default:
        *parts = append(*parts, shoppingModeBlock)
        *parts = append(*parts, tutoringModeBlock)
        *parts = append(*parts, supportModeBlock)
        return "通用"
    }
}
```

注意：`buildPrompt` 是 `AgentEngine` 的方法，但 `AgentEngine` 定义在 engine.go 中，此处只写函数体逻辑。Task 11 整合时统一放入 engine.go。

- [ ] **Step 6: 编译验证**

```bash
go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add service/agent/prompts.go
git commit -m "feat: 6 模块 Prompt 拼装 + SessionState + buildUserContextBlock"
```

---

### Task 7: memory.go — L3 长期记忆读写

**Files:**
- Create: `service/agent/memory.go`

**Interfaces:**
- Consumes: model.UserMemory, model.Session
- Produces:
  - `func loadUserMemories(userID uint) []model.UserMemory`
  - `func SaveUserMemory(userID uint, key, value, source string, confidence float64) error`
  - `func buildUserContextBlock(userID uint, mode string) string` (实现在 Task 6 的 prompts.go 中)
  - `func computeToDo(mode string, completed []string, ctx ContextData) []string`

- [ ] **Step 1: 创建文件并实现 L3 读取**

```go
package agent

import (
    "edu_market/database"
    "edu_market/model"
)

// loadUserMemories 加载用户所有活跃长期记忆。
func loadUserMemories(userID uint) []model.UserMemory {
    var memories []model.UserMemory
    database.DB.Where("user_id = ? AND status = 'active'", userID).Find(&memories)
    return memories
}
```

- [ ] **Step 2: 实现 L3 写入（带过筛）**

```go
// allowedKeys L3 Key 白名单。
var allowedKeys = map[string]bool{
    "knowledge_level":        true,
    "interest_tags":          true,
    "preferred_price_range":  true,
    "purchased_material_ids": true,
}

// SaveUserMemory 写入一条长期记忆到 L3，自动过筛。
func SaveUserMemory(userID uint, key, value, source string, confidence float64) error {
    // 1. Key 白名单
    if !allowedKeys[key] { return nil }

    // 2. Value 校验
    if err := validateMemoryValue(key, value); err != nil { return err }

    // 3. 去重 + 冲突合并
    var existing model.UserMemory
    result := database.DB.Where("user_id = ? AND mem_key = ?", userID, key).First(&existing)
    if result.Error == nil {
        // 旧值 confidence 更高 → 不覆盖
        if existing.Confidence > confidence { return nil }
        // 更新
        database.DB.Model(&existing).Updates(map[string]interface{}{
            "mem_value": value, "source": source, "confidence": confidence, "status": "active",
        })
        return nil
    }

    // 4. 新写入
    return database.DB.Create(&model.UserMemory{
        UserID: userID, MemKey: key, MemValue: value,
        Source: source, Confidence: confidence, Status: "active",
    }).Error
}

func validateMemoryValue(key, value string) error {
    switch key {
    case "knowledge_level":
        if value != "beginner" && value != "intermediate" && value != "advanced" {
            return fmt.Errorf("invalid knowledge_level: %s", value)
        }
    // 其余 key 暂不做格式校验（实现阶段完善）
    }
    return nil
}
```

- [ ] **Step 3: 实现 computeToDo**

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
    case "support":
        if !containsStr(completed, "查FAQ") {
            todos = append(todos, "如当前信息无法解决，查 search_faq")
        }
    }
    return todos
}

func containsStr(slice []string, substr string) bool {
    for _, s := range slice {
        if strings.Contains(s, substr) { return true }
    }
    return false
}
```

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add service/agent/memory.go
git commit -m "feat: L3 长期记忆读写 + computeToDo"
```

---

### Task 8: quality.go — 硬字段自动修正

**Files:**
- Create: `service/agent/quality.go`

**Interfaces:**
- Consumes: FactItem (from prompts.go)
- Produces:
  - `type HardFieldCorrector` + `Correct()`

- [ ] **Step 1: 创建文件并实现 HardFieldCorrector**

```go
package agent

import (
    "log/slog"
    "regexp"
    "strings"
)

// HardFieldCorrector 硬字段自动修正器。
// 比对 LLM 回答中的价格、金额、资料名与 facts 中的对应值，不一致则替换。
type HardFieldCorrector struct{}

// Correct 修正 LLM 回答中的硬字段偏差。
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

type hardField struct {
    field string
    value string
    raw   string
}

// extractHardFields 从 facts 中提取正确的硬字段值。
func extractHardFields(facts []FactItem) map[string]string {
    result := make(map[string]string)
    for _, f := range facts {
        // 提取价格：¥XX.XX 或 XX.XX元
        if re := regexp.MustCompile(`¥[\d.]+`); re.MatchString(f.Content) {
            result["price"] = re.FindString(f.Content)
        }
        // 提取资料名：书名号内容
        if re := regexp.MustCompile(`《([^》]+)》`); re.MatchString(f.Content) {
            result["title"] = re.FindString(f.Content)
        }
    }
    return result
}

// extractHardFieldsFromText 从 LLM 回答中提取声称的硬字段。
func extractHardFieldsFromText(text string) []hardField {
    var fields []hardField
    if re := regexp.MustCompile(`¥[\d.]+`); re.MatchString(text) {
        raw := re.FindString(text)
        fields = append(fields, hardField{field: "price", value: raw, raw: raw})
    }
    if re := regexp.MustCompile(`《([^》]+)》`); re.MatchString(text) {
        raw := re.FindString(text)
        fields = append(fields, hardField{field: "title", value: raw, raw: raw})
    }
    return fields
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/agent/quality.go
git commit -m "feat: quality.go 硬字段自动修正器"
```

---

### Task 9: engine.go — 上下文分层管理函数

**Files:**
- Modify: `service/agent/engine.go`

**Interfaces:**
- Consumes: SessionState, FactItem (prompts.go), CircuitBreaker, SemanticLoopDetector, ToolBudget (safety.go)
- Produces:
  - `func (e *AgentEngine) updateFactsAndHypotheses()`
  - `func (e *AgentEngine) expireOldFacts()`
  - `func (e *AgentEngine) updateTaskState()`
  - `func (e *AgentEngine) initState()`

在现有 engine.go 文件底部新增以下方法：

- [ ] **Step 1: 实现 updateFactsAndHypotheses**

```go
// updateFactsAndHypotheses 上下文分层维护。每轮 Tool 执行后调用。
func (e *AgentEngine) updateFactsAndHypotheses(state *SessionState, toolName, argsJSON string, result ToolResult) {
    source := fmt.Sprintf("%s(%s)", toolName, argsJSON)

    if result.Success {
        e.expireOldFacts(state, source)
        state.Facts = append(state.Facts, FactItem{
            Content: truncateRunes(result.Content, 150),
            Source:  source,
        })
    }

    // RAG 结果按 confidence 分流
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

注：`parseChunks` 从 RAG 返回的 JSON 中解析 chunk 数组，格式见 spec 2.5。

- [ ] **Step 2: 实现 updateTaskState**

```go
// updateTaskState 更新进度状态：completed / context / to_do。
func (e *AgentEngine) updateTaskState(state *SessionState, tool Tool, toolName, argsJSON string, result ToolResult) {
    if result.Success {
        desc := tool.Describe(argsJSON, result)
        state.Completed = append(state.Completed, desc)
    }

    // 更新业务上下文
    switch toolName {
    case "get_material_detail":
        var args struct{ MaterialID uint }
        json.Unmarshal([]byte(argsJSON), &args)
        state.Context.FocusID = args.MaterialID
        state.Context.MaterialsViewed = appendUnique(state.Context.MaterialsViewed, args.MaterialID)
    case "trigger_purchase_offer":
        state.Context.CardSent = true
    case "query_materials":
        state.Context.Candidates = extractCandidates(result.Content)
    }

    // 重新计算 to_do
    state.ToDo = computeToDo(getModeFromState(state), state.Completed, state.Context)
}

func appendUnique(slice []uint, v uint) []uint {
    for _, x := range slice {
        if x == v { return slice }
    }
    return append(slice, v)
}
```

- [ ] **Step 3: 实现 initState**

```go
// initState 初始化会话 State（task = userMsg，不调 LLM）。
func (e *AgentEngine) initState(userMsg string) string {
    state := SessionState{
        Task: userMsg,
        Context: ContextData{},
    }
    b, _ := json.Marshal(state)
    return string(b)
}
```

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add service/agent/engine.go
git commit -m "feat: 上下文分层 + State 管理函数"
```

---

### Task 10: engine.go — 重写 Run() 循环

**Files:**
- Modify: `service/agent/engine.go`

**Interfaces:**
- Consumes: 所有前序 Task 的产出
- Produces: 完整集成所有组件的新 Run() 循环

- [ ] **Step 1: 更新 engine.go 的 import 和 AgentEngine 字段**

```go
// 无需新增字段，buildPrompt 和 buildContext 通过 receiver 调用
// 去除旧的 SystemPromptV3 引用，改用 buildPrompt
```

- [ ] **Step 2: 替换 Run() 方法**

用 spec 第七章完整 Run() 循环替换现有实现。关键变化：

```go
func (e *AgentEngine) Run(session *model.Session, userMsg string,
    tools map[string]Tool, sseHandler SSEHandler, requestID string) error {

    // 0. 初始化
    breaker := &CircuitBreaker{}
    loopDetector := &SemanticLoopDetector{}
    budget := NewToolBudget()
    corrector := &HardFieldCorrector{}

    // 1. 存用户消息
    // 2. 初始化 State（新会话：initState）  
    // 3. buildPrompt + loadRecentMessages 拼接 history
    // 4. Tool Calling 循环：
    //    - Level 3 硬上限
    //    - callLLM（带 tools）
    //    - 有 tool_calls → 逐 tool 执行：
    //        Level 1 精确重复 → 白名单 → 参数校验 → 预算
    //        → Execute → Level 2 语义回路
    //        → updateFactsAndHypotheses → updateTaskState
    //        → resolveMode → saveSession
    //    - 无 tool_calls → streamFinalAnswer:
    //        callLLM（不带 tools）→ quality.Correct → streamAnswer → store + done
}
```

完整代码见 spec 第七章。

- [ ] **Step 3: 更新 AgentService.Chat 调用签名**

在 `service/agent/service.go` 中，将 `e.engine.Run(...)` 的参数更新：不再传 `systemPrompt`（引擎内部 buildPrompt 生成）。

- [ ] **Step 4: 处理未定义函数的 TODO**

确认以下函数存在或实现：
- `storeUserMessage()` — 已有（原 engine.go）
- `storeToolMessages()` — 已有
- `storeAssistantMessage()` — 已有
- `handleAction()` — 已有
- `streamAnswer()` — 已有或从原 engine.go 的 streamFinalAnswer 中提取
- `saveSession()` — 新增：`database.DB.Save(session)`
- `getFacts()` — 新增：从 state JSON 解析 facts 数组

- [ ] **Step 5: 错误恢复实现**

在 `callLLM` 调用处加重试逻辑：

```go
resp, err := e.callLLM(history, openAITools)
if err != nil {
    time.Sleep(3 * time.Second)
    resp, err = e.callLLM(history, openAITools)
    if err != nil {
        sseHandler("error", `{"message":"AI 服务暂时不可用，请稍后重试"}`)
        slog.Error("LLM 调用失败（已重试）", "request_id", requestID, "error", err)
        return err
    }
}
```

- [ ] **Step 6: 编译并通过现有测试**

```bash
go build ./...
go test ./service/agent/ -v -short
```
Expected: 编译通过，现有测试可能需要微调（因 Run() 签名变化）。

- [ ] **Step 7: Commit**

```bash
git add service/agent/engine.go service/agent/service.go
git commit -m "feat: 重写 Run() 循环，集成所有防护组件"
```

---

### Task 11: 控制器适配 + 清理

**Files:**
- Modify: `controller/agent_controller.go`
- Modify: `router/router.go` 或相关路由文件
- Delete: `service/agent_router.go` (already deleted in Task 1)
- Delete: `service/agent_router_test.go`

**Interfaces:**
- Consumes: Task 10 (新 AgentService 接口)
- Produces: 全链路打通

- [ ] **Step 1: 更新 controller/agent_controller.go**

Chat 方法中，`sseHandler` 和 `searchFunc` 类型改为 agent 包：

```go
import "edu_market/service/agent"

// ...

sseHandler := agent.SSEHandler(func(event string, data string) error { ... })
searchFunc := agent.SearchFunc(func(courseID uint, query string, topK int) (string, error) { ... })
```

- [ ] **Step 2: 确认路由注册**

检查 router 中 Agent 相关路由引用的是 agent 包：

```go
agentCtrl := controller.NewAgentController(agent.NewAgentService(agent.NewAgentEngine()))
```

- [ ] **Step 3: 删除测试文件**

```bash
rm service/agent_router_test.go
# 如果 agent_engine_test.go / agent_service_test.go 已移动，确认它们在新位置能跑
```

- [ ] **Step 4: 全量编译 + 运行现有测试**

```bash
go build ./...
go test ./...
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: 控制器适配 agent 包，清理废弃文件"
```

---

### Task 12: 单元测试

**Files:**
- Create: `service/agent/safety_test.go`
- Create: `service/agent/quality_test.go`

**Interfaces:**
- Consumes: safety.go + quality.go

- [ ] **Step 1: 写 safety_test.go — 熔断器测试**

```go
package agent

import "testing"

// TestCircuitBreaker_Check 精确重复检测
func TestCircuitBreaker_Check(t *testing.T) {
    cb := &CircuitBreaker{}
    
    // 第一轮：正常
    blocked, _ := cb.Check("query_materials", `{"keyword":"Python"}`)
    if blocked { t.Error("第一次调用不应该被拦截") }
    cb.Record("query_materials", `{"keyword":"Python"}`)

    // 第二轮：完全相同 → 拦截
    blocked, reason := cb.Check("query_materials", `{"keyword":"Python"}`)
    if !blocked { t.Error("第二次完全相同的调用应该被拦截") }
    if reason == "" { t.Error("拦截应该有 reason") }

    // 第三轮：不同参数 → 不拦截
    blocked, _ = cb.Check("query_materials", `{"keyword":"Java"}`)
    if blocked { t.Error("不同参数不应该被拦截") }
}

// TestSemanticLoopDetector_Feed 语义回路检测
func TestSemanticLoopDetector_Feed(t *testing.T) {
    d := &SemanticLoopDetector{}

    // 三轮完全不同 → 不触发
    blocked, _ := d.Feed("Python从入门到实战 ¥19.90 3章")
    blocked, _ = d.Feed("Java核心技术 ¥39.90 10章")
    blocked, _ = d.Feed("Go语言编程 ¥29.90 8章")
    if blocked { t.Error("三轮不同结果不应该触发回路") }

    // 三轮高度重复 → 触发
    d2 := &SemanticLoopDetector{}
    d2.Feed("平台暂无相关资料，建议换个方向试试看")
    d2.Feed("平台暂无相关资料，换个方向再试试")
    blocked, _ = d2.Feed("平台暂无相关资料，建议换个方向")
    if !blocked { t.Error("三轮重复结果应该触发回路") }
}

// TestResolveMode 模式切换
func TestResolveMode(t *testing.T) {
    session := &model.Session{Mode: "", UserID: 1, State: `{"context":{"focus_id":0}}`}
    
    mode := ResolveMode(session, []string{"query_materials"})
    if mode != "shopping" { t.Errorf("query_materials → shopping, got %s", mode) }

    mode = ResolveMode(session, []string{"query_orders"})
    if mode != "support" { t.Errorf("query_orders → support, got %s", mode) }

    mode = ResolveMode(session, []string{}) // 没调 tool
    if mode != "" { t.Errorf("没调 tool 应保持当前模式, got %s", mode) }
}

// TestToolBudget_Spend 调用预算
func TestToolBudget_Spend(t *testing.T) {
    b := NewToolBudget()
    for i := 0; i < 3; i++ {
        if err := b.Spend("trigger_purchase_offer"); err != nil {
            t.Errorf("第%d次调用不应超限: %v", i+1, err)
        }
    }
    if err := b.Spend("trigger_purchase_offer"); err == nil {
        t.Error("第4次调用应该超限")
    }
}
```

- [ ] **Step 2: 写 quality_test.go**

```go
func TestHardFieldCorrector_Correct(t *testing.T) {
    c := &HardFieldCorrector{}
    facts := []FactItem{
        {Content: "《Python 从入门到实战》¥19.90 3章", Source: "get_material_detail(2)"},
    }

    answer := "推荐《Python入门》，只要 ¥9.90，很划算"
    corrected := c.Correct(answer, facts)

    if !strings.Contains(corrected, "《Python 从入门到实战》") {
        t.Errorf("资料名应被修正, got: %s", corrected)
    }
    if !strings.Contains(corrected, "¥19.90") {
        t.Errorf("价格应被修正, got: %s", corrected)
    }
}
```

- [ ] **Step 3: 运行测试**

```bash
go test ./service/agent/ -v -run "TestCircuitBreaker|TestSemanticLoopDetector|TestResolveMode|TestToolBudget|TestHardFieldCorrector"
```

- [ ] **Step 4: Commit**

```bash
git add service/agent/safety_test.go service/agent/quality_test.go
git commit -m "test: 熔断器 + 状态机 + quality 单元测试"
```

---

### Task 13: 端到端验证

- [ ] **Step 1: 启动服务**

```bash
taskkill //F //IM main.exe 2>/dev/null
go run .
# 另一个终端：cd web && npm run dev
```

- [ ] **Step 2: 手动测试核心场景**

```
场景 A（导购）：
  1. 打开 AI 助手 → 新对话
  2. 输入"帮我找 Python 入门课"
  3. 确认收到 thinking → tool 执行 → 资料推荐
  4. 输入"第二个 ¥19.90，买"
  5. 确认收到 action 购买卡片
  6. 刷新页面 → 确认购买卡片依然显示

场景 B（导购→助教）：
  1. 输入"我上次买的 Python 入门，闭包怎么回事"
  2. 确认切到助教模式 → 调 search_documents → 回答

场景 C（客服）：
  1. 输入"我的订单"
  2. 确认切到客服模式 → 调 query_orders → 列订单
```

- [ ] **Step 3: 查看日志确认防护触发**

```bash
# 看日志中是否有模式切换、熔断触发等记录
grep "熔断\|模式切换\|quality" <日志文件>
```

- [ ] **Step 4: Commit**

```bash
git commit -m "verify: E2E 手动验证通过" --allow-empty
```
