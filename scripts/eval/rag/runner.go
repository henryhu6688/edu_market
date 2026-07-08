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
		{Name: "裸向量检索", Hybrid: false, Rerank: false, CacheEnabled: false, BM25Enabled: false},
		{Name: "+BM25混合检索", Hybrid: false, Rerank: false, CacheEnabled: false, BM25Enabled: true},
		{Name: "+Rerank精排", Hybrid: false, Rerank: true, CacheEnabled: false, BM25Enabled: true},
		{Name: "+两级缓存", Hybrid: false, Rerank: true, CacheEnabled: true, BM25Enabled: true},
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
	config.App.RAG.BM25Enabled = cfg.BM25Enabled

	// Warmup：用第一条查询的 material，query 加前缀
	if len(queries) > 0 {
		ragSvc.Search(queries[0].MaterialID, "warmup_query_"+queries[0].ID, topK, true)
	}

	var results []RAGQueryResult
	for _, q := range queries {
		var result RAGQueryResult
		result.QueryID = q.ID

		if cfg.CacheEnabled {
			// 缓存组特殊：搜两次，第一次 populate，第二次测命中
			firstResults, _ := doSearch(ragSvc, q.MaterialID, q.Query, topK)
			_, lat := doSearch(ragSvc, q.MaterialID, q.Query, topK)

			result.ReturnedChunkIDs = firstResults
			result.LatencyMs = lat
		} else {
			returnedIDs, lat := doSearch(ragSvc, q.MaterialID, q.Query, topK)
			result.ReturnedChunkIDs = returnedIDs
			result.LatencyMs = lat
		}

		result.PrecisionAtK = precisionAtK(result.ReturnedChunkIDs, q.RelevantChunkIDs, topK)
		result.RecallAtK = recallAtK(result.ReturnedChunkIDs, q.RelevantChunkIDs, topK)
		result.TopKHit = topKHit(result.ReturnedChunkIDs, q.RelevantChunkIDs, topK)

		results = append(results, result)
	}
	return results, nil
}

// doSearch 执行一次 Search，返回 (返回的 chunkID 列表, 延迟毫秒)。
func doSearch(ragSvc *rag.RAGService, materialID uint, query string, topK int) ([]uint, float64) {
	start := time.Now()
	searchResults, err := ragSvc.Search(materialID, query, topK, true)
	lat := float64(time.Since(start).Microseconds()) / 1000.0

	if err != nil {
		slog.Warn("rag-eval Search 失败", "material_id", materialID, "query", query, "err", err)
		return nil, lat
	}

	ids := make([]uint, len(searchResults))
	for i, r := range searchResults {
		ids[i] = r.ChunkID
	}
	return ids, lat
}

// ragConfigSnapshot 保存 config.App.RAG 快照。
type ragConfigSnapshot struct {
	HybridSearch bool
	Rerank       bool
	CacheEnabled bool
	BM25Enabled  bool
}

func saveRAGConfig() ragConfigSnapshot {
	return ragConfigSnapshot{
		HybridSearch: config.App.RAG.HybridSearch,
		Rerank:       config.App.RAG.Rerank,
		CacheEnabled: config.App.RAG.CacheEnabled,
		BM25Enabled:  config.App.RAG.BM25Enabled,
	}
}

func restoreRAGConfig(s ragConfigSnapshot) {
	config.App.RAG.HybridSearch = s.HybridSearch
	config.App.RAG.Rerank = s.Rerank
	config.App.RAG.CacheEnabled = s.CacheEnabled
	config.App.RAG.BM25Enabled = s.BM25Enabled
}
