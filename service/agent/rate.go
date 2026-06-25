package agent

import "golang.org/x/time/rate"

// 令牌桶：限制 API 调用频率（非并发数），避免触发第三方 429
var (
	llmRate   = rate.NewLimiter(10, 3) // 每秒 10 次，突发 3 次
	embedRate = rate.NewLimiter(8, 2)  // 每秒 8 次，突发 2 次
)
