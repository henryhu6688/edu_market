// Package rag 提供基于向量检索的 RAG（Retrieval-Augmented Generation）服务。
//
// 支持多种向量存储后端（Redis Stack / MySQL 关键词搜索），
// 通过 Embedding API 将文档切片向量化，实现语义搜索。
//
// 文件职责：
//   - rag.go         核心服务 + 向量存储接口 + 文本切片 + 初始化
//   - embedding.go    Embedding API 调用 + 向量工具函数
//   - redis_store.go  Redis Stack KNN 向量存储
//   - simple_store.go MySQL LIKE 关键词搜索（降级）
package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// SearchResult 检索结果
type SearchResult struct {
	ChunkID     uint    `json:"chunk_id"`
	Content     string  `json:"content"`
	Score       float32 `json:"score"`
	DocumentID  uint    `json:"document_id"`  // 来源文档 ID
	SectionPath string  `json:"section_path"` // 章节路径
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

// ============ 全局 RAG ============

var globalRAG *RAGService

// Init 初始化全局 RAG 服务（Redis Stack 向量存储）
func Init() {
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

// Get 获取全局 RAG 服务实例
func Get() *RAGService {
	return globalRAG
}
