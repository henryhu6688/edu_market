# RAG 缓存访问权限隔离 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** RAG L1/L2 缓存 key 按 `hasAccess` 分 `full`/`preview` 两池，隔离已购买和未购买用户的检索缓存。

**Architecture:** `Execute(userID)` → `checkHasAccess` → `hasAccess` → `SearchFunc(materialID, query, topK, hasAccess)` → Controller 透传 → `RAGService.Search(courseID, query, topK, hasAccess)` → L1 key `rag:exact:<MD5>:full|preview` / L2 key `rag:sem:<courseID>:full|preview`

**Tech Stack:** Go 1.22+, GORM, Redis, Qdrant

**Spec:** [2026-06-30-rag-cache-isolation-design.md](../specs/2026-06-30-rag-cache-isolation-design.md)

## Global Constraints

- 不改 Embedding 缓存 `emb:<MD5(text)>`
- 不改 Search 返回内容（仍全量 chunk）
- `checkHasAccess` 逻辑不变（发布者 + 已购买）
- `SearchFunc` 签名从 3 参数改为 4 参数，加 `hasAccess bool`

---

### Task 1: 改 `SearchFunc` 类型签名 + `Execute` 传参（tools.go）

**Files:**
- Modify: `service/agent/tools.go:52-53`（`SearchFunc` 类型）
- Modify: `service/agent/tools.go:222`（`Execute` 方法签名和逻辑）
- Modify: `service/agent/tools.go:232`（`searchFunc` 调用）

**Interfaces:**
- Consumes: `checkHasAccess(userID, materialID uint) bool`（已在 `safety.go` 中）
- Produces: `SearchFunc func(materialID uint, query string, topK int, hasAccess bool) (string, error)`

- [ ] **Step 1: 改 `SearchFunc` 类型签名**

```go
// 改前（line 52-53）
// SearchFunc RAG 检索函数类型（由 RAGService 注入，避免循环依赖）
type SearchFunc func(materialID uint, query string, topK int) (string, error)

// 改后
// SearchFunc RAG 检索函数类型（由 RAGService 注入，避免循环依赖）
type SearchFunc func(materialID uint, query string, topK int, hasAccess bool) (string, error)
```

- [ ] **Step 2: 改 `Execute` 方法——用 userID、算 hasAccess、传入 searchFunc**

```go
// 改前（line 222-240）
func (t searchMaterialsTool) Execute(_ uint, argsJSON string) ToolResult {
	var args struct {
		MaterialID uint   `json:"material_id"`
		Query      string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Success: false, Content: "参数解析失败，请调整参数格式后重试", ErrorCode: "INVALID_ARGUMENT", Recoverable: true, RecommendedAction: "fix_arguments_and_retry"}
	}
	if t.searchFunc == nil {
		return ToolResult{Success: false, Content: "资料检索服务暂不可用，请稍后重试", ErrorCode: "SERVICE_UNAVAILABLE", Recoverable: false, RecommendedAction: "tell_user_service_unavailable"}
	}
	content, err := t.searchFunc(args.MaterialID, args.Query, 5)
	// ...

// 改后
func (t searchMaterialsTool) Execute(userID uint, argsJSON string) ToolResult {
	var args struct {
		MaterialID uint   `json:"material_id"`
		Query      string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return ToolResult{Success: false, Content: "参数解析失败，请调整参数格式后重试", ErrorCode: "INVALID_ARGUMENT", Recoverable: true, RecommendedAction: "fix_arguments_and_retry"}
	}
	if t.searchFunc == nil {
		return ToolResult{Success: false, Content: "资料检索服务暂不可用，请稍后重试", ErrorCode: "SERVICE_UNAVAILABLE", Recoverable: false, RecommendedAction: "tell_user_service_unavailable"}
	}
	hasAccess := checkHasAccess(userID, args.MaterialID)
	content, err := t.searchFunc(args.MaterialID, args.Query, 5, hasAccess)
	// 后面不变...
```

- [ ] **Step 3: 提交**

```bash
git add service/agent/tools.go
git commit -m "feat: SearchFunc 签名加 hasAccess，Execute 计算并传入"
```

---

### Task 2: 改 `RAGService.Search` 签名 + 缓存 key 分池（rag.go）

**Files:**
- Modify: `service/rag/rag.go:142`（`Search` 方法签名）
- Modify: `service/rag/rag.go:148`（L1 key 拼接）
- Modify: `service/rag/rag.go:160-166`（L2 key 拼接）
- Modify: `service/rag/rag.go:217`（L1 缓存写入 key）
- Modify: `service/rag/rag.go:230-231`（L2 缓存写入 key）

**Interfaces:**
- Consumes: `hasAccess bool`
- Produces: `Search(courseID uint, query string, topK int, hasAccess bool) ([]SearchResult, error)`

- [ ] **Step 1: 改 `Search` 方法签名**

```go
// 改前（line 142）
func (r *RAGService) Search(courseID uint, query string, topK int) ([]SearchResult, error) {

// 改后
func (r *RAGService) Search(courseID uint, query string, topK int, hasAccess bool) ([]SearchResult, error) {
	queryPreview := truncateStr(query, 60)
	pipeStart := time.Now()

	accessLevel := "preview"
	if hasAccess {
		accessLevel = "full"
	}
```

- [ ] **Step 2: 改 L1 精确缓存 key（读取 + 写入两处）**

```go
// 读取（line 148）——改前
exactKey := fmt.Sprintf("rag:exact:%x", md5.Sum([]byte(fmt.Sprintf("%s_%d", query, courseID))))

// 读取——改后
exactKey := fmt.Sprintf("rag:exact:%x:%s", md5.Sum([]byte(fmt.Sprintf("%s_%d", query, courseID))), accessLevel)
```

