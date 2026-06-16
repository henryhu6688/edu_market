package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// SearchResult 检索结果
type SearchResult struct {
	ChunkID uint    `json:"chunk_id"`
	Content string  `json:"content"`
	Score   float32 `json:"score"`
}

// VectorStore 向量存储接口（预留切换 Pinecone/Qdrant）
type VectorStore interface {
	// Search 向量相似度搜索
	Search(courseID uint, query string, topK int) ([]SearchResult, error)
	// Index 写入向量索引
	Index(chunkID uint, courseID uint, content string) error
	// Delete 删除某课程的所有向量
	Delete(courseID uint) error
}

// RAGService RAG 服务
type RAGService struct {
	vectorStore  VectorStore
	chunkSize    int
	chunkOverlap int
}

// NewRAGService 创建 RAG 服务
func NewRAGService(vs VectorStore) *RAGService {
	cfg := config.App.Agent
	return &RAGService{
		vectorStore:  vs,
		chunkSize:    cfg.ChunkSize,
		chunkOverlap: cfg.ChunkOverlap,
	}
}

// IndexCourse 入库课程资料：切片 + 存 DB + 向量索引
func (r *RAGService) IndexCourse(courseID uint, fullText string) error {
	// 清空旧数据
	database.DB.Where("course_id = ?", courseID).Delete(&model.DocumentChunk{})
	r.vectorStore.Delete(courseID)

	// 切片
	chunks := r.chunkText(fullText)

	// 逐个入库
	for i, chunk := range chunks {
		dc := &model.DocumentChunk{
			CourseID:   courseID,
			Content:    chunk,
			ChunkIndex: i,
		}
		if err := database.DB.Create(dc).Error; err != nil {
			return fmt.Errorf("保存文档块失败: %w", err)
		}
		// 向量索引（忽略错误，非关键路径）
		if err := r.vectorStore.Index(dc.ID, courseID, chunk); err != nil {
			fmt.Printf("警告: 向量索引失败 (chunk %d): %v\n", dc.ID, err)
		}
	}
	return nil
}

// Search 检索课程资料
func (r *RAGService) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	if r.vectorStore == nil {
		return nil, errors.New("向量存储未初始化")
	}
	return r.vectorStore.Search(courseID, query, topK)
}

// chunkText 文本切片：每 chunkSize 字一块，重叠 chunkOverlap 字
func (r *RAGService) chunkText(text string) []string {
	var chunks []string
	runes := []rune(text)
	total := len(runes)

	if total <= r.chunkSize {
		return []string{text}
	}

	step := r.chunkSize - r.chunkOverlap
	if step <= 0 {
		step = r.chunkSize
	}

	for i := 0; i < total; i += step {
		end := i + r.chunkSize
		if end > total {
			end = total
		}
		chunk := string(runes[i:end])
		chunks = append(chunks, strings.TrimSpace(chunk))
		if end == total {
			break
		}
	}
	return chunks
}

// ============ 简易向量存储实现（MySQL 关键词搜索） ============

// SimpleSearchVectorStore 纯 MySQL 关键词搜索实现
type SimpleSearchVectorStore struct{}

// NewSimpleSearchVectorStore 创建简易搜索
func NewSimpleSearchVectorStore() *SimpleSearchVectorStore {
	return &SimpleSearchVectorStore{}
}

// Search 通过 MySQL LIKE 做关键词检索
func (vs *SimpleSearchVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	var chunks []model.DocumentChunk
	keywords := strings.Fields(query)
	if len(keywords) == 0 {
		return nil, nil
	}

	// 构建 OR 条件（用括号包裹，避免影响外层 course_id 约束）
	likeClauses := make([]string, 0, len(keywords))
	likeArgs := make([]interface{}, 0, len(keywords))
	for _, kw := range keywords {
		likeClauses = append(likeClauses, "content LIKE ?")
		likeArgs = append(likeArgs, "%"+kw+"%")
	}
	orCondition := "(" + strings.Join(likeClauses, " OR ") + ")"

	queryArgs := append([]interface{}{courseID}, likeArgs...)
	if err := database.DB.Where("course_id = ? AND "+orCondition, queryArgs...).
		Order("chunk_index ASC").Limit(topK).Find(&chunks).Error; err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, c := range chunks {
		results = append(results, SearchResult{
			ChunkID: c.ID,
			Content: c.Content,
			Score:   0.5,
		})
	}
	return results, nil
}

