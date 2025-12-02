package pool

import (
	"context"
	"sync"
)

// Worker 工作單元
type Worker struct {
	ID      int
	JobChan chan func()
	quit    chan bool
}

// WorkerPool 工作池
type WorkerPool struct {
	workers    []*Worker
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
		workers:    make([]*Worker, workerSize),
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
		worker := &Worker{
			ID:      i,
			JobChan: make(chan func()),
			quit:    make(chan bool),
		}
		p.workers[i] = worker
		p.wg.Add(1)
		go p.work(worker)
	}

	// 任務分發器
	go p.dispatch()
}

// work 工作邏輯
func (p *WorkerPool) work(w *Worker) {
	defer p.wg.Done()

	for {
		select {
		case job := <-w.JobChan:
			if job != nil {
				job()
			}
		case <-w.quit:
			return
		case <-p.ctx.Done():
			return
		}
	}
}

// dispatch 分發任務
func (p *WorkerPool) dispatch() {
	for {
		select {
		case job := <-p.jobQueue:
			// 嘗試找到可用的 worker
			dispatched := false
			for _, worker := range p.workers {
				select {
				case worker.JobChan <- job:
					dispatched = true
				default:
					continue
				}
				if dispatched {
					break
				}
			}
			// 如果所有 worker 都忙碌，阻塞等待第一個 worker
			if !dispatched {
				p.workers[0].JobChan <- job
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
	p.cancel()

	for _, worker := range p.workers {
		worker.quit <- true
	}

	p.wg.Wait()
	close(p.jobQueue)
}
