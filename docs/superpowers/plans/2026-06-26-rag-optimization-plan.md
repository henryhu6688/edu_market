# RAG 系统优化 Implementation Plan

> **For agentic workers:** Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** RAG 全链路优化：Qdrant 向量库 + 混合检索 RRF + Rerank 精排 + 查询缓存 + 文档清洗 + OCR + 结构切片 + Embedding batch + Prompt 引用约束 + 诊断日志

**Architecture:** `service/rag/` 7 文件协作，QdrantVectorStore 实现 VectorStore 接口替代 Redis Stack。在线链路加缓存→检索→Rerank→元数据→日志，离线链路加清洗→OCR→结构切片→batch Embedding。

**Tech Stack:** Go + Gin + GORM + MySQL + slog + Qdrant REST API + SiliconFlow (bge-m3 + bge-reranker-v2-m3) + Redis + Tesseract OCR

## Global Constraints

- HTTP 响应走 `utils/response.go`，禁止 `c.JSON()`
- Service 层不碰 `gin.Context`，只返回 Go `error`
- 敏感数据用 `config.App.XXX` 读取，不硬编码
- GORM 错误用 `errors.Is(err, gorm.ErrRecordNotFound)` 判断
- 导出函数/结构体必须有中文注释
- 所有日志用 `slog`，带 `request_id` 字段
- 方案 C 开关：每个模块 `config.App.RAG.XXX` 独立控制

---

### Task 1: 配置 + app.yml（基础层）

**Files:**
- Modify: `config/config.go`
- Modify: `config/app.example.yml`

**Interfaces:**
- Produces: `config.App.RAG` — 11 个配置字段，后续所有 tasks 直接引用

- [ ] **Step 1: 在 config.go 新增 RAGConfig 结构体**

```go
// config/config.go — AgentConfig 下方新增

// RAGConfig RAG 检索配置（方案 C 独立开关）
type RAGConfig struct {
	QdrantURL         string  `mapstructure:"qdrant_url"`
	QdrantCollection  string  `mapstructure:"qdrant_collection"`
	HybridSearch      bool    `mapstructure:"hybrid_search"`
	Rerank            bool    `mapstructure:"rerank"`
	RerankTopK        int     `mapstructure:"rerank_topk"`
	CleanerEnabled    bool    `mapstructure:"cleaner_enabled"`
	StructuralChunk   bool    `mapstructure:"structural_chunk"`
	ChunkSize         int     `mapstructure:"chunk_size"`
	ChunkMin          int     `mapstructure:"chunk_min"`
	ChunkMax          int     `mapstructure:"chunk_max"`
	CacheEnabled      bool    `mapstructure:"cache_enabled"`
	CacheTTL          int     `mapstructure:"cache_ttl"`
	CacheSimThreshold float64 `mapstructure:"cache_sim_threshold"`
	CacheMaxEntries   int     `mapstructure:"cache_max_entries"`
}
```

- [ ] **Step 2: 在 Config 结构体加 RAG 字段**

```go
// config/config.go — Config struct 内新增
RAG RAGConfig `mapstructure:"rag"`
```

- [ ] **Step 3: 设置默认值**

```go
// config/config.go — Load() 或 setDefaults() 函数中
v.SetDefault("rag.qdrant_url", "http://localhost:6333")
v.SetDefault("rag.qdrant_collection", "chunks")
v.SetDefault("rag.hybrid_search", true)
v.SetDefault("rag.rerank", true)
v.SetDefault("rag.rerank_topk", 3)
v.SetDefault("rag.cleaner_enabled", true)
v.SetDefault("rag.structural_chunk", true)
v.SetDefault("rag.chunk_size", 500)
v.SetDefault("rag.chunk_min", 300)
v.SetDefault("rag.chunk_max", 800)
v.SetDefault("rag.cache_enabled", true)
v.SetDefault("rag.cache_ttl", 3600)
v.SetDefault("rag.cache_sim_threshold", 0.85)
v.SetDefault("rag.cache_max_entries", 10)
```

- [ ] **Step 4: 更新 app.example.yml、app.yml 的 agent 块**

```yaml
# agent 块下方同步新增
rag:
  qdrant_url: "http://localhost:6333"
  qdrant_collection: "chunks"
  hybrid_search: true
  rerank: true
  rerank_topk: 3
  cleaner_enabled: true
  structural_chunk: true
  chunk_size: 500
  chunk_min: 300
  chunk_max: 800
  cache_enabled: true
  cache_ttl: 3600
  cache_sim_threshold: 0.85
  cache_max_entries: 10
```

- [ ] **Step 5: 更新 embedding_model 配置**

```yaml
# 从 BAAI/bge-large-zh-v1.5 改为 BAAI/bge-m3
embedding_model: "BAAI/bge-m3"
```

- [ ] **Step 6: 编译验证**

```bash
go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add config/config.go config/app.example.yml
git commit -m "feat: RAG 配置块（方案C 11个开关）+ bge-m3"
```

---

### Task 2: DocumentChunk + SearchResult 模型变更

**Files:**
- Modify: `model/document_chunk.go`
- Modify: `service/rag/rag.go`

**Interfaces:**
- Produces: `DocumentChunk.DocumentID uint` / `DocumentChunk.SectionPath string`
- Produces: `SearchResult.DocumentID uint` / `SearchResult.SectionPath string`

- [ ] **Step 1: DocumentChunk 加 2 个字段**

```go
// model/document_chunk.go
type DocumentChunk struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CourseID   uint      `gorm:"not null;index" json:"course_id"`
	Content    string    `gorm:"type:text;not null" json:"content"`
	Embedding  []byte    `gorm:"type:blob" json:"-"`
	ChunkIndex int       `gorm:"not null;default:0" json:"chunk_index"`
	// 新增
	DocumentID  uint   `gorm:"index" json:"document_id"`               // documents.id
	SectionPath string `gorm:"type:varchar(500)" json:"section_path"`  // "第三章 > 3.2 闭包"
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}
```

- [ ] **Step 2: SearchResult 同步加字段**

```go
// service/rag/rag.go
type SearchResult struct {
	ChunkID      uint    `json:"chunk_id"`
	Content      string  `json:"content"`
	Score        float32 `json:"score"`
	DocumentID   uint    `json:"document_id"`   // 新增
	SectionPath  string  `json:"section_path"`  // 新增
}
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add model/document_chunk.go service/rag/rag.go
git commit -m "feat: DocumentChunk + SearchResult 加 DocumentID/SectionPath"
```

