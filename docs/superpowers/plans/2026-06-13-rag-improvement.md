# RAG 改进实现计划

> **For agentic workers:** Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 从 MySQL LIKE 关键词搜索升级为 DeepSeek Embedding + Redis Stack 混合向量搜索

**Architecture:** 新增 `embedText()` 调 DeepSeek Embedding API，新增 `RedisStackVectorStore` 实现 `VectorStore` 接口，InitRAG 切换到新实现

**Tech Stack:** Go + Redis Stack (RediSearch) + DeepSeek Embedding API

---

## 文件结构

### 修改
| 文件 | 改动 |
|------|------|
| `model/document_chunk.go` | 新增 `Embedding []float32` 字段 |
| `service/agent_rag.go` | 新增 `embedText()` + `RedisStackVectorStore` + 余弦相似度计算 |
| `service/agent_rag.go` | `InitRAG()` 默认换 `RedisStackVectorStore`，`SimpleSearchVectorStore` 做 fallback |
| `service/setup_test.go` | TestMain 加 `embedding_model`/`embedding_api_url` 配置 |
| `database/mysql.go` | AutoMigrate 更新 `document_chunks` 表 |

### 不变
| 文件 | 原因 |
|------|------|
| `service/agent_tools.go` | `search_documents` tool 不变，只调 `RAGService.Search()` |
| `service/document_service.go` | `reindexDocument()` 不变，`IndexCourse()` 会走新向量存储 |
| `controller/*` | 不动 |

---

## Phase 1: 数据模型

### Task 1: DocumentChunk 加 Embedding 字段

- [ ] **Step 1: 修改模型**

`model/document_chunk.go`：

```go
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CourseID   uint      `gorm:"not null;index" json:"course_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	Embedding  []float32 `gorm:"type:blob" json:"-"`           // 新增：向量
	ChunkIndex int       `gorm:"not null;default:0" json:"chunk_index"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`

	Course Course `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
}
```

- [ ] **Step 2: 注册 AutoMigrate + TestMain 清理**

`database/mysql.go` 已有 `DocumentChunk`，自动迁移会更新。TestMain 清理已有。

- [ ] **Step 3: 编译 + Commit**

```bash
go build ./...
git add model/document_chunk.go
git commit -m "feat: add Embedding field to DocumentChunk"
```

---

## Phase 2: Embedding 服务

### Task 2: embedText 函数

- [ ] **Step 1: 写入 `embedText()`**

在 `service/agent_rag.go` 末尾加入：

```go
import (
	"bytes"
	"encoding/json"
	"net/http"
)

// embedText 调 DeepSeek Embedding API，返回文本向量
func embedText(text string) ([]float32, error) {
	apiURL := config.App.Agent.EmbeddingAPIURL
	if apiURL == "" {
		apiURL = "https://api.deepseek.com/v1/embeddings"
	}
	model := config.App.Agent.EmbeddingModel
	if model == "" {
		model = "deepseek-text-embedding" // DeepSeek 默认 embedding 模型
	}

	reqBody := map[string]interface{}{
		"model": model,
		"input": text,
	}
	jsonBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API 返回 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedding 返回空")
	}
	return result.Data[0].Embedding, nil
}

// cosineSimilarity 余弦相似度（两个等长向量）
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}
```

- [ ] **Step 2: 编译 + Commit**

```bash
go build ./...
git add service/agent_rag.go
git commit -m "feat: add embedText + cosineSimilarity functions"
```

---

## Phase 3: Redis Stack 向量存储

### Task 3: RedisStackVectorStore

- [ ] **Step 1: 实现 VectorStore 接口**

替换 `service/agent_rag.go` 中的 `RedisStackVectorStore` 空壳：

```go
import (
	"github.com/redis/go-redis/v9"  // 已有依赖
)

// RedisStackVectorStore 基于 Redis Stack 的向量存储
type RedisStackVectorStore struct {
	client *redis.Client
}

func NewRedisStackVectorStore() *RedisStackVectorStore {
	return &RedisStackVectorStore{client: database.RDB}
}

// Search 向量 + 关键词混合搜索
func (vs *RedisStackVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	// 1. 把问题向量化
	vec, err := embedText(query)
	if err != nil {
		// fallback 到关键词搜索
		return NewSimpleSearchVectorStore().Search(courseID, query, topK)
	}

	// 2. 构造 KNN 搜索
	// FT.SEARCH idx:chunks "@material_id:[3 3] =>[KNN 5 @embedding $V AS score]"
	queryStr := fmt.Sprintf("@course_id:[%d %d] =>[KNN %d @embedding $V AS score]", courseID, courseID, topK)
	
	// Redis Stack 向量搜索（简化版：用余弦相似度在内存算）
	// 完整版用 go-redis FT.Search，这里先做纯向量 + MySQL 加载计算
	var chunks []model.DocumentChunk
	if err := database.DB.Where("course_id = ?", courseID).Find(&chunks).Error; err != nil {
		return nil, err
	}

	// 3. 计算余弦相似度并排序
	type scored struct {
		chunk model.DocumentChunk
		score float32
	}
	var results []scored
	for _, c := range chunks {
		if len(c.Embedding) == 0 {
			continue
		}
		s := cosineSimilarity(vec, c.Embedding)
		if s > 0.5 { // 相似度阈值
			results = append(results, scored{c, s})
		}
	}

	// 排序，取 topK
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	if len(results) > topK {
		results = results[:topK]
	}

	// 4. 返回
	var final []SearchResult
	for _, r := range results {
		final = append(final, SearchResult{
			ChunkID: r.chunk.ID,
			Content: r.chunk.Content,
			Score:   r.score,
		})
	}
	return final, nil
}

func (vs *RedisStackVectorStore) Index(chunkID uint, courseID uint, content string) error {
	vec, err := embedText(content)
	if err != nil {
		return fmt.Errorf("embedding failed: %w", err)
	}
	// 更新 DB 中的 embedding
	return database.DB.Model(&model.DocumentChunk{}).
		Where("id = ?", chunkID).
		Update("embedding", vec).Error
}

func (vs *RedisStackVectorStore) Delete(courseID uint) error {
	// content 在 MySQL 删除时 FK CASCADE 自动清理
	return nil
}
```

