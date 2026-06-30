# RAG 缓存访问权限隔离

**日期**: 2026-06-30
**分支**: v19_rag_refactor
**状态**: 待实现

## 问题

RAG 检索的 L1（精确匹配）和 L2（语义匹配）缓存 key 只包含 `query + courseID`，不区分用户访问权限。导致：

- 已购买用户搜索 → 缓存完整结果
- 未购买用户搜索同一问题 → L1/L2 命中 → 拿到同样的完整结果（数据泄漏）

当前唯一防护是 System Prompt 约束 LLM 不向未购买用户透露详情，属于软约束，不可靠。

## 方案

**缓存分池**：L1/L2 缓存 key 增加 `accessLevel` 维度，分 `full`（有权限）和 `preview`（无权限）两池。

不改 Search 返回内容（仍返回全量 chunk），不改 Embedding 缓存（与用户无关）。

## 改动

### 1. `SearchFunc` 类型签名（`service/agent/tools.go`）

```go
// 改前
type SearchFunc func(materialID uint, query string, topK int) (string, error)

// 改后
type SearchFunc func(materialID uint, query string, topK int, hasAccess bool) (string, error)
```

### 2. `searchMaterialsTool.Execute` 计算访问权限（`service/agent/tools.go`）

`Execute` 第一个参数 `userID`（当前被忽略）直接用于计算访问权限，`checkHasAccess` 已在 `safety.go` 中，同包直接调用：

```go
func (t searchMaterialsTool) Execute(userID uint, argsJSON string) ToolResult {
    var args struct {
        MaterialID uint   `json:"material_id"`
        Query      string `json:"query"`
    }
    // ...解析 args...
    hasAccess := checkHasAccess(userID, args.MaterialID)
    content, err := t.searchFunc(args.MaterialID, args.Query, 5, hasAccess)
    // ...
}
```

### 3. Controller 闭包透传（`controller/agent_controller.go`）

Controller 做纯透传，不引入 `checkHasAccess` 逻辑：

```go
searchFunc = func(courseID uint, query string, topK int, hasAccess bool) (string, error) {
    results, err := ragSvc.Search(courseID, query, topK, hasAccess)
    // ...格式化 results 不变...
}
```

### 4. `RAGService.Search` 签名和缓存 key（`service/rag/rag.go`）

```go
func (r *RAGService) Search(courseID uint, query string, topK int, hasAccess bool) ([]SearchResult, error) {
    accessLevel := "preview"
    if hasAccess {
        accessLevel = "full"
    }

    // L1: rag:exact:<MD5>:full  /  rag:exact:<MD5>:preview
    exactKey := fmt.Sprintf("rag:exact:%x:%s", md5.Sum([]byte(fmt.Sprintf("%s_%d", query, courseID))), accessLevel)

    // L2: rag:sem:<courseID>:full  /  rag:sem:<courseID>:preview
    semKey := fmt.Sprintf("rag:sem:%d:%s", courseID, accessLevel)

    // ...其余管线不变：L1查→L2查→Qdrant→Rerank→写回L1L2...
}
```

`IndexCourse` 清空语义缓存时同步清理两个池：

```go
database.RDB.Del(ctx, fmt.Sprintf("rag:sem:%d:full", courseID))
database.RDB.Del(ctx, fmt.Sprintf("rag:sem:%d:preview", courseID))
```

### 5. `scripts/test_agent.go` 适配

`buildSearchFunc` 闭包签名加 `hasAccess` 参数并透传。

## 不改的

- **Embedding 缓存** `emb:<MD5(text)>` — 相同文本永远产出相同向量，与用户无关
- **Search 返回内容** — 仍返回全量 chunk，不改 Qdrant 查询
- **`checkHasAccess` 逻辑** — 已判断发布者 + 已购买，不需修改

## 影响范围

| 文件 | 改动 |
|---|---|
| `service/agent/tools.go` | `SearchFunc` 签名 + `Execute` 传 `hasAccess` |
| `service/rag/rag.go` | `Search` 签名 + L1/L2 key 加 `accessLevel` + `IndexCourse` 清两个池 |
| `controller/agent_controller.go` | 闭包签名加参数 + 透传 |
| `scripts/test_agent.go` | `buildSearchFunc` 闭包适配 |

## 测试要点

1. 已购买用户搜索 → 缓存落在 `full` 池 → 未购买用户搜同一问题 → L1 不命中（key 不同）
2. 未购买用户搜索 → 缓存落在 `preview` 池 → 另一个未购买用户搜近义问题 → L2 命中 `preview` 池
3. `IndexCourse` 后两个池的语义缓存均被清空
4. Embedding 缓存不受影响，两个访问级别的相同文本命中同一 embedding 缓存
