// Package rag 提供基于向量检索的 RAG（Retrieval-Augmented Generation）服务。
//
// 支持多种向量存储后端（Qdrant / MySQL 关键词搜索），
// 通过 Embedding API 将文档切片向量化，实现语义搜索。
//
// 文件职责：
//   - rag.go          核心服务 + 向量存储接口 + 文本切片 + 初始化
//   - embedding.go     Embedding API 调用 + 向量工具函数
//   - qdrant_store.go  Qdrant 向量存储（HTTP REST API）
//   - simple_store.go  MySQL LIKE 关键词搜索（降级）
package rag

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
		chunks = r.fixedSizeChunks(fullText)
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

// fixedSizeChunks 固定大小切片（structural_chunk 关闭时回退）。
func (r *RAGService) fixedSizeChunks(text string) []Chunk {
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

// cachedEntry 语义缓存条目
type cachedEntry struct {
	Query   string         `json:"query"`
	Vector  []float32      `json:"vector"`
	Results []SearchResult `json:"results"`
}

// Search 检索课程资料（带两级缓存 + Rerank）。
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

	// ====== L2 语义匹配 ======
	var queryVec []float32
	if config.App.RAG.CacheEnabled && database.RDB != nil {
		vecs, _ := embedTexts([]string{query})
		if len(vecs) > 0 {
			queryVec = vecs[0]
			semKey := fmt.Sprintf("rag:sem:%d", courseID)
			if b, err := database.RDB.Get(context.Background(), semKey).Bytes(); err == nil {
				var recent []cachedEntry
				if json.Unmarshal(b, &recent) == nil {
					for _, entry := range recent {
						if float64(cosineSimilarity(queryVec, entry.Vector)) >= config.App.RAG.CacheSimThreshold {
							return entry.Results, nil
						}
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

	// Rerank [开关: rerank]
	if config.App.RAG.Rerank && len(results) > 1 {
		reranker := NewReranker()
		ranked, err := reranker.Rerank(query, results, config.App.RAG.RerankTopK)
		if err != nil {
			slog.Warn("Rerank 失败，降级到原始结果", "err", err)
			if len(results) > config.App.RAG.RerankTopK {
				results = results[:config.App.RAG.RerankTopK]
			}
		} else {
			results = ranked
		}
	} else if len(results) > topK {
		results = results[:topK]
	}

	// ====== 写入缓存 ======
	if config.App.RAG.CacheEnabled && database.RDB != nil && len(results) > 0 {
		// L1
		exactKey := fmt.Sprintf("rag:exact:%x", md5.Sum([]byte(fmt.Sprintf("%s_%d", query, courseID))))
		b, _ := json.Marshal(results)
		database.RDB.SetEx(context.Background(), exactKey, b, time.Duration(config.App.RAG.CacheTTL)*time.Second)

		// L2: 如果还没拿到 query vector，再调一次
		if len(queryVec) == 0 {
			vecs, _ := embedTexts([]string{query})
			if len(vecs) > 0 {
				queryVec = vecs[0]
			}
		}
		if len(queryVec) > 0 {
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
			b, _ := json.Marshal(recent)
			database.RDB.SetEx(context.Background(), semKey, b, 0)
		}
	}

	return results, nil
}

// ============ 全局 RAG ============

var globalRAG *RAGService

// Init 初始化全局 RAG 服务（Qdrant 向量存储）
func Init() {
	vs := NewQdrantVectorStore()
	vs.checkOrCreateCollection(1024) // bge-m3 = 1024 维
	globalRAG = NewRAGService(vs)
}

// Get 获取全局 RAG 服务实例
func Get() *RAGService {
	return globalRAG
}
