package service

import (
	"chatroom/config"
	"chatroom/metrics"
	"chatroom/models"
	"chatroom/pool"
	"chatroom/ratelimit"
	"testing"
	"time"
)

// MockRepository 模擬排行榜存儲
type MockRepository struct {
	scores []models.GameScore
}

func (m *MockRepository) Add(s models.GameScore) error {
	m.scores = append(m.scores, s)
	return nil
}

func (m *MockRepository) Load() ([]models.GameScore, error) {
	return m.scores, nil
}

func (m *MockRepository) Save(scores []models.GameScore) error {
	m.scores = scores
	return nil
}

func (m *MockRepository) GetTop(n int) ([]models.GameScore, error) {
	return m.scores, nil
}

func (m *MockRepository) GetAll() []models.GameScore {
	return m.scores
}

func (m *MockRepository) Clear() error {
	m.scores = []models.GameScore{}
	return nil
}

func TestStateServiceV2_VoteLogic(t *testing.T) {
	// 1. 初始化依賴
	mockRepo := &MockRepository{}
	cfg := &config.Config{}
	// 為了測試邏輯，不需要真實的 WorkerPool 或 RateLimiter
	// 但 NewStateServiceWithDeps 需要它們
	wp := pool.NewWorkerPool(1, 1)
	rl := ratelimit.NewRateLimiter(10, time.Second, false)
	mt := metrics.GetMetrics()
	broadcastChan := make(chan models.Message, 10)

	service := NewStateServiceWithDeps(broadcastChan, mockRepo, wp, rl, mt, cfg)

	// 2. 測試發起投票
	roomName := "test_room"
	voteMsg := models.Message{
		Type:     "vote",
		Room:     roomName,
		Nickname: "UserA",
		Question: "Is Go great?",
		Options:  []string{"Yes", "No"},
	}

	service.ProcessMessage(voteMsg)

	// 驗證投票是否創建
	service.VotesMutex.RLock()
	vote, exists := service.Votes[roomName]
	service.VotesMutex.RUnlock()

	if !exists {
		t.Fatal("Vote should be created")
	}
	if vote.Question != "Is Go great?" {
		t.Errorf("Expected question 'Is Go great?', got '%s'", vote.Question)
	}
	if len(vote.Options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(vote.Options))
	}

	// 3. 測試投票回答
	answerMsg := models.Message{
		Type:     "vote_answer",
		Room:     roomName,
		Nickname: "UserB",
		Answer:   "Yes",
	}

	service.ProcessMessage(answerMsg)

	// 驗證票數
	service.VotesMutex.RLock()
	vote = service.Votes[roomName]
	count := vote.Options["Yes"]
	service.VotesMutex.RUnlock()

	if count != 1 {
		t.Errorf("Expected 1 vote for 'Yes', got %d", count)
	}

	// 4. 測試重複投票 (應該被忽略或處理，視邏輯而定)
	// 假設邏輯是每個用戶只能投一次
	service.ProcessMessage(answerMsg) // UserB 再次投票

	service.VotesMutex.RLock()
	vote = service.Votes[roomName]
	count = vote.Options["Yes"]
	service.VotesMutex.RUnlock()

	if count != 1 {
		t.Errorf("Expected vote count to remain 1 for duplicate vote, got %d", count)
	}
}

func TestStateServiceV2_GameScore(t *testing.T) {
	mockRepo := &MockRepository{}
	cfg := &config.Config{}
	wp := pool.NewWorkerPool(1, 1)
	rl := ratelimit.NewRateLimiter(10, time.Second, false)
	mt := metrics.GetMetrics()
	broadcastChan := make(chan models.Message, 10)

	service := NewStateServiceWithDeps(broadcastChan, mockRepo, wp, rl, mt, cfg)

	scoreMsg := models.Message{
		Type:     "game_score",
		Nickname: "Gamer1",
		Tries:    5,
		Time:     30,
	}

	service.ProcessMessage(scoreMsg)

	// 驗證 Repository 是否收到數據
	if len(mockRepo.scores) != 1 {
		t.Fatalf("Expected 1 score in repo, got %d", len(mockRepo.scores))
	}
	if mockRepo.scores[0].Nickname != "Gamer1" {
		t.Errorf("Expected nickname Gamer1, got %s", mockRepo.scores[0].Nickname)
	}
}

func TestStateServiceV2_QuizLogic(t *testing.T) {
	mockRepo := &MockRepository{}
	cfg := &config.Config{}
	wp := pool.NewWorkerPool(1, 1)
	rl := ratelimit.NewRateLimiter(10, time.Second, false)
	mt := metrics.GetMetrics()
	broadcastChan := make(chan models.Message, 10)

	service := NewStateServiceWithDeps(broadcastChan, mockRepo, wp, rl, mt, cfg)

	roomName := "quiz_room"

	// 1. 開始搶答
	quizMsg := models.Message{
		Type:     "quiz_start",
		Room:     roomName,
		Nickname: "Host",
		Question: "1+1=?",
		Answer:   "2",
	}

	service.ProcessMessage(quizMsg)

	service.QuizzesMutex.RLock()
	quiz, exists := service.Quizzes[roomName]
	service.QuizzesMutex.RUnlock()

	if !exists {
		t.Fatal("Quiz should be created")
	}
	if quiz.Answer != "2" {
		t.Errorf("Expected answer '2', got '%s'", quiz.Answer)
	}

	// 2. 錯誤回答
	wrongMsg := models.Message{
		Type:     "quiz_answer",
		Room:     roomName,
		Nickname: "Player1",
		Answer:   "3",
	}
	service.ProcessMessage(wrongMsg)

	// 驗證 Quiz 狀態 (應該還在進行中)
	service.QuizzesMutex.RLock()
	quiz = service.Quizzes[roomName]
	service.QuizzesMutex.RUnlock()
	if quiz == nil {
		t.Fatal("Quiz should still exist")
	}

	// 3. 正確回答
	correctMsg := models.Message{
		Type:     "quiz_answer",
		Room:     roomName,
		Nickname: "Player2",
		Answer:   "2",
	}
	service.ProcessMessage(correctMsg)

	// 驗證 Quiz 是否標記為非活躍
	service.QuizzesMutex.RLock()
	quiz = service.Quizzes[roomName]
	service.QuizzesMutex.RUnlock()

	if quiz == nil {
		t.Fatal("Quiz should exist")
	}
	if quiz.Active {
		t.Error("Quiz should be inactive after correct answer")
	}
}