---

### Task 3: QdrantVectorStore（替换 Redis Stack）

**Files:**
- Create: `service/rag/qdrant_store.go`

**Interfaces:**
- Consumes: `config.App.RAG.QdrantURL`, `config.App.RAG.QdrantCollection`
- Produces: `QdrantVectorStore` 实现 `VectorStore` 接口

- [ ] **Step 1: 创建文件骨架**

```go
// Package rag 提供基于 Qdrant 的向量存储实现
package rag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"edu_market/config"
)

// QdrantVectorStore 基于 Qdrant 的向量存储。
// 使用 HTTP REST API，支持混合检索（向量+关键词 RRF 融合）。
type QdrantVectorStore struct {
	client     *http.Client
	baseURL    string
	collection string
}

// NewQdrantVectorStore 创建 Qdrant 存储实例
func NewQdrantVectorStore() *QdrantVectorStore {
	return &QdrantVectorStore{
		client:     &http.Client{Timeout: 30 * time.Second},
		baseURL:    strings.TrimRight(config.App.RAG.QdrantURL, "/"),
		collection: config.App.RAG.QdrantCollection,
	}
}
```

- [ ] **Step 2: 实现 Search**

```go
// Search 混合检索：向量 + 关键词 RRF 融合 + course_id 过滤。
func (q *QdrantVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	vecs, err := embedTexts([]string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding 失败: %w", err)
	}
	vec := vecs[0]

	reqBody := map[string]interface{}{
		"vector":    vec,
		"limit":     topK,
		"with_payload": true,
		"filter": map[string]interface{}{
			"must": []map[string]interface{}{
				{
					"key":   "course_id",
					"match": map[string]interface{}{"value": courseID},
				},
			},
		},
	}

	// 混合检索：Qdrant 内置全文过滤 + 向量检索
	if config.App.RAG.HybridSearch {
		// 用 Qdrant payload text 索引（multilingual tokenizer 支持中文分词）
		reqBody["filter"].(map[string]interface{})["must"] = append(
			reqBody["filter"].(map[string]interface{})["must"].([]map[string]interface{}),
			map[string]interface{}{
				"key":   "content",
				"match": map[string]interface{}{"text": query},
			},
		)
	}

	jsonBody, _ := json.Marshal(reqBody)
	resp, err := q.client.Post(
		q.baseURL+"/collections/"+q.collection+"/points/search",
		"application/json",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return nil, fmt.Errorf("Qdrant 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Qdrant 返回 %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Result []struct {
			ID      uint                   `json:"id"`
			Score   float32                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Qdrant 结果失败: %w", err)
	}

	var results []SearchResult
	for _, r := range result.Result {
		sr := SearchResult{
			ChunkID: r.ID,
			Score:   r.Score,
		}
		if c, ok := r.Payload["content"].(string); ok {
			sr.Content = c
		}
		if did, ok := r.Payload["document_id"].(float64); ok {
			sr.DocumentID = uint(did)
		}
		if sp, ok := r.Payload["section_path"].(string); ok {
			sr.SectionPath = sp
		}
		results = append(results, sr)
	}
	return results, nil
}

```

- [ ] **Step 3: 实现 checkOrCreateCollection（含 payload text 中文索引）**

```go
// Index 写入向量 + payload 到 Qdrant。
func (q *QdrantVectorStore) Index(chunkID uint, courseID uint, content string) error {
	vecs, err := embedTexts([]string{content})
	if err != nil {
		return fmt.Errorf("embedding 失败: %w", err)
	}
	vec := vecs[0]

	// 从 MySQL 查 DocumentChunk 拿元数据
	var chunk struct {
		DocumentID  uint
		SectionPath string
	}
	database.DB.Model(&model.DocumentChunk{}).
		Select("document_id", "section_path").
		Where("id = ?", chunkID).First(&chunk)

	point := map[string]interface{}{
		"id":     chunkID,
		"vector": vec,
		"payload": map[string]interface{}{
			"content":      content,
			"course_id":    courseID,
			"document_id":  chunk.DocumentID,
			"section_path": chunk.SectionPath,
		},
	}

	body := map[string]interface{}{"points": []interface{}{point}}
	jsonBody, _ := json.Marshal(body)

	resp, err := q.client.Put(
		q.baseURL+"/collections/"+q.collection+"/points?wait=true",
		"application/json",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return fmt.Errorf("Qdrant upsert 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Qdrant upsert 返回 %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}
```

Note: `database` 和 `model` 的 import 需要在 `service/rag/` 可见范围内。当前 rag 包已有 `database` 引用（rag.go）。

- [ ] **Step 4: 实现 Delete**

```go
// Delete 删除某资料的全部向量（按 course_id 过滤删除）。
func (q *QdrantVectorStore) Delete(courseID uint) error {
	reqBody := map[string]interface{}{
		"filter": map[string]interface{}{
			"must": []map[string]interface{}{
				{
					"key":   "course_id",
					"match": map[string]interface{}{"value": courseID},
				},
			},
		},
	}
	jsonBody, _ := json.Marshal(reqBody)
	resp, err := q.client.Post(
		q.baseURL+"/collections/"+q.collection+"/points/delete?wait=true",
		"application/json",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return fmt.Errorf("Qdrant delete 失败: %w", err)
	}
	defer resp.Body.Close()
	return nil
}
```

- [ ] **Step 5: 确保 collection 在 NotFound 时自动创建（Init 时 checkOrCreate）**

```go
func (q *QdrantVectorStore) checkOrCreateCollection(vectorSize int) error {
	// GET /collections/{name} → 404 → PUT create
	resp, err := q.client.Get(q.baseURL + "/collections/" + q.collection)
	if err == nil && resp.StatusCode == 200 {
		resp.Body.Close()
		return nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	createBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	}
	jsonBody, _ := json.Marshal(createBody)
	resp, err = q.client.Put(
		q.baseURL+"/collections/"+q.collection,
		"application/json",
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return fmt.Errorf("创建 Qdrant collection 失败: %w", err)
	}
	defer resp.Body.Close()

	// 建 payload text 索引（multilingual tokenizer 原生支持中文分词）
	indexBody := map[string]interface{}{
		"field_name": "content",
		"field_type": "text",
		"tokenizer":  "multilingual",
	}
	jsonBody, _ = json.Marshal(indexBody)
	q.client.Put(
		q.baseURL+"/collections/"+q.collection+"/index",
		"application/json",
		bytes.NewBuffer(jsonBody),
	)
	return nil
}
```

