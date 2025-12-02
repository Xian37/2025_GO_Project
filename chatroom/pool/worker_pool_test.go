package pool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool(t *testing.T) {
	t.Run("Basic execution", func(t *testing.T) {
		pool := NewWorkerPool(5, 10)
		pool.Start()
		defer pool.Stop()

		var counter int32
		var wg sync.WaitGroup

		// 提交 100 個任務
		for i := 0; i < 100; i++ {
			wg.Add(1)
			pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
				wg.Done()
			})
		}

		wg.Wait()

		if counter != 100 {
			t.Errorf("Expected counter to be 100, got %d", counter)
		}
	})

	t.Run("Concurrent submissions", func(t *testing.T) {
		pool := NewWorkerPool(10, 100)
		pool.Start()
		defer pool.Stop()

		var counter int32
		var wg sync.WaitGroup

		// 並發提交任務
		for i := 0; i < 10; i++ {
			go func() {
				for j := 0; j < 10; j++ {
					wg.Add(1)
					pool.Submit(func() {
						atomic.AddInt32(&counter, 1)
						time.Sleep(time.Millisecond)
						wg.Done()
					})
				}
			}()
		}

		wg.Wait()

		if counter != 100 {
			t.Errorf("Expected counter to be 100, got %d", counter)
		}
	})

	t.Run("Stop gracefully", func(t *testing.T) {
		pool := NewWorkerPool(3, 10)
		pool.Start()

		var counter int32
		var wg sync.WaitGroup

		// 提交一些任務
		for i := 0; i < 10; i++ {
			wg.Add(1)
			pool.Submit(func() {
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt32(&counter, 1)
				wg.Done()
			})
		}

		// 等待任務完成
		wg.Wait()

		// 停止 pool
		pool.Stop()

		// 驗證所有任務都執行了
		if counter != 10 {
			t.Errorf("Expected counter to be 10, got %d", counter)
		}
	})
}

func BenchmarkWorkerPool(b *testing.B) {
	pool := NewWorkerPool(10, 1000)
	pool.Start()
	defer pool.Stop()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.Submit(func() {
				// 模擬工作
				time.Sleep(time.Microsecond)
			})
		}
	})
}

func BenchmarkWorkerPoolVsGoroutine(b *testing.B) {
	b.Run("WorkerPool", func(b *testing.B) {
		pool := NewWorkerPool(10, 1000)
		pool.Start()
		defer pool.Stop()

		for i := 0; i < b.N; i++ {
			pool.Submit(func() {
				time.Sleep(time.Microsecond)
			})
		}
	})

	b.Run("DirectGoroutine", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			go func() {
				time.Sleep(time.Microsecond)
			}()
		}
	})
}
