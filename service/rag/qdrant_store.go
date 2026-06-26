package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// QdrantVectorStore 基于 Qdrant 的向量存储。
// 使用 HTTP REST API，支持混合检索（向量 + payload text 索引过滤）。
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

// Search 混合检索：向量 + payload text 过滤 + course_id 过滤。
func (q *QdrantVectorStore) Search(courseID uint, query string, topK int) ([]SearchResult, error) {
	vecs, err := embedTexts([]string{query})
	if err != nil {
		return nil, fmt.Errorf("embedding 失败: %w", err)
	}
	vec := vecs[0]

	filterMust := []map[string]interface{}{
		{
			"key":   "course_id",
			"match": map[string]interface{}{"value": courseID},
		},
	}

	// 混合检索：Qdrant payload text 索引（multilingual tokenizer 支持中英文分词）
	if config.App.RAG.HybridSearch {
		filterMust = append(filterMust, map[string]interface{}{
			"key":   "content",
			"match": map[string]interface{}{"text": query},
		})
	}

	reqBody := map[string]interface{}{
		"vector":       vec,
		"limit":        topK,
		"with_payload": true,
		"filter": map[string]interface{}{
			"must": filterMust,
		},
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
		sr := SearchResult{ChunkID: r.ID, Score: r.Score}
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

// Index 写入向量 + payload 到 Qdrant。
func (q *QdrantVectorStore) Index(chunkID uint, courseID uint, content string) error {
	vecs, err := embedTexts([]string{content})
	if err != nil {
		return fmt.Errorf("embedding 失败: %w", err)
	}
	vec := vecs[0]

	// 从 MySQL 查 DocumentChunk 拿元数据
	var chunk model.DocumentChunk
	if err := database.DB.Where("id = ?", chunkID).First(&chunk).Error; err != nil {
		// chunk 还没存 MySQL？先尽力拿，拿不到就不带元数据
	}

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

	req, _ := http.NewRequest("PUT",
		q.baseURL+"/collections/"+q.collection+"/points?wait=true",
		bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
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

// checkOrCreateCollection 确保 collection 存在，不存在则创建并建 payload text 索引。
func (q *QdrantVectorStore) checkOrCreateCollection(vectorSize int) error {
	resp, err := q.client.Get(q.baseURL + "/collections/" + q.collection)
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		return nil
	}
	if resp != nil {
		resp.Body.Close()
	}

	// 创建 collection
	createBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	}
	jsonBody, _ := json.Marshal(createBody)
	creq, _ := http.NewRequest("PUT",
		q.baseURL+"/collections/"+q.collection,
		bytes.NewBuffer(jsonBody))
	creq.Header.Set("Content-Type", "application/json")
	resp, err = q.client.Do(creq)
	if err != nil {
		return fmt.Errorf("创建 Qdrant collection 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("创建 collection 失败 %d: %s", resp.StatusCode, string(body))
	}

	// 建 payload text 索引（multilingual tokenizer 原生支持中英文分词）
	indexBody := map[string]interface{}{
		"field_name": "content",
		"field_type": "text",
		"tokenizer":  "multilingual",
	}
	ib, _ := json.Marshal(indexBody)
	ireq, _ := http.NewRequest("PUT",
		q.baseURL+"/collections/"+q.collection+"/index",
		bytes.NewBuffer(ib))
	ireq.Header.Set("Content-Type", "application/json")
	iresp, ierr := q.client.Do(ireq)
	if ierr == nil {
		iresp.Body.Close()
	}
	return nil
}

// ensureContext 确保 context 可用。
func ensureContext() context.Context {
	return context.Background()
}
