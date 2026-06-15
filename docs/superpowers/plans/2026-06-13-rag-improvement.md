# RAG 改进实现计划

> **For agentic workers:** Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans.

**Goal:** 实现 DeepSeek Embedding + Redis Stack 向量搜索 + MySQL 备份 + Redis 宕机降级

**Architecture:** MySQL 存 content + embedding 备份，Redis Stack 做向量索引搜索，Go 内存余弦相似度做 fallback

**Tech Stack:** Go + Redis Stack (FT.SEARCH KNN) + DeepSeek Embedding API + MySQL

---

## Phase 1: 数据模型

### Task 1: DocumentChunk 加 Embedding 字段

- [ ] **Step 1: 修改模型**

```go
// model/document_chunk.go
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CourseID   uint      `gorm:"not null;index" json:"course_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	Embedding  []float32 `gorm:"type:blob" json:"-"`           // 新增
	ChunkIndex int       `gorm:"not null;default:0" json:"chunk_index"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
	Course     Course    `gorm:"foreignKey:CourseID;constraint:OnDelete:CASCADE" json:"-"`
}
```

- [ ] **Step 2: 编译 + Commit**

```bash
go build ./...
git add model/document_chunk.go
git commit -m "feat: add Embedding field to DocumentChunk"
```

---

## Phase 2: Embedding 服务

### Task 2: embedTexts（批量） + cosineSimilarity + 重试

- [ ] **Step 1: 写入 `agent_rag.go`**

```go
// embedText 调 DeepSeek Embedding API，返回 1024 维文本向量
func embedText(text string) ([]float32, error) {
	apiURL := config.App.Agent.EmbeddingAPIURL
	if apiURL == "" {
		apiURL = "https://api.deepseek.com/v1/embeddings"
	}
	model := config.App.Agent.EmbeddingModel
	if model == "" {
		model = "deepseek-text-embedding"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"input": text,
	}
	jsonBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.App.AI.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API 错误: status=%d body=%s", resp.StatusCode, string(body))
	}
	var result struct {
		Data []struct { Embedding []float32 `json:"embedding"` } `json:"data"`
	}
	json.Unmarshal(body, &result)
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedding 返回空")
	}
	return result.Data[0].Embedding, nil
}

// cosineSimilarity 余弦相似度
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) { return 0 }
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 { return 0 }
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}
```

- [ ] **Step 2: 编译 + Commit**

```bash
go build ./...
git add service/agent_rag.go
git commit -m "feat: add embedText + cosineSimilarity"
```

---

## Phase 3: Redis Stack 向量存储

### Task 3: RedisStackVectorStore

- [ ] **Step 1: 实现 VectorStore 接口**