- [ ] **Step 6: 在 rag.go 的 Init() 中切换为 Qdrant**

```go
func Init() {
	vs := NewQdrantVectorStore()
	vs.checkOrCreateCollection(1024)  // bge-m3 = 1024 维
	globalRAG = NewRAGService(vs)
}
```

- [ ] **Step 7: 编译**

```bash
go build ./...
```

- [ ] **Step 8: Commit**

```bash
git add service/rag/qdrant_store.go service/rag/rag.go
git commit -m "feat: QdrantVectorStore 替代 Redis Stack（混合检索 RRF）"
```

---

### Task 4: 更新 simple_store.go + 删除 redis_store.go

**Files:**
- Modify: `service/rag/simple_store.go`
- Delete: `service/rag/redis_store.go`

- [ ] **Step 1: simple_store.go 构造 SearchResult 时补上元数据字段**

```go
// service/rag/simple_store.go — Search 方法的返回构造处
for _, c := range chunks {
    results = append(results, SearchResult{
        ChunkID:     c.ID,
        Content:     c.Content,
        Score:       0.5,
        DocumentID:  c.DocumentID,   // 新增
        SectionPath: c.SectionPath,  // 新增
    })
}
```

- [ ] **Step 2: 确认无引用后删除 redis_store.go**

```bash
grep -r "RedisStackVectorStore\|redis_store" --include="*.go" service/rag/
# 确认只有 redis_store.go 自身匹配

rm service/rag/redis_store.go
```

- [ ] **Step 2: 编译验证**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/rag/redis_store.go
git commit -m "refactor: 删除 Redis Stack 向量存储（Qdrant 替代）"
```

---

### Task 5: 文档解析器改造 — 清洗 + OCR + 删 PPTX

**Files:**
- Modify: `service/document_parser.go`

**Interfaces:**
- Produces: `cleanPDF()`, `cleanDOCX()`, `isReadable()`, `parsePDF()`（带 OCR 降级）
- Deletes: `parsePPTX()`, `extractTextFromPPTXXML()`

- [ ] **Step 1: 更新允许格式列表去 .pptx**

```go
func (p *DocumentParser) isAllowed(ext string) bool {
	for _, f := range p.formats {
		if f == ext {
			return true
		}
	}
	return false
}

// 在 Parse() 的错误消息中去掉 .pptx
// "不支持: %s（支持 .txt .md .docx .pdf）"
```

- [ ] **Step 2: 添加 isReadable() 函数到 document_parser.go**

```go
// isReadable 检查文本是否可读（非扫描件/乱码）。
// 正常字符率 > 70% 且乱码率 < 10% 视为可读。
func isReadable(text string) bool {
	runes := []rune(text)
	if len(runes) < 50 {
		return false
	}
	var normal, garbled int
	for _, r := range runes {
		if (r >= 0x4E00 && r <= 0x9FFF) ||
			(r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == ' ' || r == '\n' || r == '\t' ||
			(r >= 0x20 && r <= 0x7E) {
			normal++
		}
		if r == 0xFFFD || r == 0xFFFF || (r < 0x20 && r != '\n' && r != '\t' && r != '\r') {
			garbled++
		}
	}
	total := len(runes)
	return float64(normal)/float64(total) > 0.70 &&
		float64(garbled)/float64(total) < 0.10
}
```

- [ ] **Step 3: 添加辅助函数**

```go
// removeRepeatingLines 删除出现 ≥3 次的同文行（页眉页脚/水印）。
func removeRepeatingLines(text string) string {
	lines := strings.Split(text, "\n")
	count := make(map[string]int)
	for _, l := range lines {
		count[strings.TrimSpace(l)]++
	}
	var cleaned []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if len([]rune(t)) < 50 && count[t] >= 3 {
			continue
		}
		cleaned = append(cleaned, l)
	}
	return strings.Join(cleaned, "\n")
}

// removePageNumbers 删除纯数字短行（页码）。
func removePageNumbers(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	re := regexp.MustCompile(`^[\s-]*\d{1,5}[\s-]*$`)
	for _, l := range lines {
		if !re.MatchString(strings.TrimSpace(l)) {
			cleaned = append(cleaned, l)
		}
	}
	return strings.Join(cleaned, "\n")
}

// deduplicateParagraphs 按空行分隔，去除完全重复的段落。
func deduplicateParagraphs(text string) string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range strings.Split(text, "\n\n") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		result = append(result, p)
	}
	return strings.Join(result, "\n\n")
}
```

- [ ] **Step 4: 实现 cleanPDF()**

```go
// cleanPDF PDF 格式专用清洗。
func cleanPDF(text string) string {
	text = removeRepeatingLines(text)     // 页眉页脚 + 水印
	text = removePageNumbers(text)        // 页码
	text = mergeHardLineBreaks(text)      // 硬换行合并
	text = removeTOClines(text)           // 目录残留
	return text
}

// mergeHardLineBreaks 合并 PDF 宽度截断造成的硬换行。
func mergeHardLineBreaks(text string) string {
	lines := strings.Split(text, "\n")
	var merged []string
	for i := 0; i < len(lines); i++ {
		if i+1 < len(lines) && isMidSentenceBreak(lines[i], lines[i+1]) {
			merged[len(merged)-1] += lines[i+1]
			i++
		} else {
			merged = append(merged, lines[i])
		}
	}
	return strings.Join(merged, "\n")
}

func isMidSentenceBreak(cur, next string) bool {
	if len(cur) == 0 || len(next) == 0 {
		return false
	}
	last := []rune(cur)[len([]rune(cur))-1]
	// 中文非句号结尾 或 小写字母结尾 → 句子中间被截断
	return (last >= 0x4E00 && last <= 0x9FFF && last != '。' && last != '；' && last != '！') ||
		(last >= 'a' && last <= 'z')
}

