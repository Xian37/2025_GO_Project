package pool

import (
	"context"
	"sync"
)

// WorkerPool 工作池
type WorkerPool struct {
	jobQueue   chan func()
	workerSize int
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewWorkerPool 創建新的工作池
func NewWorkerPool(workerSize int, queueSize int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		jobQueue:   make(chan func(), queueSize),
		workerSize: workerSize,
		ctx:        ctx,
		cancel:     cancel,
	}

	return pool
}

// Start 啟動工作池
func (p *WorkerPool) Start() {
	for i := 0; i < p.workerSize; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// worker 工作邏輯
func (p *WorkerPool) worker() {
	defer p.wg.Done()

	for {
		select {
		case job, ok := <-p.jobQueue:
			if !ok {
				return // Channel 已關閉
			}
			if job != nil {
				job()
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// Submit 提交任務
func (p *WorkerPool) Submit(job func()) {
	select {
	case p.jobQueue <- job:
	case <-p.ctx.Done():
		return
	}
}

// Stop 停止工作池
func (p *WorkerPool) Stop() {
	close(p.jobQueue) // 關閉任務隊列，worker 會自然退出
	p.wg.Wait()       // 等待所有 worker 完成
	p.cancel()        // 取消 context
}
