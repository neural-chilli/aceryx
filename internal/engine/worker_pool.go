package engine

import "sync"

// WorkerPool is a bounded goroutine pool.
type WorkerPool struct {
	sem chan struct{}
	wg  sync.WaitGroup
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
		defer func() {
			<-wp.sem
			wp.wg.Done()
		}()
		fn()
	}()
}

func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}