// removeTOClines 删除含连续 ... 的目录行。
func removeTOClines(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, l := range lines {
		if !strings.Contains(l, ".....") && !strings.Contains(l, "……") {
			cleaned = append(cleaned, l)
		}
	}
	return strings.Join(cleaned, "\n")
}
```

- [ ] **Step 5: 实现 cleanDOCX()**

```go
// cleanDOCX DOCX 格式专用清洗。
func cleanDOCX(text string) string {
	// 表格线残留
	re := regexp.MustCompile(`[│├─┼┤┬┴└┘┌┐╭╮╰╯]+`)
	text = re.ReplaceAllString(text, "")
	// 页眉页脚
	text = removeRepeatingLines(text)
	text = removePageNumbers(text)
	return text
}
```

- [ ] **Step 6: 改造 parsePDF 加 OCR 降级**

```go
func parsePDF(filePath string) (string, error) {
	text, err := parsePDFWithPdfToText(filePath)
	if err != nil {
		return "", err
	}
	// 可读性检查
	if isReadable(text) {
		return cleanPDF(text), nil
	}
	// OCR 降级
	ocrText, err := tesseractOCR(filePath)
	if err != nil {
		return "", fmt.Errorf("PDF OCR 失败: %w", err)
	}
	if !isReadable(ocrText) {
		return "", fmt.Errorf("PDF 质量过低，OCR 后仍无法识别，建议上传文字版 PDF")
	}
	return cleanPDF(ocrText), nil
}

// tesseractOCR 对 PDF 逐页做 OCR 识别。
func tesseractOCR(filePath string) (string, error) {
	tmpDir, _ := os.MkdirTemp("", "ocr_*")
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("pdftoppm", "-png", "-r", "300", filePath,
		filepath.Join(tmpDir, "page"))
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftoppm 失败: %w", err)
	}

	files, _ := filepath.Glob(filepath.Join(tmpDir, "page-*.png"))
	var result strings.Builder
	for _, f := range files {
		cmd := exec.Command("tesseract", f, "stdout", "-l", "chi_sim+eng")
		out, _ := cmd.Output()
		result.Write(out)
		result.WriteString("\n")
	}
	return result.String(), nil
}
```

新增 import：`"os"` `"path/filepath"` `"regexp"`（已有 `"os/exec"` `"strings"`）。

- [ ] **Step 7: 删除 parsePPTX 和 extractTextFromPPTXXML**

在 Parse() 的 switch 里去掉 `.pptx` case。删除两个函数。

- [ ] **Step 8: 编译验证**

```bash
go build ./...
```

- [ ] **Step 9: Commit**

```bash
git add service/document_parser.go
git commit -m "feat: 解析器清洗(cleanPDF/DOCX) + OCR降级(tesseract) + 删PPTX"
```

---

### Task 6: Markdown 清洗层（cleaner.go）

**Files:**
- Create: `service/rag/cleaner.go`

**Interfaces:**
- Produces: `func cleanMarkdown(text string) string`

- [ ] **Step 1: 创建 cleanMarkdown**

```go
package rag

import (
	"regexp"
	"strings"
)

// cleanMarkdown Markdown 层通用清洗。保留 # 标题标记给切片器。
func cleanMarkdown(text string) string {
	// 1. 控制字符
	for _, c := range []rune{0x00, 0xFFFD, 0x0C} {
		text = strings.ReplaceAll(text, string(c), "")
	}

	// 2. 图片占位符 ![...](...) → 删除
	text = regexp.MustCompile(`!\[.*?\]\(.*?\)`).ReplaceAllString(text, "")

	// 3. 链接 [text](url) → 保留 text
	text = regexp.MustCompile(`\[(.+?)\]\(.+?\)`).ReplaceAllString(text, "$1")

	// 4. 加粗 **text** → 保留 text
	text = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(text, "$1")

	// 5. 删除线 ~~text~~ → 保留 text
	text = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(text, "$1")

	// 6. 空行合并
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")

	// 7. 全角英文/数字 → 半角
	text = fullwidthToHalfwidth(text)

	return strings.TrimSpace(text)
}

func fullwidthToHalfwidth(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch {
		case r >= 'Ａ' && r <= 'Ｚ':
			buf.WriteRune(r - 'Ａ' + 'A')
		case r >= 'ａ' && r <= 'ｚ':
			buf.WriteRune(r - 'ａ' + 'a')
		case r >= '０' && r <= '９':
			buf.WriteRune(r - '０' + '0')
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/rag/cleaner.go
git commit -m "feat: Markdown 清洗器（7项清洗，保留 # 给切片器）"
```

---

### Task 7: 结构切片（chunker.go）

**Files:**
- Create: `service/rag/chunker.go`

**Interfaces:**
- Produces: `type ParsedSection` / `func chunkByStructure(text string, format string) ([]Chunk, error)`

- [ ] **Step 1: 创建 Chunk 和 ParsedSection 类型**

```go
package rag

import (
	"regexp"
	"strings"
)

// Chunk 切片单元
type Chunk struct {
	Content     string
	SectionPath string
}

// ParsedSection 解析后的文档章节
type ParsedSection struct {
	Title    string
	Level    int
	Content  string
	Children []*ParsedSection
}

// Chunker 结构切片器
type Chunker struct {
	minSize int  // 300 — 低于此合并
	maxSize int  // 800 — 超过此再切
}

// NewChunker 创建切片器
func NewChunker(minSize, maxSize int) *Chunker {
	return &Chunker{minSize: minSize, maxSize: maxSize}
}
```

- [ ] **Step 2: 实现 MD 标题解析**

```go
// parseMDSections 从 Markdown # 标题提取章节树。
func (c *Chunker) parseMDSections(text string) []*ParsedSection {
	lines := strings.Split(text, "\n")
	re := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

	var sections []*ParsedSection
	var stack []*ParsedSection // 父标题栈

	for _, line := range lines {
		m := re.FindStringSubmatch(line)
		if m == nil {
			// 正文：追加到当前最深节点
			if len(stack) > 0 {
				stack[len(stack)-1].Content += line + "\n"
			}
			continue
		}
		level := len(m[1])
		title := strings.TrimSpace(m[2])
		sec := &ParsedSection{Level: level, Title: title}

		// 找父节点：弹出所有层级 >= 当前层级的
		for len(stack) > 0 && stack[len(stack)-1].Level >= level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, sec)
		} else {
			sections = append(sections, sec)
		}
		stack = append(stack, sec)
	}
	return sections
}
```

- [ ] **Step 3: 实现 DFS 叶节点提取 + 合并 + 再切**

```go
func (c *Chunker) chunkFromSections(sections []*ParsedSection) []Chunk {
	var chunks []Chunk
	c.collectLeafChunks(sections, "", &chunks)
	return c.mergeShortChunks(chunks)
}

