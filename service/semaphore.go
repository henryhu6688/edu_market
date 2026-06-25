package service

// Semaphore 并发控制（buffered channel 实现，保留给后续扩展用）
type Semaphore struct {
	ch chan struct{}
}

func NewSemaphore(capacity int) *Semaphore {
	return &Semaphore{ch: make(chan struct{}, capacity)}
}

func (s *Semaphore) Acquire() { s.ch <- struct{}{} }
func (s *Semaphore) Release() { <-s.ch }