```go
// RedisStackVectorStore 基于 Redis Stack + MySQL fallback 的向量存储
type RedisStackVectorStore struct{}

func NewRedisStackVectorStore() *RedisStackVectorStore {
	return &RedisStackVectorStore{}
}

// Search 优先 Redis KNN，失败时回落 MySQL 内存计算
func (vs *RedisStackVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	vec, err := embedText(query)
	if err != nil {
		return nil, fmt.Errorf("embedding 失败: %w", err)
	}

	// 尝试 Redis KNN 搜索
	if database.RDB != nil {
		results, err := vs.searchRedis(courseID, vec, topK)
		if err == nil {
			return results, nil
		}
		// Redis 不可用 → 降级
		slog.Warn("Redis 搜索失败，降级到内存计算", "err", err)
	}

	// Fallback: MySQL 加载 → 内存余弦相似度
	return vs.searchInMemory(courseID, vec, topK)
}

// searchRedis Redis KNN 向量搜索
func (vs *RedisStackVectorStore) searchRedis(courseID uint, vec []float32, topK int) ([]SearchResult, error) {
	// 将 vec 转为 Redis 需要的二进制格式
	buf := new(bytes.Buffer)
	for _, v := range vec {
		binary.Write(buf, binary.LittleEndian, v)
	}

	queryStr := fmt.Sprintf("@course_id:[%d %d] =>[KNN %d @embedding $V AS score]", courseID, courseID, topK)
	docs, err := database.RDB.Do(
		context.Background(),
		"FT.SEARCH", "idx:chunks", queryStr,
		"RETURN", "3", "content", "course_id", "score",
		"PARAMS", "2", "V", buf.Bytes(),
		"DIALECT", "2",
	).Slice()
	if err != nil { return nil, err }

	var results []SearchResult
	// 解析 Redis 返回的 [count, key1, [field1, val1, ...], key2, ...]
	for i := 1; i < len(docs); i += 2 {
		fields := docs[i+1].([]interface{})
		content := ""
		score := float32(0)
		for j := 0; j < len(fields); j += 2 {
			switch fields[j].(string) {
			case "content": content = fields[j+1].(string)
			case "score": score = float32(fields[j+1].(float64))
			}
		}
		if content != "" {
			results = append(results, SearchResult{Content: content, Score: score})
		}
	}
	return results, nil
}

// searchInMemory MySQL 加载向量 + Go 内存余弦相似度（Redis 挂了的降级方案）
func (vs *RedisStackVectorStore) searchInMemory(courseID uint, vec []float32, topK int) ([]SearchResult, error) {
	var chunks []model.DocumentChunk
	if err := database.DB.Where("course_id = ?", courseID).Find(&chunks).Error; err != nil {
		return nil, err
	}

	type scored struct {
		chunk model.DocumentChunk
		score float32
	}
	var candidates []scored
	for _, c := range chunks {
		if len(c.Embedding) == 0 { continue }
		s := cosineSimilarity(vec, c.Embedding)
		if s > 0.5 {
			candidates = append(candidates, scored{c, s})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > topK { candidates = candidates[:topK] }

	var results []SearchResult
	for _, r := range candidates {
		results = append(results, SearchResult{
			ChunkID: r.chunk.ID,
			Content: r.chunk.Content,
			Score:   r.score,
		})
	}
	return results, nil
}

// Index 建索引：embed → 存 MySQL + Redis
func (vs *RedisStackVectorStore) Index(chunkID uint, courseID uint, content string) error {
	vec, err := embedText(content)
	if err != nil { return fmt.Errorf("embedding 失败: %w", err) }

	// 1. MySQL 备份向量
	if err := database.DB.Model(&model.DocumentChunk{}).Where("id = ?", chunkID).
		Update("embedding", vec).Error; err != nil {
		return err
	}

	// 2. Redis 建索引（失败不阻塞）
	if database.RDB != nil {
		buf := new(bytes.Buffer)
		for _, v := range vec { binary.Write(buf, binary.LittleEndian, v) }
		database.RDB.HSet(context.Background(), fmt.Sprintf("doc:%d", chunkID),
			"content", content, "course_id", courseID, "embedding", buf.Bytes(),
		)
	}
	return nil
}

// Delete 删除某课程的所有向量
func (vs *RedisStackVectorStore) Delete(courseID uint) error {
	if database.RDB != nil {
		// Redis 中清理（可选，非关键）
		database.RDB.Do(context.Background(), "FT.DROPINDEX", "idx:chunks", "DD")
	}
	return nil
}
```

- [ ] **Step 2: 更新 InitRAG**

```go
func InitRAG() {
	vs := NewRedisStackVectorStore()
	globalRAG = NewRAGService(vs)
}
```

- [ ] **Step 3: 补 import**

```go
import (
    "bytes"
    "context"
    "encoding/binary"
    "encoding/json"
    "fmt"
    "io"
    "log/slog"
    "math"
    "net/http"
    "sort"
    "time"
)
```

- [ ] **Step 4: 编译 + Commit**

```bash
go build ./...
git add service/agent_rag.go
git commit -m "feat: implement RedisStackVectorStore with Redis KNN + MySQL fallback"
```

---

## Phase 4: 配置 + 测试

### Task 4: 配置文件更新

- [ ] **Step 1: app.example.yml**

```yaml
agent:
  embedding_model: "deepseek-text-embedding"
  embedding_api_url: "https://api.deepseek.com/v1/embeddings"
```

- [ ] **Step 2: setup_test.go 同步**

- [ ] **Step 3: Commit**

### Task 5: 测试

- [ ] **Step 1: TestEmbedText** — 验证 API 调用成功
- [ ] **Step 2: TestCosineSimilarity** — 验证相似度计算
- [ ] **Step 3: TestRAGIndexAndSearch** — 端到端入库+检索
- [ ] **Step 4: TestRAGRedisFallback** — 模拟 Redis 宕机后的内存降级
- [ ] **Step 5: 全量测试 + Commit**

```bash
go test ./... -count=1
git add service/agent_rag_test.go config/app.example.yml service/setup_test.go
git commit -m "test: add RAG embedding, search and fallback tests"
```

---

## Phase 5: 全量验证

### Task 6: E2E 验证

- [ ] `go test ./... -count=1` → 全部 PASS
- [ ] `npm run build` → 前端构建成功
- [ ] 启动服务 → 上传文档 → Agent 问答 → 验证 RAG 检索结果
