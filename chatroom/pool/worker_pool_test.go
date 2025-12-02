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

		// 提交 10 個任務（減少數量加快測試）
		for i := 0; i < 10; i++ {
			wg.Add(1)
			pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
				wg.Done()
			})
		}

		// 添加超時保護
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			if counter != 10 {
				t.Errorf("Expected counter to be 10, got %d", counter)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Test timeout after 5 seconds")
		}
	})

	t.Run("Concurrent submissions", func(t *testing.T) {
		pool := NewWorkerPool(5, 50)
		pool.Start()
		defer pool.Stop()

		var counter int32
		var wg sync.WaitGroup
		var submitWg sync.WaitGroup

		// 並發提交任務（減少數量）
		for i := 0; i < 5; i++ {
			submitWg.Add(1)
			go func() {
				defer submitWg.Done()
				for j := 0; j < 4; j++ {
					wg.Add(1)
					pool.Submit(func() {
						atomic.AddInt32(&counter, 1)
						wg.Done()
					})
				}
			}()
		}

		// 等待所有任務提交完成
		submitWg.Wait()

		// 添加超時保護
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			if counter != 20 {
				t.Errorf("Expected counter to be 20, got %d", counter)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Test timeout after 5 seconds")
		}
	})

	t.Run("Stop gracefully", func(t *testing.T) {
		pool := NewWorkerPool(3, 10)
		pool.Start()

		var counter int32
		var wg sync.WaitGroup

		// 提交一些任務
		for i := 0; i < 6; i++ {
			wg.Add(1)
			pool.Submit(func() {
				atomic.AddInt32(&counter, 1)
				wg.Done()
			})
		}

		// 等待任務完成（帶超時）
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("Tasks did not complete in time")
		}

		// 停止 pool
		pool.Stop()

		// 驗證所有任務都執行了
		if counter != 6 {
			t.Errorf("Expected counter to be 6, got %d", counter)
		}
	})
}

func BenchmarkWorkerPool(b *testing.B) {
	pool := NewWorkerPool(10, 100)
	pool.Start()
	defer pool.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(1)
		pool.Submit(func() {
			wg.Done()
		})
		wg.Wait()
	}
}
