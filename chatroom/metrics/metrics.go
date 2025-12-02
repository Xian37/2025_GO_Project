package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics 應用程式指標
type Metrics struct {
	// 連線相關
	TotalConnections    int64
	ActiveConnections   int64
	TotalDisconnections int64

	// 訊息相關
	TotalMessages    int64
	MessagesSent     int64
	MessagesReceived int64
	MessagesFailed   int64

	// 房間相關
	ActiveRooms int64
	TotalRooms  int64

	// 錯誤相關
	TotalErrors      int64
	RateLimitErrors  int64
	ConnectionErrors int64

	// 效能相關
	mu                sync.RWMutex
	AverageLatency    time.Duration
	MaxLatency        time.Duration
	latencySamples    []time.Duration
	maxLatencySamples int
}

var globalMetrics *Metrics
var once sync.Once

// GetMetrics 獲取全局 Metrics 實例
func GetMetrics() *Metrics {
	once.Do(func() {
		globalMetrics = &Metrics{
			maxLatencySamples: 1000,
			latencySamples:    make([]time.Duration, 0, 1000),
		}
	})
	return globalMetrics
}

// IncrementConnections 增加連線計數
func (m *Metrics) IncrementConnections() {
	atomic.AddInt64(&m.TotalConnections, 1)
	atomic.AddInt64(&m.ActiveConnections, 1)
}

// DecrementConnections 減少連線計數
func (m *Metrics) DecrementConnections() {
	atomic.AddInt64(&m.ActiveConnections, -1)
	atomic.AddInt64(&m.TotalDisconnections, 1)
}

// IncrementMessages 增加訊息計數
func (m *Metrics) IncrementMessages() {
	atomic.AddInt64(&m.TotalMessages, 1)
	atomic.AddInt64(&m.MessagesSent, 1)
}

// IncrementMessagesReceived 增加接收訊息計數
func (m *Metrics) IncrementMessagesReceived() {
	atomic.AddInt64(&m.MessagesReceived, 1)
}

// IncrementMessagesFailed 增加失敗訊息計數
func (m *Metrics) IncrementMessagesFailed() {
	atomic.AddInt64(&m.MessagesFailed, 1)
}

// IncrementRooms 增加房間計數
func (m *Metrics) IncrementRooms() {
	atomic.AddInt64(&m.TotalRooms, 1)
	atomic.AddInt64(&m.ActiveRooms, 1)
}

// DecrementRooms 減少房間計數
func (m *Metrics) DecrementRooms() {
	atomic.AddInt64(&m.ActiveRooms, -1)
}

// IncrementErrors 增加錯誤計數
func (m *Metrics) IncrementErrors() {
	atomic.AddInt64(&m.TotalErrors, 1)
}

// IncrementRateLimitErrors 增加限流錯誤計數
func (m *Metrics) IncrementRateLimitErrors() {
	atomic.AddInt64(&m.RateLimitErrors, 1)
}

// IncrementConnectionErrors 增加連線錯誤計數
func (m *Metrics) IncrementConnectionErrors() {
	atomic.AddInt64(&m.ConnectionErrors, 1)
}

// RecordLatency 記錄延遲
func (m *Metrics) RecordLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.latencySamples = append(m.latencySamples, latency)
	if len(m.latencySamples) > m.maxLatencySamples {
		m.latencySamples = m.latencySamples[1:]
	}

	// 更新平均延遲
	var total time.Duration
	for _, l := range m.latencySamples {
		total += l
	}
	m.AverageLatency = total / time.Duration(len(m.latencySamples))

	// 更新最大延遲
	if latency > m.MaxLatency {
		m.MaxLatency = latency
	}
}

// GetSnapshot 獲取指標快照
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSnapshot{
		TotalConnections:    atomic.LoadInt64(&m.TotalConnections),
		ActiveConnections:   atomic.LoadInt64(&m.ActiveConnections),
		TotalDisconnections: atomic.LoadInt64(&m.TotalDisconnections),
		TotalMessages:       atomic.LoadInt64(&m.TotalMessages),
		MessagesSent:        atomic.LoadInt64(&m.MessagesSent),
		MessagesReceived:    atomic.LoadInt64(&m.MessagesReceived),
		MessagesFailed:      atomic.LoadInt64(&m.MessagesFailed),
		ActiveRooms:         atomic.LoadInt64(&m.ActiveRooms),
		TotalRooms:          atomic.LoadInt64(&m.TotalRooms),
		TotalErrors:         atomic.LoadInt64(&m.TotalErrors),
		RateLimitErrors:     atomic.LoadInt64(&m.RateLimitErrors),
		ConnectionErrors:    atomic.LoadInt64(&m.ConnectionErrors),
		AverageLatency:      m.AverageLatency,
		MaxLatency:          m.MaxLatency,
	}
}

// MetricsSnapshot 指標快照
type MetricsSnapshot struct {
	TotalConnections    int64
	ActiveConnections   int64
	TotalDisconnections int64
	TotalMessages       int64
	MessagesSent        int64
	MessagesReceived    int64
	MessagesFailed      int64
	ActiveRooms         int64
	TotalRooms          int64
	TotalErrors         int64
	RateLimitErrors     int64
	ConnectionErrors    int64
	AverageLatency      time.Duration
	MaxLatency          time.Duration
}

// Reset 重置所有指標
func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.TotalConnections, 0)
	atomic.StoreInt64(&m.ActiveConnections, 0)
	atomic.StoreInt64(&m.TotalDisconnections, 0)
	atomic.StoreInt64(&m.TotalMessages, 0)
	atomic.StoreInt64(&m.MessagesSent, 0)
	atomic.StoreInt64(&m.MessagesReceived, 0)
	atomic.StoreInt64(&m.MessagesFailed, 0)
	atomic.StoreInt64(&m.ActiveRooms, 0)
	atomic.StoreInt64(&m.TotalRooms, 0)
	atomic.StoreInt64(&m.TotalErrors, 0)
	atomic.StoreInt64(&m.RateLimitErrors, 0)
	atomic.StoreInt64(&m.ConnectionErrors, 0)

	m.mu.Lock()
	m.AverageLatency = 0
	m.MaxLatency = 0
	m.latencySamples = make([]time.Duration, 0, m.maxLatencySamples)
	m.mu.Unlock()
}
