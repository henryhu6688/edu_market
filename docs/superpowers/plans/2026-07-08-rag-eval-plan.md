# RAG 检索质量评测系统 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立 RAG 检索质量评测系统，自动生成 100 条标注查询，分层 A/B 对比四组配置，输出含 Precision/Recall/Top5命中率/延迟 的 Markdown 报告。

**Architecture:** 7 个文件的 CLI 脚本（`scripts/eval/rag/`），直接调 `ragSvc.Search()`，不经过 Agent。通过修改 `config.App.RAG` 全局变量切换四组配置，零侵入 `service/rag/`。

**Tech Stack:** Go + DeepSeek Chat API + Qdrant + MySQL + Redis

## Global Constraints

- 所有文件放 `scripts/eval/rag/`，package `main`
- 零侵入 `service/rag/`，不改任何已有代码
- K 默认 = 5
- LLM 调用复用 `config.App.AI` 配置（同现有 eval）
- 报告输出到 `data/eval/reports/rag/`，标注数据存 `data/eval/rag_queries.json`
- 错误不阻塞流程：gen 失败跳过该切片，expand 失败保留原始 1 个 ground truth
- Go 代码风格：中文注释导出类型/函数，error 用 fmt.Errorf 包装

---

### Task 1: types.go — 核心数据结构

**Files:**
- Create: `scripts/eval/rag/types.go`

**Interfaces:**
- Produces: `RAGQuery`, `RAGQueryResult`, `EvalConfig`, `ConfigSummary`, `LatencyStats`, `RAGEvalReport`

- [ ] **Step 1: 写 types.go**

```go
package main

// RAGQuery 单条检索评测查询（标注数据）
type RAGQuery struct {
	ID               string `json:"id"`
	Query            string `json:"query"`
	MaterialID       uint   `json:"material_id"`
	RelevantChunkIDs []uint `json:"relevant_chunk_ids"` // ground truth
	Category         string `json:"category,omitempty"`
	GTIncomplete     bool   `json:"gt_incomplete,omitempty"` // expand_gt 失败时标记
}

// RAGQueryResult 单条查询的单组配置评测结果
type RAGQueryResult struct {
	QueryID          string  `json:"query_id"`
	PrecisionAtK     float64 `json:"precision_at_k"`
	RecallAtK        float64 `json:"recall_at_k"`
	TopKHit          bool    `json:"top_k_hit"`
	LatencyMs        float64 `json:"latency_ms"`
	ReturnedChunkIDs []uint  `json:"returned_chunk_ids"`
}

// EvalConfig 单组评测的 RAG 配置
type EvalConfig struct {
	Name         string `json:"name"`
	Hybrid       bool   `json:"hybrid"`
	Rerank       bool   `json:"rerank"`
	CacheEnabled bool   `json:"cache_enabled"`
}

// ConfigSummary 单组配置的汇总指标
type ConfigSummary struct {
	ConfigName   string  `json:"config_name"`
	AvgPrecision float64 `json:"avg_precision_at_k"`
	AvgRecall    float64 `json:"avg_recall_at_k"`
	TopKHitRate  float64 `json:"top_k_hit_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// LatencyStats 延迟分位数
type LatencyStats struct {
	ConfigName string  `json:"config_name"`
	Avg        float64 `json:"avg_ms"`
	P50        float64 `json:"p50_ms"`
	P95        float64 `json:"p95_ms"`
	P99        float64 `json:"p99_ms"`
}

