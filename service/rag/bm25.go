package rag

import (
	"fmt"
	"log/slog"
	"time"

	"edu_market/database"
)

// rrfScored RRF 融合得分条目
type rrfScored struct {
	result *SearchResult
	score  float64
}

// bm25Search MySQL FULLTEXT 关键词搜索，返回带自然语言模式得分的 chunk。
func bm25Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	start := time.Now()

	type row struct {
		ID        uint    `gorm:"column:id"`
		Content   string  `gorm:"column:content"`
		CourseID  uint    `gorm:"column:course_id"`
		ChunkID   uint    `gorm:"column:id"`
		Score     float32 `gorm:"column:bm25_score"`
		DocumentID uint  `gorm:"column:document_id"`
		SectionPath string `gorm:"column:section_path"`
	}

	var rows []row
	err := database.DB.Raw(`
		SELECT dc.id, dc.content, dc.course_id, dc.document_id, dc.section_path,
		       MATCH(dc.content) AGAINST(? IN NATURAL LANGUAGE MODE) AS bm25_score
		FROM document_chunks dc
		WHERE dc.course_id = ?
		  AND MATCH(dc.content) AGAINST(? IN NATURAL LANGUAGE MODE) > 0
		ORDER BY bm25_score DESC
		LIMIT ?
	`, query, courseID, query, topK).Scan(&rows).Error

	if err != nil {
		return nil, fmt.Errorf("BM25 搜索失败: %w", err)
	}

	results := make([]SearchResult, len(rows))
	for i, r := range rows {
		results[i] = SearchResult{
			ChunkID:     r.ChunkID,
			Content:     r.Content,
			Score:       r.Score,
			DocumentID:  r.DocumentID,
			SectionPath: r.SectionPath,
		}
	}

	slog.Info("rag BM25关键词检索完成", "course_id", courseID, "results", len(results), "bm25_ms", time.Since(start).Milliseconds())
	return results, nil
}

// rrfFuse 双路 RRF（Reciprocal Rank Fusion）融合。
// 同一文档在两路中的排名越靠前，融合得分越高。
func rrfFuse(vectorResults, bm25Results []SearchResult, topK int) []SearchResult {
	const k = 60 // RRF 常数

	// 建立 chunkID → 排名 映射
	vecRank := make(map[uint]int)
	for i, r := range vectorResults {
		if _, exists := vecRank[r.ChunkID]; !exists {
			vecRank[r.ChunkID] = i + 1 // 1-indexed
		}
	}
	bm25Rank := make(map[uint]int)
	for i, r := range bm25Results {
		if _, exists := bm25Rank[r.ChunkID]; !exists {
			bm25Rank[r.ChunkID] = i + 1
		}
	}

	// 合并所有 chunk，计算 RRF 得分
	allChunks := make(map[uint]*SearchResult)
	for _, r := range vectorResults {
		allChunks[r.ChunkID] = &SearchResult{
			ChunkID:     r.ChunkID,
			Content:     r.Content,
			DocumentID:  r.DocumentID,
			SectionPath: r.SectionPath,
		}
	}
	for _, r := range bm25Results {
		if _, exists := allChunks[r.ChunkID]; !exists {
			allChunks[r.ChunkID] = &SearchResult{
				ChunkID:     r.ChunkID,
				Content:     r.Content,
				DocumentID:  r.DocumentID,
				SectionPath: r.SectionPath,
			}
		}
	}

	// 计算 RRF 得分
	var scoredList []rrfScored
	for _, r := range allChunks {
		rrf := 0.0
		if rank, ok := vecRank[r.ChunkID]; ok {
			rrf += 1.0 / (float64(k) + float64(rank))
		}
		if rank, ok := bm25Rank[r.ChunkID]; ok {
			rrf += 1.0 / (float64(k) + float64(rank))
		}
		r.Score = float32(rrf)
		scoredList = append(scoredList, rrfScored{r, rrf})
	}

	// 按 RRF 得分降序排列
	sortScored(scoredList)

	// 截断
	if len(scoredList) > topK {
		scoredList = scoredList[:topK]
	}

	results := make([]SearchResult, len(scoredList))
	for i, s := range scoredList {
		results[i] = *s.result
	}

	slog.Info("rag RRF双路融合完成", "vector_count", len(vectorResults), "bm25_count", len(bm25Results), "merged", len(results))
	return results
}

// sortScored 按 score 降序排列
func sortScored(list []rrfScored) {
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].score > list[i].score {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
}
