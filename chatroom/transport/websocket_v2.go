package transport

import (
	"chatroom/config"
	"chatroom/logger"
	"chatroom/models"
	"chatroom/service"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WebsocketHandlerV2 增強版 WebSocket 處理器
type WebsocketHandlerV2 struct {
	Service  *service.StateServiceV2
	config   *config.Config
	upgrader websocket.Upgrader
}

// NewWebsocketHandlerWithConfig 創建帶配置的 WebSocket 處理器
func NewWebsocketHandlerWithConfig(s *service.StateServiceV2, cfg *config.Config) *WebsocketHandlerV2 {
	return &WebsocketHandlerV2{
		Service: s,
		config:  cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin:     func(r *http.Request) bool { return true },
			ReadBufferSize:  cfg.WebSocket.ReadBufferSize,
			WriteBufferSize: cfg.WebSocket.WriteBufferSize,
		},
	}
}

// HandleConnections 處理 WebSocket 連線
func (h *WebsocketHandlerV2) HandleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Upgrade error", zap.Error(err))
		return
	}
	defer ws.Close()

	// 設置讀取限制
	ws.SetReadLimit(h.config.WebSocket.MaxMessageSize)

	// 讀取初始訊息
	var initMsg models.Message
	err = ws.ReadJSON(&initMsg)
	if err != nil {
		logger.Error("Init message read error", zap.Error(err))
		return
	}

	// 創建客戶端
	client := &models.Client{
		Conn:     ws,
		Nickname: initMsg.Nickname,
		Room:     initMsg.Room,
		Avatar:   initMsg.Avatar,
	}

	// 註冊客戶端
	h.Service.RegisterClient(client)

	// 發送歷史記錄
	if !strings.HasPrefix(client.Room, "_") {
		h.Service.SendHistory(client)

		// 發送加入訊息
		joinMsg := models.Message{
			Type:      "join",
			Room:      client.Room,
			Content:   client.Nickname + " 加入了聊天室",
			Timestamp: time.Now().Format("15:04"),
		}
		h.Service.Broadcast <- joinMsg
		go h.Service.BroadcastOnlineCount()
	}

	// 啟動讀取循環和心跳檢測
	h.readLoopWithHeartbeat(client)
}

// readLoopWithHeartbeat 帶心跳檢測的讀取循環
func (h *WebsocketHandlerV2) readLoopWithHeartbeat(client *models.Client) {
	// 設置 pong 處理器
	client.Conn.SetReadDeadline(time.Now().Add(h.config.WebSocket.PongWait))
	client.Conn.SetPongHandler(func(string) error {
		client.Conn.SetReadDeadline(time.Now().Add(h.config.WebSocket.PongWait))
		return nil
	})

	// 啟動 ping ticker
	ticker := time.NewTicker(h.config.WebSocket.PingInterval)
	defer ticker.Stop()

	// 用於停止 ping goroutine
	done := make(chan struct{})
	defer close(done)

	// Ping goroutine
	go func() {
		for {
			select {
			case <-ticker.C:
				client.Mu.Lock()
				err := client.Conn.WriteControl(
					websocket.PingMessage,
					[]byte{},
					time.Now().Add(h.config.WebSocket.WriteWait),
				)
				client.Mu.Unlock()

				if err != nil {
					logger.Warn("Ping failed",
						zap.String("nickname", client.Nickname),
						zap.Error(err))
					return
				}
			case <-done:
				return
			}
		}
	}()

	// 讀取循環
	for {
		var msg models.Message
		err := client.Conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Warn("Unexpected close",
					zap.String("nickname", client.Nickname),
					zap.Error(err))
			}
			roomToUpdate := h.Service.UnregisterClient(client)

			// 廣播在線人數更新
			if !strings.HasPrefix(roomToUpdate, "_") {
				go h.Service.BroadcastOnlineCount()
			}
			break
		}

		// 限流檢查
		if msg.UserId != "" && !h.Service.CheckRateLimit(msg.UserId) {
			warningMsg := models.Message{
				Type:    "error",
				Content: "發送訊息過於頻繁，請稍後再試",
			}
			client.Mu.Lock()
			client.Conn.WriteJSON(warningMsg)
			client.Mu.Unlock()
			continue
		}

		// 處理訊息
		msg.Avatar = client.Avatar
		msg.Nickname = client.Nickname

		if msg.Type != "switch" {
			msg.Room = client.Room
		}

		// 根據訊息類型處理
		h.handleMessage(client, msg)
	}
}

