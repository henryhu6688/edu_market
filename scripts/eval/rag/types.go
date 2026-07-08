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

// EvalGroupResult 单组配置的完整评测结果
type EvalGroupResult struct {
	Config  EvalConfig       `json:"config"`
	Results []RAGQueryResult `json:"results"`
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
	Timestamp    string            `json:"timestamp"`
	TopK         int               `json:"top_k"`
	TotalQueries int               `json:"total_queries"`
	Configs      []EvalConfig      `json:"configs"`
	Results      []RAGQueryResult  `json:"results"`
	Summaries    []ConfigSummary   `json:"summaries"`
	LatencyStats []LatencyStats    `json:"latency_stats"`
}
