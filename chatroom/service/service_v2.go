package service

import (
	"chatroom/config"
	"chatroom/logger"
	"chatroom/metrics"
	"chatroom/models"
	"chatroom/pool"
	"chatroom/ratelimit"
	"chatroom/repository"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StateServiceV2 增強版狀態服務
type StateServiceV2 struct {
	// 原有欄位
	Rooms     map[string]map[*models.Client]bool
	History   map[string][]models.Message
	Votes     map[string]*models.Vote
	Quizzes   map[string]*models.Quiz
	Broadcast chan models.Message

	DrawStates    map[string]*models.DrawState
	RoomPasswords map[string]string

	// 互斥鎖
	RoomsMutex         sync.RWMutex
	HistoryMutex       sync.RWMutex
	VotesMutex         sync.RWMutex
	QuizzesMutex       sync.RWMutex
	DrawStateMutex     sync.RWMutex
	RoomPasswordsMutex sync.RWMutex

	// 新增依賴
	leaderboardRepo repository.LeaderboardRepository
	workerPool      *pool.WorkerPool
	rateLimiter     *ratelimit.RateLimiter
	metrics         *metrics.Metrics
	config          *config.Config
}

// NewStateServiceWithDeps 使用依賴注入創建服務
func NewStateServiceWithDeps(
	broadcastChan chan models.Message,
	repo repository.LeaderboardRepository,
	pool *pool.WorkerPool,
	limiter *ratelimit.RateLimiter,
	metrics *metrics.Metrics,
	cfg *config.Config,
) *StateServiceV2 {
	s := &StateServiceV2{
		Rooms:           make(map[string]map[*models.Client]bool),
		History:         make(map[string][]models.Message),
		Votes:           make(map[string]*models.Vote),
		Quizzes:         make(map[string]*models.Quiz),
		Broadcast:       broadcastChan,
		DrawStates:      make(map[string]*models.DrawState),
		RoomPasswords:   make(map[string]string),
		leaderboardRepo: repo,
		workerPool:      pool,
		rateLimiter:     limiter,
		metrics:         metrics,
		config:          cfg,
	}

	logger.Info("StateService initialized with dependencies")
	return s
}

// HandleMessageLoopWithContext 帶上下文的訊息處理循環
func (s *StateServiceV2) HandleMessageLoopWithContext(ctx context.Context) {
	logger.Info("Starting message loop")

	for {
		select {
		case msg, ok := <-s.Broadcast:
			if !ok {
				// Channel 已關閉
				logger.Info("Broadcast channel closed")
				return
			}
			// 使用 worker pool 處理訊息
			s.workerPool.Submit(func() {
				start := time.Now()
				s.ProcessMessage(msg)
				s.metrics.RecordLatency(time.Since(start))
			})

		case <-ctx.Done():
			logger.Info("Message loop stopped by context")
			return
		}
	}
}

// broadcastMessage 廣播訊息到房間
func (s *StateServiceV2) broadcastMessage(msg models.Message) {
	s.RoomsMutex.RLock()
	clients := s.Rooms[msg.Room]
	s.RoomsMutex.RUnlock()

	if clients == nil {
		return
	}

	// 添加到歷史記錄
	if msg.Type == "chat" || msg.Type == "image" || msg.Type == "voice" {
		s.AddHistory(msg)
	}

	// 廣播給所有客戶端
	for client := range clients {
		if !s.safeWriteJSON(client, msg) {
			s.metrics.IncrementMessagesFailed()
		} else {
			s.metrics.IncrementMessages()
		}
	}
}

// safeWriteJSON 安全地寫入 JSON
func (s *StateServiceV2) safeWriteJSON(client *models.Client, msg models.Message) bool {
	client.Mu.Lock()
	defer client.Mu.Unlock()

	if err := client.Conn.WriteJSON(msg); err != nil {
		if !strings.Contains(err.Error(), "use of closed network connection") &&
			!strings.Contains(err.Error(), "connection was aborted") &&
			!strings.Contains(err.Error(), "broken pipe") {
			logger.Warn("WriteJSON error",
				zap.String("nickname", client.Nickname),
				zap.Error(err))
		}
		return false
	}
	return true
}

// RegisterClient 註冊客戶端
func (s *StateServiceV2) RegisterClient(client *models.Client) {
	s.RoomsMutex.Lock()
	if s.Rooms[client.Room] == nil {
		s.Rooms[client.Room] = make(map[*models.Client]bool)
		s.metrics.IncrementRooms()
	}
	s.Rooms[client.Room][client] = true
	s.RoomsMutex.Unlock()

	s.metrics.IncrementConnections()

	if !strings.HasPrefix(client.Room, "_") {
		s.BroadcastRoomList()
	}

	logger.Info("Client registered",
		zap.String("nickname", client.Nickname),
		zap.String("room", client.Room))
}

// UnregisterClient 取消註冊客戶端
func (s *StateServiceV2) UnregisterClient(client *models.Client) string {
	roomToUpdate := client.Room

	s.RoomsMutex.Lock()
	delete(s.Rooms[roomToUpdate], client)
	roomIsEmpty := len(s.Rooms[roomToUpdate]) == 0
	if roomIsEmpty {
		delete(s.Rooms, roomToUpdate)
		s.metrics.DecrementRooms()

		if roomToUpdate == "_draw_game_" {
			s.DrawStateMutex.Lock()
			delete(s.DrawStates, roomToUpdate)
			s.DrawStateMutex.Unlock()
		}
	}
	s.RoomsMutex.Unlock()

	s.metrics.DecrementConnections()

	if !strings.HasPrefix(roomToUpdate, "_") {
		s.BroadcastRoomList()
	}

	logger.Info("Client unregistered",
		zap.String("nickname", client.Nickname),
		zap.String("room", roomToUpdate),
		zap.Bool("room_empty", roomIsEmpty))

	return roomToUpdate
}

// SwitchRoom 切換房間
func (s *StateServiceV2) SwitchRoom(client *models.Client, newRoom, password string) (string, error) {
	oldRoom := client.Room
	isSwitchingToGame := strings.HasPrefix(newRoom, "_")
	isSwitchingFromGame := strings.HasPrefix(oldRoom, "_")

	// 檢查密碼
	s.RoomPasswordsMutex.RLock()
	expectedPassword, passwordRequired := s.RoomPasswords[newRoom]
	s.RoomPasswordsMutex.RUnlock()

	// 檢查房間是否存在
	s.RoomsMutex.RLock()
	_, roomExists := s.Rooms[newRoom]
	s.RoomsMutex.RUnlock()

	isNewRoom := !roomExists

	// 驗證密碼
	if passwordRequired {
		if password != expectedPassword {
			errorType := "wrong_password"
			if password == "" {
				errorType = "password_required"
			}
			logger.Warn("Password verification failed",
				zap.String("room", newRoom),
				zap.String("error", errorType))
			return "", fmt.Errorf("%s", errorType)
		}
	} else if isNewRoom && password != "" {
		// 新房間設置密碼
		s.RoomPasswordsMutex.Lock()
		s.RoomPasswords[newRoom] = password
		s.RoomPasswordsMutex.Unlock()
		logger.Info("Room password set", zap.String("room", newRoom))
	}

	// 發送離開訊息
	if !isSwitchingFromGame {
		leaveMsg := models.Message{
			Type:      "leave",
			Room:      oldRoom,
			Content:   client.Nickname + " 離開了聊天室",
			Timestamp: time.Now().Format("15:04"),
		}
		s.Broadcast <- leaveMsg
	}

	// 從舊房間移除並加入新房間
	s.RoomsMutex.Lock()
	delete(s.Rooms[oldRoom], client)
	if len(s.Rooms[oldRoom]) == 0 {
		delete(s.Rooms, oldRoom)
		s.metrics.DecrementRooms()

		// 清理空房間的密碼
		s.RoomPasswordsMutex.Lock()
		delete(s.RoomPasswords, oldRoom)
		s.RoomPasswordsMutex.Unlock()
	}

	client.Room = newRoom
	if s.Rooms[newRoom] == nil {
		s.Rooms[newRoom] = make(map[*models.Client]bool)
		s.metrics.IncrementRooms()
	}
	s.Rooms[newRoom][client] = true
	s.RoomsMutex.Unlock()

	// 更新房間列表和在線人數
	if !isSwitchingFromGame || !isSwitchingToGame {
		s.BroadcastRoomList()
		go s.BroadcastOnlineCount()
	}

	logger.Info("Client switched room",
		zap.String("nickname", client.Nickname),
		zap.String("from", oldRoom),
		zap.String("to", newRoom),
		zap.Bool("new_room", isNewRoom))

	return oldRoom, nil
}

// AddHistory 添加歷史記錄
func (s *StateServiceV2) AddHistory(msg models.Message) {
	s.HistoryMutex.Lock()
	defer s.HistoryMutex.Unlock()

	s.History[msg.Room] = append(s.History[msg.Room], msg)

	// 限制歷史記錄大小
	maxSize := s.config.Storage.HistoryMaxSize
	if len(s.History[msg.Room]) > maxSize {
		s.History[msg.Room] = s.History[msg.Room][len(s.History[msg.Room])-maxSize:]
	}
}

// SendHistory 發送歷史記錄
func (s *StateServiceV2) SendHistory(client *models.Client) {
	s.HistoryMutex.RLock()
	history := s.History[client.Room]
	s.HistoryMutex.RUnlock()

	for _, msg := range history {
		s.safeWriteJSON(client, msg)
	}

	logger.Debug("History sent",
		zap.String("room", client.Room),
		zap.Int("count", len(history)))
}

// CheckRateLimitexceeded 檢查限流
func (s *StateServiceV2) CheckRateLimit(clientID string) bool {
	if !s.rateLimiter.Allow(clientID) {
		s.metrics.IncrementRateLimitErrors()
		logger.Warn("Rate limit exceeded", zap.String("client_id", clientID))
		return false
	}
	return true
}

// BroadcastRoomList 廣播房間列表
func (s *StateServiceV2) BroadcastRoomList() {
	// 先收集房間資訊，避免長時間持有鎖
	roomInfo := make(map[string]bool)

	// 確保聊天大廳始終在列表中
	roomInfo["聊天大廳"] = false

	s.RoomsMutex.RLock()
	roomNames := make([]string, 0, len(s.Rooms))
	for roomName := range s.Rooms {
		if !strings.HasPrefix(roomName, "_") {
			roomNames = append(roomNames, roomName)
		}
	}
	s.RoomsMutex.RUnlock()

	// 檢查每個房間是否有密碼
	s.RoomPasswordsMutex.RLock()
	for _, roomName := range roomNames {
		roomInfo[roomName] = s.RoomPasswords[roomName] != ""
	}
	s.RoomPasswordsMutex.RUnlock()

	// 建立訊息
	msg := models.Message{
		Type:     "room_list",
		RoomInfo: roomInfo,
	}

	// 廣播給所有非遊戲房間的客戶端
	s.RoomsMutex.RLock()
	allClients := make([]*models.Client, 0)
	for roomName, clients := range s.Rooms {
		if !strings.HasPrefix(roomName, "_") {
			for client := range clients {
				allClients = append(allClients, client)
			}
		}
	}
	s.RoomsMutex.RUnlock()

	// 發送訊息
	for _, client := range allClients {
		s.safeWriteJSON(client, msg)
	}
}

// BroadcastOnlineCount 廣播在線人數
func (s *StateServiceV2) BroadcastOnlineCount() {
	s.RoomsMutex.RLock()
	defer s.RoomsMutex.RUnlock()

	roomCounts := make(map[string]int)
	for room, clients := range s.Rooms {
		if !strings.HasPrefix(room, "_") {
			roomCounts[room] = len(clients)
		}
	}

	for room, count := range roomCounts {
		msg := models.Message{
			Type:    "online_count",
			Room:    room,
			Content: fmt.Sprintf("%d", count),
		}

		if clients, ok := s.Rooms[room]; ok {
			for client := range clients {
				s.safeWriteJSON(client, msg)
			}
		}
	}
}

// UpdateLeaderboard 更新排行榜
func (s *StateServiceV2) UpdateLeaderboard(score models.GameScore) {
	if err := s.leaderboardRepo.Add(score); err != nil {
		logger.Error("Failed to update leaderboard", zap.Error(err))
		return
	}

	s.broadcastLeaderboard()

	// 發送系統公告
	announceMsg := models.Message{
		Type:      "chat",
		Room:      "聊天大廳",
		Nickname:  "🏆 系統",
		Avatar:    "🏆",
		Content:   fmt.Sprintf("%s 在猜數字遊戲中獲勝了 (猜 %d 次, %d 秒)！", score.Nickname, score.Tries, score.Time),
		Timestamp: time.Now().Format("15:04:05"),
	}
	s.Broadcast <- announceMsg

	logger.Info("Leaderboard updated",
		zap.String("player", score.Nickname),
		zap.Int("tries", score.Tries),
		zap.Int("time", score.Time))
}

// broadcastLeaderboard 廣播排行榜
func (s *StateServiceV2) broadcastLeaderboard() {
	scores := s.leaderboardRepo.GetAll()
	scoresJSON, err := json.Marshal(scores)
	if err != nil {
		logger.Error("Failed to marshal leaderboard", zap.Error(err))
		return
	}

	msg := models.Message{
		Type:    "leaderboard_update",
		Content: string(scoresJSON),
		Room:    "_game_",
	}

	s.BroadcastToRoom(msg)
}

// BroadcastToRoom 廣播到指定房間
func (s *StateServiceV2) BroadcastToRoom(msg models.Message) {
	s.broadcastMessage(msg)
}

// GetLeaderboardJSON 獲取排行榜 JSON
func (s *StateServiceV2) GetLeaderboardJSON() ([]byte, error) {
	scores := s.leaderboardRepo.GetAll()
	return json.Marshal(scores)
}

// 其他方法保持不變，從原 service.go 複製...
// (Votes, Quizzes, DrawStates 相關方法)