```go
// 写入（line 217）——改前
exactKey := fmt.Sprintf("rag:exact:%x", md5.Sum([]byte(fmt.Sprintf("%s_%d", query, courseID))))

// 写入——改后（用同一 exactKey，方法开头已算好 accessLevel）
```

实际上写入在方法末尾，`accessLevel` 变量已定义，直接用 `exactKey` 同变量即可——两处都用同一个 `exactKey`。

- [ ] **Step 3: 改 L2 语义缓存 key（读取 + 写入两处）**

```go
// 读取（line 166）——改前
semKey := fmt.Sprintf("rag:sem:%d", courseID)

// 读取——改后
semKey := fmt.Sprintf("rag:sem:%d:%s", courseID, accessLevel)
```

```go
// 写入（line 231）——改前
semKey := fmt.Sprintf("rag:sem:%d", courseID)

// 写入——改后
semKey := fmt.Sprintf("rag:sem:%d:%s", courseID, accessLevel)
```

- [ ] **Step 4: 改 `IndexCourse` 清空两个池的语义缓存**

```go
// 改前（line 71-72）
if database.RDB != nil {
    database.RDB.Del(context.Background(), fmt.Sprintf("rag:sem:%d", courseID))
}

// 改后
if database.RDB != nil {
    database.RDB.Del(context.Background(),
        fmt.Sprintf("rag:sem:%d:full", courseID),
        fmt.Sprintf("rag:sem:%d:preview", courseID),
    )
}
```

- [ ] **Step 5: 提交**

```bash
git add service/rag/rag.go
git commit -m "feat: RAG Search 缓存 key 按 hasAccess 分 full/preview 两池"
```

---

### Task 3: 改 Controller 闭包透传 `hasAccess`（agent_controller.go）

**Files:**
- Modify: `controller/agent_controller.go:61`（SearchFunc 闭包）

**Interfaces:**
- Consumes: `SearchFunc` 新签名（4 参数）
- Produces: 无新增接口

- [ ] **Step 1: 闭包签名加 `hasAccess` 并透传给 `ragSvc.Search`**

```go
// 改前（line 61-62）
searchFunc = func(courseID uint, query string, topK int) (string, error) {
    start := time.Now()
    results, err := ragSvc.Search(courseID, query, topK)

// 改后
searchFunc = func(courseID uint, query string, topK int, hasAccess bool) (string, error) {
    start := time.Now()
    results, err := ragSvc.Search(courseID, query, topK, hasAccess)
```

后续格式化逻辑完全不变。

- [ ] **Step 2: 提交**

```bash
git add controller/agent_controller.go
git commit -m "feat: Controller SearchFunc 闭包透传 hasAccess"
```

---

### Task 4: 改 `scripts/test_agent.go` 适配新签名

**Files:**
- Modify: `scripts/test_agent.go:170-189`（`buildSearchFunc`）

**Interfaces:**
- Consumes: `SearchFunc` 新签名（4 参数）
- Produces: 无新增接口

- [ ] **Step 1: `buildSearchFunc` 闭包签名适配**

```go
// 改前（line 175-176）
return func(courseID uint, query string, topK int) (string, error) {
    results, err := ragSvc.Search(courseID, query, topK)

// 改后
return func(courseID uint, query string, topK int, hasAccess bool) (string, error) {
    results, err := ragSvc.Search(courseID, query, topK, hasAccess)
```

- [ ] **Step 2: 提交**

```bash
git add scripts/test_agent.go
git commit -m "fix: test_agent buildSearchFunc 适配 SearchFunc 新签名"
```

---

### Task 5: 构建验证 + 全量测试

**Files:**
- 无新建/修改

- [ ] **Step 1: 构建检查**

```bash
cd d:/Vscoding/edu_market && go build ./...
```

预期：无编译错误。

- [ ] **Step 2: 运行测试**

```bash
go test ./service/ -v -run TestAgent 2>&1 | head -50
go test ./service/rag/ -v 2>&1 | head -50
```

预期：现有测试通过。

- [ ] **Step 3: 旧缓存兼容性确认**

旧 key 格式 `rag:exact:<MD5>` 和 `rag:sem:<courseID>` 不会被清理，但新请求只会读写新 key 格式。旧 key 随 TTL 到期自然淘汰（L1 TTL 3600s，L2 无 TTL 但 `IndexCourse` 全量重建时会清）。可接受的迁移行为。

```bash
git add -A && git commit -m "chore: 构建验证通过，缓存分池改动完成"
```

---

### Task 6: 最终检查清单

- [ ] **Step 1: 确认所有 `SearchFunc` 调用点已更新**

```bash
grep -rn "SearchFunc" --include="*.go" | grep -v "_test.go" | grep -v vendor
```

预期：3 处——`tools.go` 类型定义、`agent_controller.go` 闭包、`test_agent.go` 闭包。全部 4 参数。

- [ ] **Step 2: 确认 `rag.Search` 所有调用点已更新**

```bash
grep -rn "\.Search(" --include="*.go" | grep rag
```

预期：`agent_controller.go`、`test_agent.go`、`rag_test.go`（如有）。全部传 `hasAccess`。

- [ ] **Step 3: 确认缓存 key 格式一致**

```bash
grep -rn "rag:exact:" service/rag/rag.go
grep -rn "rag:sem:" service/rag/rag.go
```

预期：所有 key 末尾带 `:%s` 或 `:full`/`:preview`。

- [ ] **Step 4: 提交**

```bash
git commit -m "chore: 最终检查清单通过" --allow-empty
```
