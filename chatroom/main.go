package main

import (
	"chatroom/config"
	"chatroom/logger"
	"chatroom/metrics"
	"chatroom/models"
	"chatroom/pool"
	"chatroom/ratelimit"
	"chatroom/repository"
	"chatroom/service"
	"chatroom/transport"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func main() {
	// 1. 初始化日誌系統
	if err := logger.InitDefault(); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting chatroom server...")

	// 2. 載入配置
	cfg := config.Load()
	logger.Info("Configuration loaded",
		zap.String("port", cfg.Server.Port),
		zap.Bool("rate_limit", cfg.RateLimit.Enabled))

	// 3. 初始化 Repository
	leaderboardRepo := repository.NewFileLeaderboardRepository(cfg.Storage.LeaderboardFile)
	logger.Info("Repository initialized")

	// 4. 初始化 Worker Pool
	workerPool := pool.NewWorkerPool(10, 100) // 10 workers, 100 queue size
	workerPool.Start()
	logger.Info("Worker pool started with 10 workers")

	// 5. 初始化 Rate Limiter
	rateLimiter := ratelimit.NewRateLimiter(
		cfg.RateLimit.MaxMessages,
		cfg.RateLimit.TimeWindow,
		cfg.RateLimit.Enabled,
	)
	logger.Info("Rate limiter initialized")

	// 6. 初始化 Metrics
	appMetrics := metrics.GetMetrics()
	logger.Info("Metrics initialized")

	// 7. 建立訊息通道
	broadcastChan := make(chan models.Message, 100)

	// 8. 初始化 Service
	stateService := service.NewStateServiceWithDeps(
		broadcastChan,
		leaderboardRepo,
		workerPool,
		rateLimiter,
		appMetrics,
		cfg,
	)
	logger.Info("State service initialized")

	// 9. 啟動訊息處理循環
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stateService.HandleMessageLoopWithContext(ctx)

	// 10. 初始化 WebSocket Handler
	wsHandler := transport.NewWebsocketHandlerWithConfig(stateService, cfg)

	// 11. 設置 HTTP 路由
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", wsHandler.HandleConnections)

	// 新增 metrics endpoint
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		snapshot := appMetrics.GetSnapshot()
		fmt.Fprintf(w, "Total Connections: %d\n", snapshot.TotalConnections)
		fmt.Fprintf(w, "Active Connections: %d\n", snapshot.ActiveConnections)
		fmt.Fprintf(w, "Total Messages: %d\n", snapshot.TotalMessages)
		fmt.Fprintf(w, "Active Rooms: %d\n", snapshot.ActiveRooms)
		fmt.Fprintf(w, "Average Latency: %v\n", snapshot.AverageLatency)
		fmt.Fprintf(w, "Total Errors: %d\n", snapshot.TotalErrors)
	})

	// 12. 建立 HTTP Server
	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// 13. 啟動伺服器
	go func() {
		logger.Info("Server starting", zap.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	// 14. 優雅關機處理
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// 建立關機超時上下文
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// 1. 先取消上下文，停止所有 goroutine
	cancel()
	logger.Info("Context cancelled")

	// 2. 停止接受新連線
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}
	logger.Info("HTTP server stopped")

	// 3. 停止 worker pool
	workerPool.Stop()
	logger.Info("Worker pool stopped")

	// 4. 關閉 broadcast channel
	close(broadcastChan)
	logger.Info("Broadcast channel closed")

	// 5. 同步日誌（有超時保護）
	syncDone := make(chan struct{})
	go func() {
		logger.Sync()
		close(syncDone)
	}()

	select {
	case <-syncDone:
		// 日誌同步成功
	case <-time.After(2 * time.Second):
		// 超時，強制退出
		fmt.Println("Logger sync timeout, forcing exit")
	}

	fmt.Println("Server exited successfully")
	os.Exit(0)
}