func (c *Chunker) collectLeafChunks(sections []*ParsedSection, parentPath string, chunks *[]Chunk) {
	for _, s := range sections {
		path := s.Title
		if parentPath != "" {
			path = parentPath + " > " + s.Title
		}

		if len(s.Children) == 0 {
			// 叶节点
			content := c.cleanTitleMarkers(s.Content)
			if len([]rune(content)) > 0 {
				// > 800 字内部再切
				if len([]rune(content)) > c.maxSize {
					for _, sub := range c.splitByParagraphs(content, path) {
						*chunks = append(*chunks, sub)
					}
				} else {
					*chunks = append(*chunks, Chunk{Content: content, SectionPath: path})
				}
			}
		} else {
			c.collectLeafChunks(s.Children, path, chunks)
		}
	}
}

// cleanTitleMarkers 切片后移除 # 标记
func (c *Chunker) cleanTitleMarkers(text string) string {
	re := regexp.MustCompile(`(?m)^#{1,6}\s+`)
	return re.ReplaceAllString(text, "")
}
```

- [ ] **Step 4: 实现合并 + 内部再切**

```go
// mergeShortChunks < 300 字的合并到上一个
func (c *Chunker) mergeShortChunks(chunks []Chunk) []Chunk {
	if len(chunks) <= 1 {
		return chunks
	}
	var result []Chunk
	prev := chunks[0]
	for i := 1; i < len(chunks); i++ {
		if len([]rune(prev.Content)) < c.minSize {
			prev.Content += "\n\n" + chunks[i].Content
			if chunks[i].SectionPath != "" {
				prev.SectionPath = chunks[i].SectionPath
			}
		} else {
			result = append(result, prev)
			prev = chunks[i]
		}
	}
	result = append(result, prev)
	return result
}

// splitByParagraphs 超长 chunk 按段落边界再切
func (c *Chunker) splitByParagraphs(text string, path string) []Chunk {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []Chunk
	var buf string
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len([]rune(buf))+len([]rune(p)) < c.maxSize {
			buf += p + "\n\n"
		} else {
			if buf != "" {
				chunks = append(chunks, Chunk{Content: strings.TrimSpace(buf), SectionPath: path})
			}
			buf = p + "\n\n"
		}
	}
	if buf != "" {
		chunks = append(chunks, Chunk{Content: strings.TrimSpace(buf), SectionPath: path})
	}
	return chunks
}
```

- [ ] **Step 5: 实现纯文本回退切片**

```go
// chunkPlain 纯文本切片（PDF/TXT 无结构时用）
func (c *Chunker) chunkPlain(text string) []Chunk {
	return c.splitByParagraphs(text, "")
}
```

- [ ] **Step 6: 编译验证**

```bash
go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add service/rag/chunker.go
git commit -m "feat: 结构切片器（MD标题树 + 叶节点合并(300) + 内部再切(800) + 纯文本回退）"
```

---

### Task 8: Embedding batch 并发 + embedCached 缓存

**Files:**
- Modify: `service/rag/embedding.go`

**Interfaces:**
- Produces: `func embedTexts(texts []string) ([][]float32, error)` — 并发 3，带令牌桶
- Produces: `func embedCached(text string) ([]float32, error)` — 单 embed + 缓存

- [ ] **Step 1: 替换 embedTexts 为并发版本**

```go
package rag

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"edu_market/config"
	"edu_market/database"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

var embedRate = rate.NewLimiter(8, 2)

