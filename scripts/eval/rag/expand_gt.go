package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"edu_market/config"
	"edu_market/service/rag"
)

// expandGroundTruth 对每条查询暴力搜 topK=50，让 LLM 判断哪些结果相关。
func expandGroundTruth(queries []RAGQuery, ragSvc *rag.RAGService) ([]RAGQuery, error) {
	// 临时关缓存 + Rerank + Hybrid，确保纯向量暴力搜
	origCache := config.App.RAG.CacheEnabled
	origRerank := config.App.RAG.Rerank
	origHybrid := config.App.RAG.HybridSearch
	config.App.RAG.CacheEnabled = false
	config.App.RAG.Rerank = false
	config.App.RAG.HybridSearch = false
	defer func() {
		config.App.RAG.CacheEnabled = origCache
		config.App.RAG.Rerank = origRerank
		config.App.RAG.HybridSearch = origHybrid
	}()

	for i := range queries {
		q := &queries[i]
		slog.Info("rag-eval 扩展 ground truth", "query_id", q.ID, "query", q.Query)

		results, err := ragSvc.Search(q.MaterialID, q.Query, 20, true)
		if err != nil {
			slog.Warn("rag-eval 暴力搜索失败，保留原始 ground truth", "query_id", q.ID, "err", err)
			q.GTIncomplete = true
			continue
		}

		if len(results) == 0 {
			q.GTIncomplete = true
			continue
		}

		// LLM 判断相关性
		relevantIDs, err := judgeRelevance(q.Query, results)
		if err != nil {
			slog.Warn("rag-eval LLM 相关性判断失败，保留原始 ground truth", "query_id", q.ID, "err", err)
			q.GTIncomplete = true
			continue
		}

		if len(relevantIDs) == 0 {
			relevantIDs = q.RelevantChunkIDs // 退回到原始
		} else {
			// 合并（去重）
			set := make(map[uint]bool)
			for _, id := range q.RelevantChunkIDs {
				set[id] = true
			}
			for _, id := range relevantIDs {
				set[id] = true
			}
			q.RelevantChunkIDs = nil
			for id := range set {
				q.RelevantChunkIDs = append(q.RelevantChunkIDs, id)
			}
		}
		slog.Info("rag-eval ground truth 扩展完成", "query_id", q.ID, "relevant", len(q.RelevantChunkIDs))
		time.Sleep(200 * time.Millisecond)
	}
	return queries, nil
}

// judgeRelevance 调用 LLM 判断哪些搜索结果与查询相关。
func judgeRelevance(query string, results []rag.SearchResult) ([]uint, error) {
	cfg := config.App.AI
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("AI API Key 未配置")
	}

	// 构建候选列表（每条截断到 200 字）
	var candidates strings.Builder
	for _, r := range results {
		candidates.WriteString(fmt.Sprintf(
			"[ID:%d] %s\n", r.ChunkID, r.Content,
		))
	}

	prompt := fmt.Sprintf(
		`查询：「%s」

下面是从资料中检索到的片段（每条带 ID）。
一条片段是"相关"的定义：它包含能回答这个查询的信息。
请判断哪些片段是相关的，返回相关片段的 ID 列表。
只返回 JSON 数组，如 [42, 88, 105]。

%s`, query, candidates.String(),
	)

	model := cfg.Model
	if model == "" {
		model = "deepseek-chat"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个检索质量评估专家。只返回 JSON 数组。"},
			{"role": "user", "content": prompt},
		},
		"stream":      false,
		"max_tokens":  512,
		"temperature": 0,
	}

	jsonBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", cfg.APIURL, bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM 返回 %d: %s", resp.StatusCode, truncateRunes(string(body), 200))
	}

	var llmResult struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &llmResult); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %w", err)
	}
	if len(llmResult.Choices) == 0 {
		return nil, fmt.Errorf("LLM 返回空 choices")
	}

	content := strings.TrimSpace(llmResult.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var ids []uint
	if err := json.Unmarshal([]byte(content), &ids); err != nil {
		return nil, fmt.Errorf("解析 LLM 返回的 ID 列表失败: %w (content: %s)", err, truncateRunes(content, 100))
	}
	return ids, nil
}
