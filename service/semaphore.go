package service

// Semaphore 并发控制（buffered channel 实现）
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore 创建信号量
func NewSemaphore(capacity int) *Semaphore {
	return &Semaphore{ch: make(chan struct{}, capacity)}
}

// Acquire 获取许可（阻塞）
func (s *Semaphore) Acquire() { s.ch <- struct{}{} }

// Release 释放许可
func (s *Semaphore) Release() { <-s.ch }

// 全局并发控制
var LLMSem = NewSemaphore(5)   // LLM API 全局最多 5 并发
var EmbedSem = NewSemaphore(3) // Embedding API 全局最多 3 并发
var ParserSem = NewSemaphore(2) // 文件解析全局最多 2 并发