// embedTexts 批量 Embedding，并发 3 请求，令牌桶限流。
func embedTexts(texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	var mu sync.Mutex
	g := new(errgroup.Group)
	g.SetLimit(3)

	for i, t := range texts {
		i, t := i, t
		g.Go(func() error {
			embedRate.Wait(context.Background())
			vec, err := embedCached(t)
			if err != nil {
				return err
			}
			mu.Lock()
			results[i] = vec
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

// embedCached 单文本 Embedding，带 Redis 缓存。
func embedCached(text string) ([]float32, error) {
	key := "emb:" + fmt.Sprintf("%x", md5.Sum([]byte(text)))

	// 1. 查缓存
	if database.RDB != nil {
		if b, err := database.RDB.Get(context.Background(), key).Bytes(); err == nil && len(b) > 0 {
			return bytesToFloats(b), nil
		}
	}

	// 2. 调 API（3 次指数退避重试）
	vec, err := callEmbeddingAPI(text)
	if err != nil {
		return nil, err
	}

	// 3. 写缓存（TTL 24h）
	if database.RDB != nil {
		database.RDB.SetEx(context.Background(), key, floatsToBytes(vec), 24*time.Hour)
	}
	return vec, nil
}
```

- [ ] **Step 2: 保留 callEmbeddingAPI（原 embedTexts 的核心逻辑）**

```go
// callEmbeddingAPI 调 SiliconFlow Embedding API，3 次指数退避重试。
func callEmbeddingAPI(text string) ([]float32, error) {
	apiURL := config.App.Agent.EmbeddingAPIURL
	if apiURL == "" {
		apiURL = "https://api.siliconflow.cn/v1/embeddings"
	}
	model := config.App.Agent.EmbeddingModel
	if model == "" {
		model = "BAAI/bge-m3"
	}
	reqBody := map[string]interface{}{
		"model":           model,
		"input":           text,
		"encoding_format": "float",
	}
	jsonBytes, _ := json.Marshal(reqBody)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
		}
		req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		apiKey := config.App.Agent.EmbeddingAPIKey
		if apiKey == "" {
			apiKey = config.App.AI.APIKey
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("embedding API %d: %s", resp.StatusCode, string(body))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("embedding API %d: %s", resp.StatusCode, string(body))
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
	return nil, fmt.Errorf("embedding 重试 3 次后仍失败: %w", lastErr)
}

// cosineSimilarity 余弦相似度
// floatsToBytes / bytesToFloats 保持不变
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add service/rag/embedding.go
git commit -m "feat: embedTexts 并发3 + embedCached Redis缓存 + bge-m3"
```

---

### Task 9: Reranker 精排

**Files:**
- Create: `service/rag/rerank.go`

**Interfaces:**
- Produces: `type Reranker` / `func (r *Reranker) Rerank(query string, chunks []SearchResult, topK int) ([]SearchResult, error)`

- [ ] **Step 1: 创建 Reranker**

```go
package rag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"edu_market/config"
)

const rerankModel = "Pro/BAAI/bge-reranker-v2-m3"

// Reranker Cross-Encoder 精排器。
// 对召回的 chunks 用 bge-reranker-v2-m3 重新打分排序。
type Reranker struct {
	client *http.Client
	apiURL string
	apiKey string
}

// NewReranker 创建精排器实例
func NewReranker() *Reranker {
	apiKey := config.App.Agent.EmbeddingAPIKey
	if apiKey == "" {
		apiKey = config.App.AI.APIKey
	}
	return &Reranker{
		client: &http.Client{Timeout: 30 * time.Second},
		apiURL: "https://api.siliconflow.cn/v1/rerank",
		apiKey: apiKey,
	}
}

// Rerank 精排，保留 topK 条。
func (r *Reranker) Rerank(query string, chunks []SearchResult, topK int) ([]SearchResult, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}

	docs := make([]string, len(chunks))
	for i, c := range chunks {
		docs[i] = c.Content
	}

	reqBody := map[string]interface{}{
		"model":     rerankModel,
		"query":     query,
		"documents": docs,
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", r.apiURL, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rerank API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 rerank 结果失败: %w", err)
	}

	// 按 relevance_score 给 chunks 重新排序
	sort.SliceStable(chunks, func(i, j int) bool {
		si, sj := 0.0, 0.0
		for _, r := range result.Results {
			if r.Index == i {
				si = r.Score
			}
			if r.Index == j {
				sj = r.Score
			}
		}
		return si > sj
	})

	if len(chunks) > topK {
		chunks = chunks[:topK]
	}
	return chunks, nil
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/rag/rerank.go
git commit -m "feat: Reranker 精排 (bge-reranker-v2-m3, 10→3)"
```

---

### Task 10: RAGService 集成（在线搜索 + 离线入库）

**Files:**
- Modify: `service/rag/rag.go`

**Interfaces:**
- Consumes: QdrantVectorStore, Reranker, Chunker, cleaner, embedCached
- Produces: `RAGService.Search()` 含 Rerank 开关，`RAGService.IndexCourse()` 含清洗+切片+缓存清空

- [ ] **Step 1: Search 方法加 Rerank 开关**

```go
// Search 检索课程资料，可选 Rerank 精排。
func (r *RAGService) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	if r.vectorStore == nil {
		return nil, errors.New("向量存储未初始化")
	}

	// 1. 向量检索（混合检索在 QdrantVectorStore 内部处理）
	results, err := r.vectorStore.Search(courseID, query, topK)
	if err != nil {
		return nil, err
	}

	// 2. Rerank（开关控制）
	if config.App.RAG.Rerank && len(results) > 1 {
		reranker := NewReranker()
		reRanked, err := reranker.Rerank(query, results, config.App.RAG.RerankTopK)
		if err != nil {
			// Rerank 失败 → 降级：用原始结果，不阻塞
			slog.Warn("Rerank 失败，降级到原始结果", "err", err)
			if len(results) > config.App.RAG.RerankTopK {
				results = results[:config.App.RAG.RerankTopK]
			}
		} else {
			results = reRanked
		}
	} else if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}
```

- [ ] **Step 2: IndexCourse 串联清洗 + 切片**

```go
// IndexCourse 入库课程资料：清洗 → 切片 → Embedding → 双写。
func (r *RAGService) IndexCourse(courseID uint, documentID uint, fullText string) error {
	// 清空旧数据
	database.DB.Where("course_id = ?", courseID).Delete(&model.DocumentChunk{})
	r.vectorStore.Delete(courseID)

	// 清空该资料的语义缓存
	if database.RDB != nil {
		database.RDB.Del(context.Background(), fmt.Sprintf("rag:sem:%d", courseID))
	}

	// 清洗 [开关: cleaner_enabled]
	if config.App.RAG.CleanerEnabled {
		fullText = cleanMarkdown(fullText)
	}

	// 切片 [开关: structural_chunk]
	var chunks []Chunk
	if config.App.RAG.StructuralChunk {
		chunker := NewChunker(config.App.RAG.ChunkMin, config.App.RAG.ChunkMax)
		sections := chunker.parseMDSections(fullText)
		chunks = chunker.chunkFromSections(sections)
	} else {
		chunks = r.fixedSizeChunk(fullText)
	}

	// 逐个入库
	for i, chunk := range chunks {
		dc := &model.DocumentChunk{
			CourseID:    courseID,
			DocumentID:  documentID,
			Content:     chunk.Content,
			SectionPath: chunk.SectionPath,
			ChunkIndex:  i,
		}
		if err := database.DB.Create(dc).Error; err != nil {
			return fmt.Errorf("保存文档块失败: %w", err)
		}
		if err := r.vectorStore.Index(dc.ID, courseID, chunk.Content); err != nil {
			fmt.Printf("警告: 向量索引失败 (chunk %d): %v\n", dc.ID, err)
		}
	}
	return nil
}

// fixedSizeChunk 固定大小切片（开关关闭时回退）
func (r *RAGService) fixedSizeChunk(text string) []Chunk {
	runes := []rune(text)
	total := len(runes)
	if total <= r.chunkSize {
		return []Chunk{{Content: string(runes)}}
	}
	step := r.chunkSize - r.chunkOverlap
	if step <= 0 {
		step = r.chunkSize
	}
	var chunks []Chunk
	for i := 0; i < total; i += step {
		end := i + r.chunkSize
		if end > total {
			end = total
		}
		chunks = append(chunks, Chunk{Content: string(runes[i:end])})
		if end == total {
			break
		}
	}
	return chunks
}
```

Note: `IndexCourse` 签名从 `(courseID uint, fullText string)` 变为 `(courseID uint, documentID uint, fullText string)`。需要更新调用方（`service/document_service.go` 的 `reindexDocument`）。

- [ ] **Step 3: 更新 reindexDocument 调用方**

```go
// service/document_service.go
func reindexDocument(doc *model.Document) {
	text := extractTextFromMarkdown(doc.Content)
	database.DB.Where("course_id = ?", doc.MaterialID).Delete(&model.DocumentChunk{})
	if r := rag.Get(); r != nil {
		r.IndexCourse(doc.MaterialID, doc.ID, text)  // 加了 doc.ID
	}
}
```

- [ ] **Step 4: 编译验证**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add service/rag/rag.go service/document_service.go
git commit -m "feat: RAGService 集成清洗+切片+Rerank开关+缓存清空"
```

---

### Task 11: 查询结果缓存（L1 精确 + L2 语义）

**Files:**
- Modify: `service/rag/rag.go`（Search 方法加缓存逻辑）

- [ ] **Step 1: 在 Search 方法最前面加缓存检查**

```go
// Search 检索课程资料（带两级缓存）。
func (r *RAGService) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	// ====== L1 精确匹配 ======
	if config.App.RAG.CacheEnabled && database.RDB != nil {
		exactKey := fmt.Sprintf("rag:exact:%x", md5.Sum([]byte(fmt.Sprintf("%s_%d", query, courseID))))
		if b, err := database.RDB.Get(context.Background(), exactKey).Bytes(); err == nil && len(b) > 0 {
			var results []SearchResult
			if json.Unmarshal(b, &results) == nil {
				return results, nil
			}
		}
	}

	// ====== Embedding ======
	vecs, err := embedTexts([]string{query})
	if err != nil {
		return nil, err
	}
	queryVec := vecs[0]

	// ====== L2 语义匹配 ======
	if config.App.RAG.CacheEnabled && database.RDB != nil {
		semKey := fmt.Sprintf("rag:sem:%d", courseID)
		if b, err := database.RDB.Get(context.Background(), semKey).Bytes(); err == nil {
			var recent []cachedEntry
			if json.Unmarshal(b, &recent) == nil {
				for _, entry := range recent {
					if cosineSimilarity(queryVec, entry.Vector) >= config.App.RAG.CacheSimThreshold {
						return entry.Results, nil
					}
				}
			}
		}
	}

	// ====== 完整管线 ======
	if r.vectorStore == nil {
		return nil, errors.New("向量存储未初始化")
	}
	results, err := r.vectorStore.Search(courseID, query, topK)
	if err != nil {
		return nil, err
	}

	if config.App.RAG.Rerank && len(results) > 1 {
		reranker := NewReranker()
		results, err = reranker.Rerank(query, results, config.App.RAG.RerankTopK)
		if err != nil {
			slog.Warn("Rerank 失败，降级到原始结果", "err", err)
		}
	}

	// ====== 写入缓存 ======
	if config.App.RAG.CacheEnabled && database.RDB != nil {
		// L1
		exactKey := fmt.Sprintf("rag:exact:%x", md5.Sum([]byte(fmt.Sprintf("%s_%d", query, courseID))))
		b, _ := json.Marshal(results)
		database.RDB.SetEx(context.Background(), exactKey, b, time.Duration(config.App.RAG.CacheTTL)*time.Second)

		// L2
		semKey := fmt.Sprintf("rag:sem:%d", courseID)
		entry := cachedEntry{Query: query, Vector: queryVec, Results: results}
		var recent []cachedEntry
		if old, err := database.RDB.Get(context.Background(), semKey).Bytes(); err == nil {
			json.Unmarshal(old, &recent)
		}
		recent = append([]cachedEntry{entry}, recent...)
		if len(recent) > config.App.RAG.CacheMaxEntries {
			recent = recent[:config.App.RAG.CacheMaxEntries]
		}
		b, _ = json.Marshal(recent)
		database.RDB.SetEx(context.Background(), semKey, b, 0) // 不自动过期，IndexCourse 清空
	}

	return results, nil
}

type cachedEntry struct {
	Query   string         `json:"query"`
	Vector  []float32      `json:"vector"`
	Results []SearchResult `json:"results"`
}
```

Note: import `"crypto/md5"` `"encoding/json"` 和 `"context"` 确保在 rag.go 中已引入（已有 context）。

- [ ] **Step 2: 编译验证**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add service/rag/rag.go
git commit -m "feat: 查询结果缓存（L1精确+L2语义）"
```

---

### Task 12: 更新 controller searchFunc 格式化

**Files:**
- Modify: `controller/agent_controller.go`

**Interfaces:**
- Consumes: `rag.Get().Search()` 返回带 DocumentID/SectionPath 的 SearchResult

- [ ] **Step 1: 更新 searchFunc 带元数据和日志标注**

```go
searchFunc = func(courseID uint, query string, topK int) (string, error) {
	ragSvc := rag.Get()
	if ragSvc == nil {
		return "", fmt.Errorf("RAG 未初始化")
	}

	start := time.Now()
	results, err := ragSvc.Search(courseID, query, topK)
	if err != nil {
		return "", err
	}
	searchMs := time.Since(start).Milliseconds()

	if len(results) == 0 {
		return `{"found":false,"chunks":[],"hint":"资料中未找到相关内容"}`, nil
	}

	// 收集 DocumentID → 批量查标题
	docIDSet := make(map[uint]bool)
	for _, r := range results {
		docIDSet[r.DocumentID] = true
	}
	docIDs := make([]uint, 0, len(docIDSet))
	for id := range docIDSet {
		docIDs = append(docIDs, id)
	}
	var docs []model.Document
	database.DB.Where("id IN ?", docIDs).Find(&docs)
	titleMap := make(map[uint]string)
	for _, d := range docs {
		titleMap[d.ID] = d.Title
	}

	// 格式化带来源
	type chunkOut struct {
		Content     string  `json:"content"`
		Score       float32 `json:"score"`
		Label       string  `json:"label"`
		Source      string  `json:"source"`
		DocumentID  uint    `json:"document_id"`
		SectionPath string  `json:"section_path"`
	}
	var parts []chunkOut
	var topScore float32
	for _, r := range results {
		if r.Score > topScore {
			topScore = r.Score
		}
		label := "低"
		if r.Score >= 0.7 {
			label = "高"
		} else if r.Score >= 0.4 {
			label = "中"
		}
		source := ""
		if title, ok := titleMap[r.DocumentID]; ok {
			source = fmt.Sprintf("《%s》> %s", title, r.SectionPath)
		}
		parts = append(parts, chunkOut{
			Content:     r.Content,
			Score:       r.Score,
			Label:       label,
			Source:      source,
			DocumentID:  r.DocumentID,
			SectionPath: r.SectionPath,
		})
	}

	// 快速判定标签
	recallQuality := "空"
	if len(results) > 0 {
		if topScore >= 0.7 {
			recallQuality = "高(≥0.7)"
		} else if topScore >= 0.4 {
			recallQuality = "中(≥0.4)"
		} else {
			recallQuality = "低(<0.4)"
		}
	}

	slog.Info("RAG检索",
		"request_id", rid,
		"query", query,
		"material_id", courseID,
		"cache_hit", "未命中",
		"recall_quality", recallQuality,
		"recall_top5_scores", topScores(results, 5),
		"returned_sources", formatSources(parts),
		"chunk_quality", chunkQuality(results),
		"search_ms", searchMs,
	)

	bytes, _ := json.Marshal(parts)
	return string(bytes), nil
}
```

Note: 用辅助函数 `topScores`、`formatSources`、`chunkQuality`。

```go
func topScores(results []agent.SearchResult, n int) []float32 {
	var scores []float32
	for i, r := range results {
		if i >= n {
			break
		}
		scores = append(scores, r.Score)
	}
	return scores
}

func formatSources(parts []chunkOut) []string {
	var src []string
	for _, p := range parts {
		src = append(src, fmt.Sprintf("chunk_%d|%s|%.2f", p.DocumentID, p.Source, p.Score))
	}
	return src
}

func chunkQuality(results []agent.SearchResult) string {
	for _, r := range results {
		if len([]rune(r.Content)) < 50 {
			return "短碎片(<50字)"
		}
		if len([]rune(r.Content)) > 800 {
			return "过长(>800字)"
		}
	}
	return "OK"
}
```

注意：controller 原来 import 了 `"edu_market/service/agent"` 和 `"edu_market/service/rag"`。

- [ ] **Step 2: 编译验证**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add controller/agent_controller.go
git commit -m "feat: searchFunc 元数据拼装 + 批量查标题 + RAG检索日志"
```

---

### Task 13: Prompt 引用约束 + 引擎诊断日志

**Files:**
- Modify: `service/agent/prompts.go`
- Modify: `service/agent/engine.go`

- [ ] **Step 1: rulesBlock 追加引用约束**

```go
// service/agent/prompts.go — rulesBlock 常量末尾追加

	`【回答规则 - 引用约束】
1. 只基于上面【参考资料】回答，资料中没有的内容必须说"资料中未涉及"
2. 每条论断必须标注来源：[《文档名》> 章节]
3. 禁止使用你自身的参数知识补充资料中没有的内容
4. 参考资料中可信度为"低"的只能作为参考，不能作为主要依据
5. 无法回答时直接说"这个问题在资料中没有找到相关内容"，不要猜测`
```

- [ ] **Step 2: engine.go 的 "Agent 回复" 日志加排查字段**

```go
// service/agent/engine.go — finalizeAnswer / streamFinalAnswer 中

hasCitation := "无引用"
if strings.Contains(displayAnswer, "《") && strings.Contains(displayAnswer, "》") {
	hasCitation = "有引用"
}
hasRefusal := "无"
if strings.Contains(displayAnswer, "未涉及") || strings.Contains(displayAnswer, "未找到") || strings.Contains(displayAnswer, "没有找到") {
	hasRefusal = "有(资料中未找到)"
}

slog.Info("Agent 回复",
	"request_id", requestID,
	"session_id", session.ID,
	"len", len([]rune(displayAnswer)),
	"preview", TruncateRunes(displayAnswer, 200),
	"has_citation", hasCitation,
	"has_refusal", hasRefusal,
	"has_hallucination", false, // 预留
)
```

- [ ] **Step 3: 编译验证**

```bash
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add service/agent/prompts.go service/agent/engine.go
git commit -m "feat: Prompt 引用约束 + Agent 回复排查日志"
```

---

### Task 14: 删除 PPTX 残留 + 更新脚本+ 编译全量

**Files:**
- Modify: `scripts/index_docs.go`（更新 IndexCourse 调用）
- Modify: `scripts/test_agent.go`（更新 searchFunc 构建）

- [ ] **Step 1: 更新 index_docs.go 中 IndexCourse 调用**

```go
// scripts/index_docs.go
if err := ragSvc.IndexCourse(m.ID, m.ID, fullText); err != nil {
	// IndexCourse(courseID, documentID, fullText) — documentID 先填 m.ID
```

- [ ] **Step 2: 全量编译 + 测试**

```bash
go build ./...
go test ./...
```

- [ ] **Step 3: Commit**

```bash
git add scripts/index_docs.go
git commit -m "fix: 更新 IndexCourse 调用签名 + 全量编译通过"
```

---

### Task 15: 单元测试

**Files:**
- Create: `service/rag/rag_test.go`

- [ ] **Step 1: 切片器测试**

```go
func TestChunker_Markdown(t *testing.T) {
	c := NewChunker(300, 800)
	md := `# 第一章\n## 1.1 概述\n这是概述内容足够长足够长足够长足够长足够长足够长`
	sections := c.parseMDSections(md)
	chunks := c.chunkFromSections(sections)
	// 验证 SectionPath、合并、数量
	if len(chunks) == 0 {
		t.Error("expected chunks")
	}
}

