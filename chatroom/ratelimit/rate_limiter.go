package ratelimit

import (
	"sync"
	"time"
)

// RateLimiter 限流器
type RateLimiter struct {
	mu         sync.RWMutex
	clients    map[string]*clientLimit
	maxMsg     int
	timeWindow time.Duration
	enabled    bool
}

// clientLimit 客戶端限制
type clientLimit struct {
	count     int
	resetTime time.Time
}

// NewRateLimiter 創建新的限流器
func NewRateLimiter(maxMsg int, timeWindow time.Duration, enabled bool) *RateLimiter {
	rl := &RateLimiter{
		clients:    make(map[string]*clientLimit),
		maxMsg:     maxMsg,
		timeWindow: timeWindow,
		enabled:    enabled,
	}

	// 定期清理過期記錄
	go rl.cleanup()

	return rl
}

// Allow 檢查是否允許通過
func (rl *RateLimiter) Allow(clientID string) bool {
	if !rl.enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	limit, exists := rl.clients[clientID]

	if !exists || now.After(limit.resetTime) {
		// 新客戶端或時間窗口已過期
		rl.clients[clientID] = &clientLimit{
			count:     1,
			resetTime: now.Add(rl.timeWindow),
		}
		return true
	}

	if limit.count < rl.maxMsg {
		limit.count++
		return true
	}

	return false
}

// Reset 重置客戶端限制
func (rl *RateLimiter) Reset(clientID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.clients, clientID)
}

// GetRemaining 獲取剩餘配額
func (rl *RateLimiter) GetRemaining(clientID string) int {
	if !rl.enabled {
		return rl.maxMsg
	}

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	limit, exists := rl.clients[clientID]
	if !exists || time.Now().After(limit.resetTime) {
		return rl.maxMsg
	}

	remaining := rl.maxMsg - limit.count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// cleanup 定期清理過期記錄
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.timeWindow)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for clientID, limit := range rl.clients {
			if now.After(limit.resetTime) {
				delete(rl.clients, clientID)
			}
		}
		rl.mu.Unlock()
	}
}

// SetEnabled 設置是否啟用
func (rl *RateLimiter) SetEnabled(enabled bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.enabled = enabled
}
