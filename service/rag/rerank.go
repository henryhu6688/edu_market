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

	req, _ := http.NewRequest("POST", "https://api.siliconflow.cn/v1/rerank", bytes.NewBuffer(jsonBody))
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

	// 构建 index → score 映射
	scoreMap := make(map[int]float64)
	for _, r := range result.Results {
		scoreMap[r.Index] = r.Score
	}

	// 按 relevance_score 降序排列
	sort.SliceStable(chunks, func(i, j int) bool {
		return scoreMap[i] > scoreMap[j]
	})

	if len(chunks) > topK {
		chunks = chunks[:topK]
	}
	return chunks, nil
}