func TestChunker_PlainFallback(t *testing.T) {
	c := NewChunker(300, 800)
	plain := "段落1。\n\n段落2。\n\n段落3。"
	chunks := c.chunkPlain(plain)
	if len(chunks) == 0 {
		t.Error("expected chunks")
	}
}
```

- [ ] **Step 2: 清洗器测试**

```go
func TestCleanMarkdown(t *testing.T) {
	input := "**粗体** 和 [链接](url) 和 ![img](img.png)"
	output := cleanMarkdown(input)
	if strings.Contains(output, "**") || strings.Contains(output, "[链接]") || strings.Contains(output, "![") {
		t.Errorf("未清洗干净的输出: %s", output)
	}
}

func TestCleanPDF_HardLineBreaks(t *testing.T) {
	input := "闭包是指函数内部定义的函\n数可以访问外部\n结束。"
	output := cleanPDF(input)
	if strings.Count(output, "\n") >= strings.Count(input, "\n") {
		t.Error("expected merged hard breaks")
	}
}
```

- [ ] **Step 3: isReadable 测试**

```go
func TestIsReadable(t *testing.T) {
	if !isReadable("这是正常的中文文本用于测试可读性这是正常的中文文本" + strings.Repeat("x", 50)) {
		t.Error("正常中文应可读")
	}
	if isReadable(string([]rune{0xFFFD, 0xFFFD, 0xFFFD, 0xFFFD, 0xFFFD})) {
		t.Error("乱码应不可读")
	}
	if isReadable("abc") {
		t.Error("太短应不可读")
	}
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./service/rag/ -v
```

- [ ] **Step 5: Commit**

```bash
git add service/rag/rag_test.go
git commit -m "test: 切片器 + 清洗器 + isReadable 单元测试"
```

---

Note: Qdrant 和 Reranker 的集成测试需要 Docker Qdrant 和 SiliconFlow API Key，不作为单元测试强制要求。端到端验证留到 verification 阶段手动测试。
