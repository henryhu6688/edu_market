package rag

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sort"

	"edu_market/database"
	"edu_market/model"
)

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
		binary.Write(buf, binary.LittleEndian, v)
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
