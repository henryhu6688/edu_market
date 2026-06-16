package service

import "golang.org/x/time/rate"

// Semaphore 并发控制（buffered channel 实现，保留给后续扩展用）
type Semaphore struct {
	ch chan struct{}
}

func NewSemaphore(capacity int) *Semaphore {
	return &Semaphore{ch: make(chan struct{}, capacity)}
}

func (s *Semaphore) Acquire() { s.ch <- struct{}{} }
func (s *Semaphore) Release() { <-s.ch }

// 令牌桶：限制 API 调用频率（非并发数），避免触发第三方 429
var (
	llmRate   = rate.NewLimiter(10, 3) // 每秒 10 次，突发 3 次
	embedRate = rate.NewLimiter(8, 2)  // 每秒 8 次，突发 2 次
)
