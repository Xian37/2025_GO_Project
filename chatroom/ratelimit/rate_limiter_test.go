package ratelimit

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	t.Run("Allow within limit", func(t *testing.T) {
		rl := NewRateLimiter(5, time.Second, true)
		clientID := "test_client"

		// 應該允許前 5 次
		for i := 0; i < 5; i++ {
			if !rl.Allow(clientID) {
				t.Errorf("Request %d should be allowed", i+1)
			}
		}

		// 第 6 次應該被拒絕
		if rl.Allow(clientID) {
			t.Error("Request 6 should be denied")
		}
	})

	t.Run("Reset after time window", func(t *testing.T) {
		rl := NewRateLimiter(2, 100*time.Millisecond, true)
		clientID := "test_client2"

		// 用完配額
		rl.Allow(clientID)
		rl.Allow(clientID)

		if rl.Allow(clientID) {
			t.Error("Should be denied")
		}

		// 等待時間窗口過期
		time.Sleep(150 * time.Millisecond)

		// 應該重新允許
		if !rl.Allow(clientID) {
			t.Error("Should be allowed after time window")
		}
	})

	t.Run("Disabled limiter", func(t *testing.T) {
		rl := NewRateLimiter(1, time.Second, false)
		clientID := "test_client3"

		// 即使配額為 1，也應該允許多次
		for i := 0; i < 10; i++ {
			if !rl.Allow(clientID) {
				t.Errorf("Disabled limiter should allow all requests")
			}
		}
	})

	t.Run("GetRemaining", func(t *testing.T) {
		rl := NewRateLimiter(5, time.Second, true)
		clientID := "test_client4"

		remaining := rl.GetRemaining(clientID)
		if remaining != 5 {
			t.Errorf("Expected 5 remaining, got %d", remaining)
		}

		rl.Allow(clientID)
		remaining = rl.GetRemaining(clientID)
		if remaining != 4 {
			t.Errorf("Expected 4 remaining, got %d", remaining)
		}
	})

	t.Run("Reset", func(t *testing.T) {
		rl := NewRateLimiter(2, time.Second, true)
		clientID := "test_client5"

		// 用完配額
		rl.Allow(clientID)
		rl.Allow(clientID)

		// 重置
		rl.Reset(clientID)

		// 應該重新允許
		if !rl.Allow(clientID) {
			t.Error("Should be allowed after reset")
		}
	})
}

func BenchmarkRateLimiter(b *testing.B) {
	rl := NewRateLimiter(1000, time.Second, true)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			clientID := "client_" + string(rune(i%100))
			rl.Allow(clientID)
			i++
		}
	})
}