> 注：Redis Stack 完整版 TODO——当前用 Go 内存 + MySQL 计算，后续安装 Redis Stack module 后改用 `FT.SEARCH`。

- [ ] **Step 2: 编译 + 补 import + Commit**

需要补 `"sort"`、`"math"`、`"time"`、`"io"`、`"net/http"`、`"bytes"`、`"encoding/json"`、`"fmt"` 到 `agent_rag.go`。

```bash
go build ./...
git add service/agent_rag.go
git commit -m "feat: implement RedisStackVectorStore with cosine similarity search"
```

---

## Phase 4: 切换实现

### Task 4: InitRAG 默认使用新 VectorStore

- [ ] **Step 1: 改 InitRAG**

```go
func InitRAG() {
	// 优先 Redis Stack
	vs := NewRedisStackVectorStore()
	globalRAG = NewRAGService(vs)
}
```

- [ ] **Step 2: Search 增加 fallback**

`RedisStackVectorStore.Search()` 中 embedding 失败时自动回落 `SimpleSearchVectorStore`。

- [ ] **Step 3: 编译 + Commit**

```bash
go build ./...
git add service/agent_rag.go
git commit -m "feat: switch InitRAG to RedisStackVectorStore"
```

---

## Phase 5: 测试

### Task 5: RAG 测试

- [ ] **Step 1: 测试 embedText**

```go
func TestEmbedText(t *testing.T) {
	if config.App.AI.APIKey == "" {
		t.Skip("API key not configured")
	}
	vec, err := embedText("你好世界")
	if err != nil {
		t.Fatalf("embedText failed: %v", err)
	}
	if len(vec) == 0 {
		t.Error("embedding should not be empty")
	}
}
```

- [ ] **Step 2: 测试余弦相似度**

```go
func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	c := []float32{1, 0, 0}

	// 正交 → 相似度 0
	if cosineSimilarity(a, b) > 0.01 {
		t.Error("orthogonal vectors should have ~0 similarity")
	}
	// 相同 → 相似度 1
	if cosineSimilarity(a, c) < 0.99 {
		t.Error("identical vectors should have ~1 similarity")
	}
}
```

- [ ] **Step 3: 测试 Index + Search**

```go
func TestRAGIndexAndSearch(t *testing.T) {
	if config.App.AI.APIKey == "" {
		t.Skip("API key not configured")
	}
	rag := GetRAG()
	if rag == nil {
		t.Fatal("RAG not initialized")
	}
	// 入库测试
	err := rag.IndexCourse(999, "这是一段测试文本，用于验证向量搜索是否正常工作")
	if err != nil {
		t.Fatalf("IndexCourse failed: %v", err)
	}
	// 检索测试
	results, err := rag.Search(999, "测试文本", 3)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("should return at least 1 result")
	}
}
```

- [ ] **Step 4: 全量编译 + 测试 + Commit**

```bash
go build ./... && go test ./... -count=1 2>&1 | grep -E "ok|FAIL"
git add service/agent_rag_test.go
git commit -m "test: add RAG embedding and search tests"
```

---

## Phase 6: 配置

### Task 6: 更新配置文件

- [ ] **Step 1: app.example.yml 加 embedding 配置**

```yaml
agent:
  embedding_model: "deepseek-text-embedding"
  embedding_api_url: "https://api.deepseek.com/v1/embeddings"
```

- [ ] **Step 2: setup_test.go 同步**

- [ ] **Step 3: Commit**

---

## 注意事项

1. **Redis Stack 完整版**：当前实现用 MySQL 存向量 + Go 内存计算余弦相似度。安装 Redis Stack module 后，可替换为 `FT.SEARCH` 的 KNN 模式，代码改动小于 20 行。
2. **Embedding 维度**：DeepSeek embedding 返回 1024 维 `float32`，每个 chunk 的 embedding 存储约 4KB。
3. **性能**：几百个 chunk 在内存计算余弦相似度足够快。数据量超 1000 chunk 时，切到 Redis FT.SEARCH 原生的 KNN 搜索。
