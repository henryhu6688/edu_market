package service

import (
	"sync"
	"testing"
)

func TestSemaphore_Basic(t *testing.T) {
	s := NewSemaphore(2)
	s.Acquire()
	s.Acquire()
	s.Release()
	s.Acquire() // 释放后又能获取了
	s.Release()
	s.Release()
}

func TestSemaphore_Concurrent(t *testing.T) {
	s := NewSemaphore(3)
	var wg sync.WaitGroup
	var count int32
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Acquire()
			mu.Lock()
			count++
			if count > 3 {
				t.Errorf("concurrent count exceeded: %d", count)
			}
			mu.Unlock()
			// 模拟工作
			mu.Lock()
			count--
			mu.Unlock()
			s.Release()
		}()
	}
	wg.Wait()
}