// handleMessage 處理不同類型的訊息
func (h *WebsocketHandlerV2) handleMessage(client *models.Client, msg models.Message) {
	switch msg.Type {
	case "switch":
		h.handleSwitchRoom(client, msg)
	case "get_leaderboard":
		h.handleGetLeaderboard(client)
	case "game_win":
		h.handleGameWin(msg)
	case "vote":
		h.handleVote(msg)
	case "quiz":
		h.handleQuiz(msg)
	default:
		// 其他訊息直接廣播
		if msg.Timestamp == "" {
			msg.Timestamp = time.Now().Format("15:04:05")
		}
		h.Service.Broadcast <- msg
	}

	logger.Debug("Message handled",
		zap.String("type", msg.Type),
		zap.String("from", client.Nickname))
}

// handleSwitchRoom 處理切換房間
func (h *WebsocketHandlerV2) handleSwitchRoom(client *models.Client, msg models.Message) {
	oldRoom, err := h.Service.SwitchRoom(client, msg.Room, msg.Password)
	if err != nil {
		// 發送錯誤訊息給客戶端
		errorMsg := models.Message{
			Type:    err.Error(), // "password_required" 或 "wrong_password"
			Room:    msg.Room,    // 包含房間名稱
			Content: "密碼驗證失敗",
		}
		client.Mu.Lock()
		client.Conn.WriteJSON(errorMsg)
		client.Mu.Unlock()
		return
	}

	// 發送切換成功確認訊息給客戶端
	confirmMsg := models.Message{
		Type:    "switch_success",
		Room:    msg.Room,
		Content: oldRoom, // 舊房間名稱
	}
	client.Mu.Lock()
	client.Conn.WriteJSON(confirmMsg)
	client.Mu.Unlock()

	// 發送歷史訊息
	h.Service.SendHistory(client)

	// 發送加入訊息
	if !strings.HasPrefix(msg.Room, "_") {
		joinMsg := models.Message{
			Type:      "join",
			Room:      msg.Room,
			Content:   client.Nickname + " 加入了聊天室",
			Timestamp: time.Now().Format("15:04"),
		}
		h.Service.Broadcast <- joinMsg
	}

	logger.Info("Room switched",
		zap.String("nickname", client.Nickname),
		zap.String("from", oldRoom),
		zap.String("to", msg.Room))
}

// handleGetLeaderboard 處理獲取排行榜請求
func (h *WebsocketHandlerV2) handleGetLeaderboard(client *models.Client) {
	scoresJSON, err := h.Service.GetLeaderboardJSON()
	if err != nil {
		logger.Error("Error marshalling leaderboard", zap.Error(err))
		return
	}

	resp := models.Message{
		Type:    "leaderboard_update",
		Content: string(scoresJSON),
	}

	client.Mu.Lock()
	client.Conn.WriteJSON(resp)
	client.Mu.Unlock()
}

// handleGameWin 處理遊戲勝利
func (h *WebsocketHandlerV2) handleGameWin(msg models.Message) {
	score := models.GameScore{
		Nickname: msg.Nickname,
		Tries:    msg.Tries,
		Time:     msg.Time,
	}
	h.Service.UpdateLeaderboard(score)
}

// handleVote 處理投票
func (h *WebsocketHandlerV2) handleVote(msg models.Message) {
	// 投票邏輯...
	h.Service.Broadcast <- msg
}

// handleQuiz 處理搶答
func (h *WebsocketHandlerV2) handleQuiz(msg models.Message) {
	// 搶答邏輯...
	h.Service.Broadcast <- msg
}
