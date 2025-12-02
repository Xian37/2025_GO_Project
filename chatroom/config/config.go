package config

import (
	"os"
	"strconv"
	"time"
)

// Config 應用程式配置
type Config struct {
	Server    ServerConfig
	WebSocket WSConfig
	Storage   StorageConfig
	RateLimit RateLimitConfig
}

// ServerConfig 伺服器配置
type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// WSConfig WebSocket 配置
type WSConfig struct {
	MaxMessageSize  int64
	PingInterval    time.Duration
	PongWait        time.Duration
	WriteWait       time.Duration
	ReadBufferSize  int
	WriteBufferSize int
}

// StorageConfig 儲存配置
type StorageConfig struct {
	LeaderboardFile string
	HistoryMaxSize  int
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled     bool
	MaxMessages int
	TimeWindow  time.Duration
}

// Load 從環境變數載入配置
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			ReadTimeout:     getDuration("READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getDuration("WRITE_TIMEOUT", 15*time.Second),
			ShutdownTimeout: getDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		WebSocket: WSConfig{
			MaxMessageSize:  getInt64("WS_MAX_MESSAGE_SIZE", 5*1024*1024), // 5MB
			PingInterval:    getDuration("WS_PING_INTERVAL", 54*time.Second),
			PongWait:        getDuration("WS_PONG_WAIT", 60*time.Second),
			WriteWait:       getDuration("WS_WRITE_WAIT", 10*time.Second),
			ReadBufferSize:  getInt("WS_READ_BUFFER", 1024),
			WriteBufferSize: getInt("WS_WRITE_BUFFER", 1024),
		},
		Storage: StorageConfig{
			LeaderboardFile: getEnv("LEADERBOARD_FILE", "leaderboard.json"),
			HistoryMaxSize:  getInt("HISTORY_MAX_SIZE", 100),
		},
		RateLimit: RateLimitConfig{
			Enabled:     getBool("RATE_LIMIT_ENABLED", true),
			MaxMessages: getInt("RATE_LIMIT_MAX_MSG", 10),
			TimeWindow:  getDuration("RATE_LIMIT_WINDOW", 10*time.Second),
		},
	}
}

// 輔助函數
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
