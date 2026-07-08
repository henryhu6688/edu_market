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
	"strings"
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
		database.RDB.Del(context.Background(),
			fmt.Sprintf("rag:sem:%d:full", courseID),
			fmt.Sprintf("rag:sem:%d:preview", courseID),
		)
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
// 日志链路：L1命中 → L2命中 → 完整管线(Qdrant→Rerank→写缓存)，每步带耗时和条数。
func (r *RAGService) Search(courseID uint, query string, topK int, hasAccess bool) ([]SearchResult, error) {
	queryPreview := truncateStr(query, 60)
	pipeStart := time.Now()

	accessLevel := "preview"
	if hasAccess {
		accessLevel = "full"
	}

	// ====== L1 精确匹配 ======
	if config.App.RAG.CacheEnabled && database.RDB != nil {
		normalized := normalizeCacheKey(query)
		exactKey := fmt.Sprintf("rag:exact:%x:%s", md5.Sum([]byte(fmt.Sprintf("%s_%d", normalized, courseID))), accessLevel)
		if b, err := database.RDB.Get(context.Background(), exactKey).Bytes(); err == nil && len(b) > 0 {
			var results []SearchResult
			if json.Unmarshal(b, &results) == nil {
				slog.Info("rag L1精确缓存命中", "course_id", courseID, "results", len(results), "query", queryPreview)
				return results, nil
			}
		}
	}

	// ====== L2 语义匹配 ======
	var queryVec []float32
	if config.App.RAG.CacheEnabled && database.RDB != nil {
		embStart := time.Now()
		vecs, _ := embedTexts([]string{query})
		if len(vecs) > 0 {
			queryVec = vecs[0]
			slog.Info("rag L2查询向量化完成，准备余弦相似度比对", "course_id", courseID, "emb_ms", time.Since(embStart).Milliseconds())
			semKey := fmt.Sprintf("rag:sem:%d:%s", courseID, accessLevel)
			if b, err := database.RDB.Get(context.Background(), semKey).Bytes(); err == nil {
				var recent []cachedEntry
				if json.Unmarshal(b, &recent) == nil {
					for _, entry := range recent {
						sim := float64(cosineSimilarity(queryVec, entry.Vector))
						if sim >= config.App.RAG.CacheSimThreshold {
							slog.Info("rag L2语义缓存命中，余弦相似度≥阈值，复用历史结果", "course_id", courseID, "similarity", fmt.Sprintf("%.3f", sim), "results", len(entry.Results), "query", queryPreview)
							return entry.Results, nil
						}
					}
					slog.Info("rag L2语义缓存未命中，已比对所有历史向量均低于阈值", "course_id", courseID, "checked", len(recent), "threshold", config.App.RAG.CacheSimThreshold)
				}
			}
		}
	}

	// ====== 完整管线 ======
	slog.Info("rag L1+L2缓存均未命中，进入Qdrant向量检索+Rerank管线", "course_id", courseID, "query", queryPreview)
	if r.vectorStore == nil {
		return nil, errors.New("向量存储未初始化")
	}

	// Rerank 时扩大召回量，粗筛 Net  → Rerank 精选
	candidateTopK := topK
	if config.App.RAG.Rerank {
		candidateTopK = topK * 4
		if candidateTopK > 50 {
			candidateTopK = 50
		}
	}

	qdrantStart := time.Now()
	results, err := r.vectorStore.Search(courseID, query, candidateTopK)
	if err != nil {
		return nil, err
	}
	slog.Info("rag Qdrant向量检索完成", "course_id", courseID, "results", len(results), "candidate_topk", candidateTopK, "hybrid", config.App.RAG.HybridSearch, "qdrant_ms", time.Since(qdrantStart).Milliseconds())

	// BM25 双路召回 + RRF 融合 [开关: bm25_enabled]
	if config.App.RAG.BM25Enabled {
		bm25Results, bm25Err := bm25Search(courseID, query, candidateTopK)
		if bm25Err != nil {
			slog.Warn("rag BM25检索失败，降级为纯向量", "err", bm25Err)
		} else if len(bm25Results) > 0 {
			results = rrfFuse(results, bm25Results, candidateTopK)
			slog.Info("rag BM25+向量RRF融合完成", "course_id", courseID, "fused", len(results))
		}
	}

	// Rerank [开关: rerank]
	if config.App.RAG.Rerank && len(results) > 1 {
		rerankStart := time.Now()
		reranker := NewReranker()
		ranked, err := reranker.Rerank(query, results, config.App.RAG.RerankTopK)
		if err != nil {
			slog.Warn("rag Rerank精排失败，降级使用原始排序", "err", err, "course_id", courseID)
			if len(results) > config.App.RAG.RerankTopK {
				results = results[:config.App.RAG.RerankTopK]
			}
		} else {
			slog.Info("rag Rerank精排完成", "course_id", courseID, "before", len(results), "after", len(ranked), "rerank_ms", time.Since(rerankStart).Milliseconds())
			results = ranked
		}
	} else if len(results) > topK {
		results = results[:topK]
	}

	// ====== 写入缓存 ======
	if config.App.RAG.CacheEnabled && database.RDB != nil  {
		// L1
		normalized := normalizeCacheKey(query)
		exactKey := fmt.Sprintf("rag:exact:%x:%s", md5.Sum([]byte(fmt.Sprintf("%s_%d", normalized, courseID))), accessLevel)
		b, _ := json.Marshal(results)
		database.RDB.SetEx(context.Background(), exactKey, b, time.Duration(config.App.RAG.CacheTTL)*time.Second)

		// L2: 如果还没拿到 query vector，再调一次
		if len(queryVec) == 0 {
			embStart := time.Now()
			vecs, _ := embedTexts([]string{query})
			if len(vecs) > 0 {
				queryVec = vecs[0]
				slog.Debug("rag L2语义缓存-写回时查询向量化完成", "course_id", courseID, "emb_ms", time.Since(embStart).Milliseconds())
			}
		}
		if len(queryVec) > 0 {
			semKey := fmt.Sprintf("rag:sem:%d:%s", courseID, accessLevel)
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
		slog.Info("rag L1+L2缓存写入完成", "course_id", courseID, "results", len(results), "query", queryPreview)
	}

	slog.Info("rag 检索管线结束", "course_id", courseID, "results", len(results), "total_ms", time.Since(pipeStart).Milliseconds())
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

// normalizeCacheKey 归一化 L1 缓存 key 中的 query 部分。
// 去掉中文停用词和标点，收起空白，使「闭包的原理」和「闭包 原理」命中同一缓存。
//
// 不涉及分词——只做字符级过滤。停用词列表覆盖最常见的虚词和标点。
// 归一化后的 key 参与 md5，对 LLM 透明。
func normalizeCacheKey(query string) string {
	// 1. 去中文标点
	punctuations := "，。！？；：\"\"''（）《》【】、—…"
	for _, r := range punctuations {
		query = strings.ReplaceAll(query, string(r), " ")
	}
	// 2. 去中文停用词（两侧加空格方便精确替换）
	stopWords := []string{"的", "了", "吗", "呢", "啊", "吧", "和", "与", "及", "或", "在", "是", "有", "被", "把", "对", "从", "到", "让", "给", "为", "以", "也", "就", "都", "而", "但", "却", "所", "者", "之", "仍", "便", "则", "虽", "即"}
	for _, sw := range stopWords {
		query = strings.ReplaceAll(query, sw, " ")
	}
	// 3. 收起空白
	fields := strings.Fields(query)
	return strings.Join(fields, " ")
}

// truncateStr 截取字符串前 n 个字符（Unicode 安全），用于日志。
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