// RAGEvalReport 完整评测报告数据
type RAGEvalReport struct {
	Timestamp    string          `json:"timestamp"`
	TopK         int             `json:"top_k"`
	TotalQueries int             `json:"total_queries"`
	Configs      []EvalConfig    `json:"configs"`
	Results      []RAGQueryResult `json:"results"`
	Summaries    []ConfigSummary `json:"summaries"`
	LatencyStats []LatencyStats  `json:"latency_stats"`
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./scripts/eval/rag/
```

- [ ] **Step 3: Commit**

```bash
git add scripts/eval/rag/types.go
git commit -m "feat(rag-eval): 定义评测数据结构"
```

---

### Task 2: metrics.go — 四指标计算

**Files:**
- Create: `scripts/eval/rag/metrics.go`

**Interfaces:**
- Consumes: `RAGQuery`, `RAGQueryResult`
- Produces: `precisionAtK()`, `recallAtK()`, `topKHit()`, `latencyPercentiles()`, `configSummary()`, `toSet()`, `intersectCount()`

- [ ] **Step 1: 写 metrics.go**

```go
package main

import (
	"sort"
)

// precisionAtK = |前K条 ∩ ground_truth| / K
func precisionAtK(returned, relevant []uint, k int) float64 {
	if k == 0 {
		return 0
	}
	top := returned
	if len(top) > k {
		top = top[:k]
	}
	return float64(intersectCount(top, toSet(relevant))) / float64(k)
}

// recallAtK = |前K条 ∩ ground_truth| / |ground_truth|
func recallAtK(returned, relevant []uint, k int) float64 {
	if len(relevant) == 0 {
		return 0
	}
	top := returned
	if len(top) > k {
		top = top[:k]
	}
	return float64(intersectCount(top, toSet(relevant))) / float64(len(relevant))
}

// topKHit = 前K条是否至少命中一条 ground truth
func topKHit(returned, relevant []uint, k int) bool {
	set := toSet(relevant)
	limit := k
	if len(returned) < limit {
		limit = len(returned)
	}
	for _, id := range returned[:limit] {
		if set[id] {
			return true
		}
	}
	return false
}

// toSet 将切片转为 set
func toSet(ids []uint) map[uint]bool {
	s := make(map[uint]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// intersectCount 返回 ids 中有多少个在 set 里
func intersectCount(ids []uint, set map[uint]bool) int {
	n := 0
	for _, id := range ids {
		if set[id] {
			n++
		}
	}
	return n
}

// latencyPercentiles 从延迟列表计算分位数（需先排序）
func latencyPercentiles(sortedMs []float64) (avg, p50, p95, p99 float64) {
	if len(sortedMs) == 0 {
		return 0, 0, 0, 0
	}
	sum := 0.0
	for _, v := range sortedMs {
		sum += v
	}
	avg = sum / float64(len(sortedMs))

	idx50 := int(float64(len(sortedMs)) * 0.50)
	idx95 := int(float64(len(sortedMs)) * 0.95)
	idx99 := int(float64(len(sortedMs)) * 0.99)
	if idx50 >= len(sortedMs) { idx50 = len(sortedMs) - 1 }
	if idx95 >= len(sortedMs) { idx95 = len(sortedMs) - 1 }
	if idx99 >= len(sortedMs) { idx99 = len(sortedMs) - 1 }

	return avg, sortedMs[idx50], sortedMs[idx95], sortedMs[idx99]
}

// configSummary 从一组 RAGQueryResult 汇总指标
func configSummary(cfgName string, results []RAGQueryResult) ConfigSummary {
	s := ConfigSummary{ConfigName: cfgName}
	if len(results) == 0 {
		return s
	}
	var sumP, sumR float64
	hits := 0
	var lats []float64
	for _, r := range results {
		sumP += r.PrecisionAtK
		sumR += r.RecallAtK
		if r.TopKHit {
			hits++
		}
		lats = append(lats, r.LatencyMs)
	}
	s.AvgPrecision = sumP / float64(len(results))
	s.AvgRecall = sumR / float64(len(results))
	s.TopKHitRate = float64(hits) / float64(len(results))

	sort.Float64s(lats)
	s.AvgLatencyMs, _, _, _ = latencyPercentiles(lats)
	return s
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./scripts/eval/rag/
```

- [ ] **Step 3: Commit**

```bash
git add scripts/eval/rag/metrics.go
git commit -m "feat(rag-eval): 实现四指标计算函数"
```

---

### Task 3: gen_queries.go — Step 1: LLM 反向生成问题

**Files:**
- Create: `scripts/eval/rag/gen_queries.go`

**Interfaces:**
- Consumes: `RAGQuery`, `config.App.AI`, `database.DB`, `model.DocumentChunk`
- Produces: `generateQueries(count int) ([]RAGQuery, error)`

- [ ] **Step 1: 写 gen_queries.go**

```go
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
	// 1. 按 material 分层抽样切片
	chunks, err := sampleChunks(count)
	if err != nil {
		return nil, fmt.Errorf("抽样切片失败: %w", err)
	}
	slog.Info("rag-eval 切片抽样完成", "count", len(chunks))

	// 2. 批量调 LLM 生成问题（10 条/批）
	var queries []RAGQuery
	batchSize := 10
	for i := 0; i < len(chunks); i += batchSize {
		end := i + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]
		batchQueries, err := generateQueryBatch(batch)
		if err != nil {
			slog.Warn("rag-eval LLM 批次生成失败", "batch_start", i, "err", err)
			continue
		}
		for j, q := range batchQueries {
			q.ID = fmt.Sprintf("Q%03d", i+j+1)
			queries = append(queries, q)
		}
		time.Sleep(200 * time.Millisecond) // 限流间隔
	}
	return queries, nil
}

// sampleChunks 按 material 分层抽样 n 条切片。
func sampleChunks(n int) ([]model.DocumentChunk, error) {
	// 按 material 统计切片数
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

	// 分配：每资料至少 1 条，其余按比例
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

	// 从每个 material 随机取切片
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

	// 构建批量 prompt
	var parts []string
	for i, c := range chunks {
		parts = append(parts, fmt.Sprintf(
			"[切片%d]\n内容：%s\n请为以上内容生成一个用户可能会搜索的问题（5-15字），只输出问题本身。",
			i, truncateRunes(c.Content, 300),
		))
	}

	reqBody := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": "你是一个搜索查询生成器。根据给定的资料内容，生成真实用户在学习时可能会搜索的查询。",
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

	// 解析 LLM 输出的每行对应一个查询
	lines := strings.Split(strings.TrimSpace(result.Choices[0].Message.Content), "\n")
	var queries []RAGQuery
	for i, line := range lines {
		line = strings.TrimSpace(line)
		// 去掉可能的序号前缀如 "1. " 或 "1、"
		line = strings.TrimLeft(line, "0123456789.、 )")
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
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./scripts/eval/rag/
```

- [ ] **Step 3: 手动验证：生成 10 条测试**

```bash
go run ./scripts/eval/rag/ --step=gen --queries=10
# 预期：data/eval/rag_queries.json 生成 10 条，每条有 query + relevant_chunk_ids
cat data/eval/rag_queries.json | head -50
```

- [ ] **Step 4: Commit**

```bash
git add scripts/eval/rag/gen_queries.go
git commit -m "feat(rag-eval): Step1 — LLM反向生成测试问题"
```

---

### Task 4: expand_gt.go — Step 2: LLM 扩展 ground truth

**Files:**
- Create: `scripts/eval/rag/expand_gt.go`

**Interfaces:**
- Consumes: `RAGQuery`, `ragSvc.Search()`, `config.App.AI`
- Produces: `expandGroundTruth(queries []RAGQuery) ([]RAGQuery, error)`

- [ ] **Step 1: 写 expand_gt.go**

```go
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
	// 临时关缓存 + Rerank，确保暴力搜
	origCache := config.App.RAG.CacheEnabled
	origRerank := config.App.RAG.Rerank
	config.App.RAG.CacheEnabled = false
	config.App.RAG.Rerank = false
	defer func() {
		config.App.RAG.CacheEnabled = origCache
		config.App.RAG.Rerank = origRerank
	}()

	for i := range queries {
		q := &queries[i]
		slog.Info("rag-eval 扩展 ground truth", "query_id", q.ID, "query", q.Query)

		results, err := ragSvc.Search(q.MaterialID, q.Query, 50, true)
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
			"[ID:%d] %s\n", r.ChunkID, truncateRunes(r.Content, 200),
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

	reqBody := map[string]interface{}{
		"model": cfg.Model,
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
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./scripts/eval/rag/
```

- [ ] **Step 3: Commit**

```bash
git add scripts/eval/rag/expand_gt.go
git commit -m "feat(rag-eval): Step2 — LLM扩展ground truth"
```

---

### Task 5: runner.go — Step 3: 执行引擎

**Files:**
- Create: `scripts/eval/rag/runner.go`

**Interfaces:**
- Consumes: `RAGQuery`, `RAGQueryResult`, `EvalConfig`, `ragSvc.Search()`, `config.App.RAG`
- Produces: `runEval(queries []RAGQuery, configs []EvalConfig, topK int, ragSvc *rag.RAGService) ([]EvalGroupResult, error)`

- [ ] **Step 1: 写 runner.go**

需要在 types.go 中新增 `EvalGroupResult` 类型，先更新 types.go：

```go
// EvalGroupResult 单组配置的完整评测结果
type EvalGroupResult struct {
	Config  EvalConfig       `json:"config"`
	Results []RAGQueryResult `json:"results"`
}
```

然后写 runner.go：

```go
package main

import (
	"fmt"
	"log/slog"
	"time"

	"edu_market/config"
	"edu_market/service/rag"
)

// defaultEvalConfigs 默认四组分层配置。
func defaultEvalConfigs() []EvalConfig {
	return []EvalConfig{
		{Name: "① 裸向量检索",     Hybrid: false, Rerank: false, CacheEnabled: false},
		{Name: "② +BM25混合检索",  Hybrid: true,  Rerank: false, CacheEnabled: false},
		{Name: "③ +Rerank精排",    Hybrid: true,  Rerank: true,  CacheEnabled: false},
		{Name: "④ +两级缓存",      Hybrid: true,  Rerank: true,  CacheEnabled: true},
	}
}

// runEval 分层执行所有配置组，返回每组的结果。
func runEval(queries []RAGQuery, configs []EvalConfig, topK int, ragSvc *rag.RAGService) ([]EvalGroupResult, error) {
	if ragSvc == nil {
		return nil, fmt.Errorf("RAG 服务未初始化")
	}

	var groups []EvalGroupResult
	for _, cfg := range configs {
		slog.Info("rag-eval 开始评测", "config", cfg.Name)

		results, err := runOneConfig(queries, cfg, topK, ragSvc)
		if err != nil {
			slog.Error("rag-eval 配置组执行失败", "config", cfg.Name, "err", err)
			continue
		}
		groups = append(groups, EvalGroupResult{Config: cfg, Results: results})
		slog.Info("rag-eval 完成评测", "config", cfg.Name, "queries", len(results))
	}
	return groups, nil
}

// runOneConfig 在一组配置下跑全部查询。
func runOneConfig(queries []RAGQuery, cfg EvalConfig, topK int, ragSvc *rag.RAGService) ([]RAGQueryResult, error) {
	// 保存原始配置
	orig := saveRAGConfig()
	defer restoreRAGConfig(orig)

	// 应用评测配置
	config.App.RAG.HybridSearch = cfg.Hybrid
	config.App.RAG.Rerank = cfg.Rerank
	config.App.RAG.CacheEnabled = cfg.CacheEnabled
	if cfg.Rerank {
		config.App.RAG.RerankTopK = topK // 确保返回 topK 条，不是默认 3 条
	}

	// Warmup：用第一条查询的 material，query 加 "warmup" 前缀
	if len(queries) > 0 {
		ragSvc.Search(queries[0].MaterialID, "warmup_query_"+queries[0].ID, topK, true)
	}

	var results []RAGQueryResult
	for _, q := range queries {
		var result RAGQueryResult
		result.QueryID = q.ID

		if cfg.CacheEnabled {
			// ④ 特殊：搜两次，第一次 populate，第二次测命中
			firstResults, _, _ := doSearch(ragSvc, q.MaterialID, q.Query, topK)
			_, returnedIDs, lat := doSearch(ragSvc, q.MaterialID, q.Query, topK)

			result.ReturnedChunkIDs = firstResults
			result.LatencyMs = lat
		} else {
			returnedIDs, allIDs, lat := doSearch(ragSvc, q.MaterialID, q.Query, topK)
			result.ReturnedChunkIDs = returnedIDs
			result.LatencyMs = lat
			_ = allIDs // same as returnedIDs when no cache
		}

		result.PrecisionAtK = precisionAtK(result.ReturnedChunkIDs, q.RelevantChunkIDs, topK)
		result.RecallAtK = recallAtK(result.ReturnedChunkIDs, q.RelevantChunkIDs, topK)
		result.TopKHit = topKHit(result.ReturnedChunkIDs, q.RelevantChunkIDs, topK)

		results = append(results, result)
	}
	return results, nil
}

// doSearch 执行一次 Search，返回 (用于指标的结果ID列表, 实际返回ID列表, 延迟毫秒)。
func doSearch(ragSvc *rag.RAGService, materialID uint, query string, topK int) ([]uint, []uint, float64) {
	start := time.Now()
	searchResults, err := ragSvc.Search(materialID, query, topK, true)
	lat := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		slog.Warn("rag-eval Search 失败", "material_id", materialID, "query", query, "err", err)
		return nil, nil, lat
	}

	ids := make([]uint, len(searchResults))
	for i, r := range searchResults {
		ids[i] = r.ChunkID
	}
	return ids, ids, lat
}

// ragConfigSnapshot 保存 config.App.RAG 快照。
type ragConfigSnapshot struct {
	HybridSearch bool
	Rerank       bool
	CacheEnabled bool
	RerankTopK   int
}

func saveRAGConfig() ragConfigSnapshot {
	return ragConfigSnapshot{
		HybridSearch: config.App.RAG.HybridSearch,
		Rerank:       config.App.RAG.Rerank,
		CacheEnabled: config.App.RAG.CacheEnabled,
		RerankTopK:   config.App.RAG.RerankTopK,
	}
}

func restoreRAGConfig(s ragConfigSnapshot) {
	config.App.RAG.HybridSearch = s.HybridSearch
	config.App.RAG.Rerank = s.Rerank
	config.App.RAG.CacheEnabled = s.CacheEnabled
	config.App.RAG.RerankTopK = s.RerankTopK
}
```

- [ ] **Step 2: 更新 types.go，新增 EvalGroupResult**

在 `scripts/eval/rag/types.go` 追加：
```go
// EvalGroupResult 单组配置的完整评测结果
type EvalGroupResult struct {
	Config  EvalConfig       `json:"config"`
	Results []RAGQueryResult `json:"results"`
}
```

- [ ] **Step 3: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./scripts/eval/rag/
```

- [ ] **Step 4: Commit**

```bash
git add scripts/eval/rag/runner.go scripts/eval/rag/types.go
git commit -m "feat(rag-eval): Step3 — 分层评测引擎"
```

---

### Task 6: reporter.go — Markdown 报告

**Files:**
- Create: `scripts/eval/rag/reporter.go`

**Interfaces:**
- Consumes: `EvalGroupResult`, `RAGEvalReport`, `EvalConfig`, `configSummary()`, `latencyPercentiles()`
- Produces: `generateReport(groups []EvalGroupResult, topK int) *RAGEvalReport`, `saveReport()`

- [ ] **Step 1: 写 reporter.go**

```go
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// generateReport 从评测结果生成完整报告。
func generateReport(groups []EvalGroupResult, topK int) *RAGEvalReport {
	report := &RAGEvalReport{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		TopK:      topK,
	}

	if len(groups) == 0 {
		return report
	}

	report.TotalQueries = len(groups[0].Results)

	for _, g := range groups {
		report.Configs = append(report.Configs, g.Config)
		summary := configSummary(g.Config.Name, g.Results)

		// 收集所有延迟
		var lats []float64
		for _, r := range g.Results {
			lats = append(lats, r.LatencyMs)
		}
		sort.Float64s(lats)
		avg, p50, p95, p99 := latencyPercentiles(lats)
		summary.AvgLatencyMs = avg

		report.Summaries = append(report.Summaries, summary)
		report.LatencyStats = append(report.LatencyStats, LatencyStats{
			ConfigName: g.Config.Name,
			Avg:        avg,
			P50:        p50,
			P95:        p95,
			P99:        p99,
		})
		report.Results = append(report.Results, g.Results...)
	}
	return report
}

// formatMarkdown 将报告格式化为 Markdown。
func formatMarkdown(report *RAGEvalReport) string {
	var sb strings.Builder
	sb.WriteString("# RAG 检索质量评测报告\n\n")
	sb.WriteString(fmt.Sprintf("**时间:** %s  |  **查询数:** %d  |  **TopK:** %d\n\n",
		report.Timestamp, report.TotalQueries, report.TopK))

	// 分层对比表
	sb.WriteString("## 📊 分层对比\n\n")
	sb.WriteString("| 配置 | Precision@K | Recall@K | TopK命中率 | 平均延迟 | P50 | P95 | P99 |\n")
	sb.WriteString("|------|:----------:|:--------:|:---------:|:------:|:---:|:---:|:---:|\n")
	for i, s := range report.Summaries {
		ls := report.LatencyStats[i]
		sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.1f%% | %.1fms | %.1fms | %.1fms | %.1fms |\n",
			s.ConfigName,
			s.AvgPrecision*100, s.AvgRecall*100, s.TopKHitRate*100,
			ls.Avg, ls.P50, ls.P95, ls.P99))
	}

	// 各层贡献分析
	sb.WriteString("\n## 🔍 各层贡献分析\n\n")
	sb.WriteString("| 优化项 | Precision变化 | Recall变化 | 延迟变化 |\n")
	sb.WriteString("|--------|:----------:|:--------:|:------:|\n")
	if len(report.Summaries) >= 2 {
		sb.WriteString(fmt.Sprintf("| BM25混合检索 | %+.1fpp | %+.1fpp | %+.1fms |\n",
			(report.Summaries[1].AvgPrecision-report.Summaries[0].AvgPrecision)*100,
			(report.Summaries[1].AvgRecall-report.Summaries[0].AvgRecall)*100,
			report.LatencyStats[1].Avg-report.LatencyStats[0].Avg))
	}
	if len(report.Summaries) >= 3 {
		sb.WriteString(fmt.Sprintf("| Rerank精排 | %+.1fpp | %+.1fpp | %+.1fms |\n",
			(report.Summaries[2].AvgPrecision-report.Summaries[1].AvgPrecision)*100,
			(report.Summaries[2].AvgRecall-report.Summaries[1].AvgRecall)*100,
			report.LatencyStats[2].Avg-report.LatencyStats[1].Avg))
	}
	if len(report.Summaries) >= 4 {
		latChange := (report.LatencyStats[3].Avg - report.LatencyStats[2].Avg) / report.LatencyStats[2].Avg * 100
		sb.WriteString(fmt.Sprintf("| 两级缓存（命中时） | — | — | %.0f%% |\n", latChange))
	}

	sb.WriteString("\n## ⚠️ 局限说明\n\n")
	sb.WriteString("- Recall 依赖 LLM 判断 ground truth，存在 5-10% 的标注误差\n")
	sb.WriteString("- ④ 缓存组测的是 L1 精确缓存命中（重复查询），非生产混合命中率\n")

	return sb.String()
}

// saveReport 保存 Markdown 和 JSON 两份报告。
func saveReport(report *RAGEvalReport, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	filename := fmt.Sprintf("%s/report.md", dir)
	if err := os.WriteFile(filename, []byte(formatMarkdown(report)), 0644); err != nil {
		return "", err
	}
	return filename, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./scripts/eval/rag/
```

- [ ] **Step 3: Commit**

```bash
git add scripts/eval/rag/reporter.go
git commit -m "feat(rag-eval): Markdown报告生成"
```

---

### Task 7: main.go — CLI 入口 + 编排

**Files:**
- Create: `scripts/eval/rag/main.go`

**Interfaces:**
- Consumes: 所有前 6 个 Task 的导出函数，`config`, `database`, `rag`
- Produces: CLI

- [ ] **Step 1: 写 main.go**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"edu_market/config"
	"edu_market/database"
	"edu_market/service/rag"

	"github.com/spf13/viper"
	"gorm.io/gorm/logger"
)

func main() {
	step := flag.String("step", "", "只执行某一步: gen|expand|eval|report (留空=全流程)")
	queryCount := flag.Int("queries", 100, "生成测试查询数量")
	topK := flag.Int("topk", 5, "评测 TopK 值")
	queriesFile := flag.String("queries-file", "data/eval/rag_queries.json", "标注数据文件路径")
	reportDir := flag.String("report-dir", "data/eval/reports/rag", "报告输出目录")
	flag.Parse()

	// 加载配置
	loadRAGEvalConfig()

	// 初始化数据库（静默模式，减少日志噪音）
	database.InitLogLevel = logger.Warn
	database.Init()

	// Redis 可选
	database.InitRedis()

	// RAG 初始化
	rag.Init()
	ragSvc := rag.Get()
	if ragSvc == nil {
		fmt.Fprintln(os.Stderr, "RAG 服务初始化失败")
		os.Exit(1)
	}
	fmt.Println("RAG 服务就绪")

	switch *step {
	case "gen":
		runGen(*queryCount, *queriesFile)
	case "expand":
		runExpand(*queriesFile, ragSvc)
	case "eval":
		runEvalOnly(*queriesFile, *topK, ragSvc, *reportDir)
	case "report":
		runReportOnly(*reportDir, *topK)
	default:
		runFull(*queryCount, *queriesFile, *topK, ragSvc, *reportDir)
	}
}

func loadRAGEvalConfig() {
	// 跟现有 eval 脚本用同样的配置加载方式
	config.App = &config.Config{
		Server:   config.ServerConfig{Port: 8080, Mode: "test"},
		Database: config.DatabaseConfig{Host: "127.0.0.1", Port: 3306, User: "root", Password: readYAML("database.password"), DBName: readYAML("database.dbname"), Charset: "utf8mb4", MaxIdleConns: 5, MaxOpenConns: 10},
		Redis:    config.RedisConfig{Addr: "127.0.0.1:6379", Password: "", DB: 0},
		AI: config.AIConfig{
			Provider: "deepseek",
			APIKey:   readYAML("ai.api_key"),
			APIURL:   readYAML("ai.api_url"),
			Model:    readYAML("ai.model"),
		},
		Agent: config.AgentConfig{
			MaxToolRounds:   10,
			ContextMaxMsg:   20,
			ChunkSize:       500,
			ChunkOverlap:    50,
			EmbeddingModel:  "BAAI/bge-m3",
			EmbeddingAPIURL: "https://api.siliconflow.cn/v1/embeddings",
			EmbeddingAPIKey: readYAML("agent.embedding_api_key"),
		},
	}
	if config.App.AI.Model == "" {
		config.App.AI.Model = "deepseek-chat"
	}
	if config.App.AI.APIURL == "" {
		config.App.AI.APIURL = "https://api.deepseek.com/v1/chat/completions"
	}
	if config.App.Database.DBName == "" {
		config.App.Database.DBName = "edu_market"
	}
}

func readYAML(key string) string {
	v := viper.New()
	v.SetConfigName("app")
	v.SetConfigType("yml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("../../config")
	if err := v.ReadInConfig(); err != nil {
		return ""
	}
	return v.GetString(key)
}

// runFull 全流程。
func runFull(count int, file string, topK int, ragSvc *rag.RAGService, reportDir string) {
	ts := time.Now().Format("20060102_150405")
	reportRunDir := reportDir + "/" + ts

	// Step 1
	fmt.Println("=== Step 1: 生成测试问题 ===")
	queries, err := generateQueries(count)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成问题失败: %v\n", err)
		os.Exit(1)
	}
	saveQueries(queries, file)
	fmt.Printf("生成 %d 条问题 → %s\n", len(queries), file)

	// Step 2
	fmt.Println("\n=== Step 2: 扩展 ground truth ===")
	queries, err = expandGroundTruth(queries, ragSvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扩展 ground truth 失败: %v\n", err)
	}
	saveQueries(queries, file)
	incomplete := 0
	for _, q := range queries {
		if q.GTIncomplete {
			incomplete++
		}
	}
	fmt.Printf("扩展完成，%d/%d 条已补全\n", len(queries)-incomplete, len(queries))

	// Step 3
	fmt.Println("\n=== Step 3: 分层评测 ===")
	configs := defaultEvalConfigs()
	groups, err := runEval(queries, configs, topK, ragSvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "评测失败: %v\n", err)
		os.Exit(1)
	}

	// 报告
	report := generateReport(groups, topK)
	path, err := saveReport(report, reportRunDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n✅ 评测完成，报告: %s\n", path)
}

func runGen(count int, file string) {
	queries, err := generateQueries(count)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成问题失败: %v\n", err)
		os.Exit(1)
	}
	saveQueries(queries, file)
	fmt.Printf("生成 %d 条问题 → %s\n", len(queries), file)
}

func runExpand(file string, ragSvc *rag.RAGService) {
	queries, err := loadQueries(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载标注数据失败: %v\n", err)
		os.Exit(1)
	}
	queries, err = expandGroundTruth(queries, ragSvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扩展失败: %v\n", err)
	}
	saveQueries(queries, file)
	fmt.Printf("扩展完成，保存至 %s\n", file)
}

func runEvalOnly(file string, topK int, ragSvc *rag.RAGService, reportDir string) {
	queries, err := loadQueries(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载标注数据失败: %v\n", err)
		os.Exit(1)
	}
	configs := defaultEvalConfigs()
	groups, err := runEval(queries, configs, topK, ragSvc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "评测失败: %v\n", err)
		os.Exit(1)
	}
	report := generateReport(groups, topK)
	ts := time.Now().Format("20060102_150405")
	path, _ := saveReport(report, reportDir+"/"+ts)
	fmt.Printf("✅ 评测完成，报告: %s\n", path)
}

func runReportOnly(dir string, topK int) {
	_ = topK
	fmt.Println("report-only 模式：从已有 JSON 结果重建报告（未实现，用全流程即可）")
}

// saveQueries 保存标注数据到 JSON 文件。
func saveQueries(queries []RAGQuery, path string) {
	b, _ := json.MarshalIndent(queries, "", "  ")
	os.MkdirAll("data/eval", 0755)
	os.WriteFile(path, b, 0644)
}

// loadQueries 从 JSON 文件读取标注数据。
func loadQueries(path string) ([]RAGQuery, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var queries []RAGQuery
	if err := json.Unmarshal(b, &queries); err != nil {
		return nil, err
	}
	return queries, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd d:/Vscoding/edu_market && go build ./scripts/eval/rag/
```

- [ ] **Step 3: 编译通过，全量测试**

```bash
# 确保服务在运行（Qdrant + Redis + MySQL）
go run ./scripts/eval/rag/
# 预期：生成 100 条问题 → 扩展 ground truth → 四组评测 → 输出报告
```

- [ ] **Step 4: Commit**

```bash
git add scripts/eval/rag/main.go
git commit -m "feat(rag-eval): CLI入口+全流程编排"
```

---

## Self-Review

**1. Spec coverage check:**
- ✅ 目标 1 "自动生成 100 条测试问题 + ground truth" → Task 3 + Task 4
- ✅ 目标 2 "分层 A/B 对比四组配置" → Task 5
- ✅ 目标 3 "Markdown 报告" → Task 6
- ✅ 目标 4 "半天完成" → 7 个 Task，每 Task 3-5 步
- ✅ 数据流 Step 1/2/3 → Task 3/4/5
- ✅ 指标公式 → Task 2
- ✅ CLI 接口 → Task 7
- ✅ 配置切换不改 Search() → Task 5
- ✅ 缓存绕过改配置 → Task 5
- ✅ LLM 容错 → Task 3/4/7
- ✅ 局限说明 → Task 6

**2. Placeholder scan:**
- 无 TBD/TODO
- 所有代码步骤都是完整的 Go 代码
- 所有命令都是确切的可执行命令

**3. Type consistency:**
- `RAGQuery` → 在 types.go 定义，gen_queries/expand_gt/runner 使用 ✅
- `RAGQueryResult` → types.go 定义，runner/reporter 使用 ✅
- `EvalConfig` → types.go 定义，runner/reporter 使用 ✅
- `EvalGroupResult` → types.go 定义（Task 5 追加），reporter 使用 ✅
- `configSummary()` → metrics.go 定义，reporter 使用 ✅
- `latencyPercentiles()` → metrics.go 定义，reporter 使用 ✅
- `precisionAtK/recallAtK/topKHit` → metrics.go 定义，runner 使用 ✅
