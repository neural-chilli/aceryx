package engine

import (
	"sync"
	"sync/atomic"
)

// WorkerPool is a bounded goroutine pool.
type WorkerPool struct {
	sem chan struct{}
	wg  sync.WaitGroup
	n   atomic.Int64
}

func NewWorkerPool(maxConcurrent int) *WorkerPool {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	return &WorkerPool{sem: make(chan struct{}, maxConcurrent)}
}

func (wp *WorkerPool) Submit(fn func()) {
	wp.sem <- struct{}{}
	wp.wg.Add(1)
	go func() {
		wp.n.Add(1)
		defer func() {
			wp.n.Add(-1)
			<-wp.sem
			wp.wg.Done()
		}()
		fn()
	}()
}

func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

func (wp *WorkerPool) Active() int {
	return int(wp.n.Load())
}

func (wp *WorkerPool) Capacity() int {
	return cap(wp.sem)
}
