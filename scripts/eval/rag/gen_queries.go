package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"edu_market/config"
	"edu_market/database"
	"edu_market/model"
)

// generateQueries 分层抽样 + LLM 反向生成测试问题。
func generateQueries(count int) ([]RAGQuery, error) {
	chunks, err := sampleChunks(count)
	if err != nil {
		return nil, fmt.Errorf("抽样切片失败: %w", err)
	}
	slog.Info("rag-eval 切片抽样完成", "count", len(chunks))

	var queries []RAGQuery
	batchSize := 10
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]
		batchQueries, err := tryGenerateBatch(batch)
		if err != nil {
			slog.Warn("rag-eval LLM 批次生成重试仍失败，跳过", "batch_start", i, "err", err)
			continue
		}
		for _, q := range batchQueries {
			q.ID = fmt.Sprintf("Q%03d", len(queries)+1)
			queries = append(queries, q)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return queries, nil
}

// tryGenerateBatch 调 LLM 生成一批查询，失败重试一次。
func tryGenerateBatch(chunks []model.DocumentChunk) ([]RAGQuery, error) {
	batchQueries, err := generateQueryBatch(chunks)
	if err != nil {
		slog.Warn("rag-eval LLM 批次生成失败，重试中", "err", err)
		time.Sleep(1 * time.Second)
		batchQueries, err = generateQueryBatch(chunks)
	}
	return batchQueries, err
}

// sampleChunks 按 material 分层抽样 n 条切片。
func sampleChunks(n int) ([]model.DocumentChunk, error) {
	type matCount struct {
		MaterialID uint
		Count      int64
	}
	var counts []matCount
	database.DB.Model(&model.DocumentChunk{}).
		Select("course_id as material_id, count(*) as count").
		Group("course_id").Order("count desc").Find(&counts)

	if len(counts) == 0 {
		return nil, fmt.Errorf("document_chunks 表为空，请先入库资料")
	}

	perMat := make(map[uint]int)
	total := 0
	for _, c := range counts {
		perMat[c.MaterialID] = 1
		total++
	}
	remaining := n - total
	if remaining > 0 {
		var totalChunks int64
		for _, c := range counts {
			totalChunks += c.Count
		}
		for _, c := range counts {
			extra := int(float64(remaining) * float64(c.Count) / float64(totalChunks))
			perMat[c.MaterialID] += extra
		}
	}

	var chunks []model.DocumentChunk
	for _, c := range counts {
		want := perMat[c.MaterialID]
		if want <= 0 {
			continue
		}
		var matChunks []model.DocumentChunk
		database.DB.Where("course_id = ?", c.MaterialID).
			Where("LENGTH(content) > 30").Find(&matChunks)
		if len(matChunks) == 0 {
			continue
		}
		rand.Shuffle(len(matChunks), func(i, j int) {
			matChunks[i], matChunks[j] = matChunks[j], matChunks[i]
		})
		if want > len(matChunks) {
			want = len(matChunks)
		}
		chunks = append(chunks, matChunks[:want]...)
	}
	return chunks, nil
}

// generateQueryBatch 对一批切片调 LLM 批量生成查询。
func generateQueryBatch(chunks []model.DocumentChunk) ([]RAGQuery, error) {
	cfg := config.App.AI
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("AI API Key 未配置")
	}

	var parts []string
	for i, c := range chunks {
		parts = append(parts, fmt.Sprintf(
			"[切片%d]\n内容：%s\n请为以上内容生成一个用户在学习这个知识点时可能会搜索的一般性问题（5-15字），问题要覆盖这个知识点而不只是其中的细节，只输出问题本身。",
			i, c.Content,
		))
	}

	model := cfg.Model
	if model == "" {
		model = "deepseek-chat"
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "你是一个搜索查询生成器。根据给定的资料内容，生成真实用户在学习时可能会搜索的查询。对每个切片，输出一行对应的查询问题，不要序号。",
			},
			{"role": "user", "content": strings.Join(parts, "\n\n")},
		},
		"stream":      false,
		"max_tokens":  1024,
		"temperature": 0.7,
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

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 LLM 响应失败: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("LLM 返回空 choices")
	}

	lines := strings.Split(strings.TrimSpace(result.Choices[0].Message.Content), "\n")
	var queries []RAGQuery
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "0123456789.、) ）")
		line = strings.TrimSpace(line)
		if line == "" || i >= len(chunks) {
			continue
		}
		c := chunks[i]
		category := ""
		if c.SectionPath != "" {
			category = strings.Split(c.SectionPath, "/")[0]
		}
		queries = append(queries, RAGQuery{
			Query:            line,
			MaterialID:       c.CourseID,
			RelevantChunkIDs: []uint{c.ID},
			Category:         category,
		})
	}
	return queries, nil
}

// truncateRunes 按 rune 截断字符串。
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
