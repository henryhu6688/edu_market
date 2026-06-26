package rag

import (
	"strings"

	"edu_market/database"
	"edu_market/model"
)

// SimpleSearchVectorStore 纯 MySQL 关键词搜索实现（不依赖 Embedding API）
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
			ChunkID:     c.ID,
			Content:     c.Content,
			Score:       0.5,
			DocumentID:  c.DocumentID,
			SectionPath: c.SectionPath,
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