// Index 简易模式不建向量索引
func (vs *SimpleSearchVectorStore) Index(chunkID uint, courseID uint, content string) error {
	return nil
}

// Delete 清理时 document_chunks 表通过 FK CASCADE 自动清理
func (vs *SimpleSearchVectorStore) Delete(courseID uint) error {
	return nil
}

// ============ 全局 RAG ============

var globalRAG *RAGService

// InitRAG 初始化全局 RAG 服务
// ============ Embedding 服务 ============

// embedTexts 批量调 DeepSeek Embedding API，返回多个文本的向量
func embedTexts(texts []string) ([][]float32, error) {
	apiURL := config.App.Agent.EmbeddingAPIURL
	if apiURL == "" {
		apiURL = "https://api.deepseek.com/v1/embeddings"
	}
	model := config.App.Agent.EmbeddingModel
	if model == "" {
		model = "deepseek-text-embedding"
	}

	input := texts[0]
	if len(texts) > 1 {
		input = strings.Join(texts, "\n")
	}
	reqBody := map[string]interface{}{
		"model":           model,
		"input":           input,
		"encoding_format": "float",
	}
	jsonBytes, _ := json.Marshal(reqBody)

	// 3 次指数退避重试
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second) // 0s, 1s, 2s
		}

		EmbedSem.Acquire()
		req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			EmbedSem.Release()
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
			EmbedSem.Release()
			lastErr = fmt.Errorf("embedding API 返回状态 %d: %s", resp.StatusCode, string(body))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			EmbedSem.Release()
			return nil, fmt.Errorf("embedding API 错误: status=%d body=%s", resp.StatusCode, string(body))
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

		embeddings := make([][]float32, len(result.Data))
		for i, d := range result.Data {
			embeddings[i] = d.Embedding
		}
		EmbedSem.Release()
		return embeddings, nil
	}
	return nil, fmt.Errorf("embedding 重试 3 次后仍失败: %w", lastErr)
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

// ============ Redis Stack 向量存储 ============

// RedisStackVectorStore 基于 Redis Stack + MySQL 双写的高性能向量存储
type RedisStackVectorStore struct{}

// NewRedisStackVectorStore 创建实例
func NewRedisStackVectorStore() *RedisStackVectorStore {
	return &RedisStackVectorStore{}
}

// Search 优先 Redis KNN，失败时降级 MySQL 内存计算
func (vs *RedisStackVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	vecs, err := embedTexts([]string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding 失败: %w", err)
	}
	vec := vecs[0]

	// 尝试 Redis KNN
	if database.RDB != nil {
		results, err := vs.searchRedis(courseID, vec, topK)
		if err == nil {
			return results, nil
		}
		slog.Warn("Redis 搜索失败，降级到内存计算", "err", err)
	}
	return vs.searchInMemory(courseID, vec, topK)
}

