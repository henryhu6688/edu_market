package service

import (
	"errors"
	"fmt"
	"strings"

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
func InitRAG() {
	vs := NewSimpleSearchVectorStore()
	globalRAG = NewRAGService(vs)
}

// GetRAG 获取全局 RAG 服务实例
func GetRAG() *RAGService {
	return globalRAG
}
