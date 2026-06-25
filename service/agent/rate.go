package agent

import "golang.org/x/time/rate"

// 令牌桶：限制 LLM API 调用频率（非并发数），避免触发第三方 429
// Embedding API 限流在 service/rag/embedding.go 中独立管理
var llmRate = rate.NewLimiter(10, 3) // 每秒 10 次，突发 3 次