// searchRedis Redis KNN 向量搜索
func (vs *RedisStackVectorStore) searchRedis(courseID uint, vec []float32, topK int) ([]SearchResult, error) {
	buf := new(bytes.Buffer)
	for _, v := range vec {
		_ = binary.Write(buf, binary.LittleEndian, v)
	}

	queryStr := fmt.Sprintf("@course_id:[%d %d] =>[KNN %d @embedding $V AS score]",
		courseID, courseID, topK)

	docs, err := database.RDB.Do(
		context.Background(),
		"FT.SEARCH", "idx:chunks", queryStr,
		"RETURN", "3", "content", "course_id", "score",
		"PARAMS", "2", "V", buf.Bytes(),
		"DIALECT", "2",
	).Slice()
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for i := 1; i < len(docs); i += 2 {
		fields, ok := docs[i+1].([]interface{})
		if !ok {
			continue
		}
		var content string
		var score float32
		for j := 0; j < len(fields); j += 2 {
			key, _ := fields[j].(string)
			switch key {
			case "content":
				content, _ = fields[j+1].(string)
			case "score":
				if v, ok := fields[j+1].(float64); ok {
					score = float32(v)
				}
			}
		}
		if content != "" {
			results = append(results, SearchResult{Content: content, Score: score})
		}
	}
	return results, nil
}

// searchInMemory MySQL 加载向量 + Go 内存余弦相似度（Redis 宕机降级）
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
		if len(c.Embedding) == 0 {
			continue
		}
		s := cosineSimilarity(vec, bytesToFloats(c.Embedding))
		if s > 0.5 {
			candidates = append(candidates, scored{c, s})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

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

// Index 建索引：批量 embed → 双写 MySQL + Redis
func (vs *RedisStackVectorStore) Index(chunkID uint, courseID uint, content string) error {
	vecs, err := embedTexts([]string{content})
	if err != nil {
		return fmt.Errorf("embedding 失败: %w", err)
	}
	vec := vecs[0]

	// 1. MySQL 备份向量
	if err := database.DB.Model(&model.DocumentChunk{}).
		Where("id = ?", chunkID).
		Update("embedding", floatsToBytes(vec)).Error; err != nil {
		return err
	}

	// 2. Redis 建索引（失败不阻塞主流程）
	if database.RDB != nil {
		buf := new(bytes.Buffer)
		for _, v := range vec {
			binary.Write(buf, binary.LittleEndian, v)
		}
		database.RDB.HSet(context.Background(),
			fmt.Sprintf("doc:%d", chunkID),
			"content", content,
			"course_id", courseID,
			"embedding", buf.Bytes(),
		)
	}
	return nil
}

// Delete 删除某课程的全部向量
func (vs *RedisStackVectorStore) Delete(courseID uint) error {
	if database.RDB != nil {
		database.RDB.Do(context.Background(), "FT.DROPINDEX", "idx:chunks", "DD")
	}
	return nil
}

// ============ 初始化 ============

func InitRAG() {
	vs := NewRedisStackVectorStore()
	globalRAG = NewRAGService(vs)

	// Redis Stack 索引初始化
	if database.RDB != nil {
		database.RDB.Do(context.Background(),
			"FT.CREATE", "idx:chunks", "IF", "NOT", "EXISTS",
			"ON", "HASH", "PREFIX", "1", "doc:",
			"SCHEMA",
			"content", "AS", "content", "TEXT",
			"course_id", "AS", "course_id", "NUMERIC", "SORTABLE",
			"embedding", "AS", "embedding", "VECTOR", "HNSW", "6",
			"TYPE", "FLOAT32", "DIM", "1024", "DISTANCE_METRIC", "COSINE",
		)
	}
}

// GetRAG 获取全局 RAG 服务实例
func GetRAG() *RAGService {
	return globalRAG
}

// ============ 辅助函数 ============

// floatsToBytes []float32 → []byte（小端序，每 float32 4 字节）
func floatsToBytes(v []float32) []byte {
	buf := new(bytes.Buffer)
	for _, f := range v {
		binary.Write(buf, binary.LittleEndian, f)
	}
	return buf.Bytes()
}

// bytesToFloats []byte → []float32（小端序，每 4 字节一个 float32）
func bytesToFloats(b []byte) []float32 {
	buf := bytes.NewReader(b)
	v := make([]float32, len(b)/4)
	binary.Read(buf, binary.LittleEndian, &v)
	return v
}
