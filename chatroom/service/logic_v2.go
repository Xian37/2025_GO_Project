package service

import (
	"chatroom/logger"
	"chatroom/models"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ProcessMessage 處理所有類型的訊息 (V2)
func (s *StateServiceV2) ProcessMessage(msg models.Message) {
	if msg.Timestamp == "" {
		msg.Timestamp = time.Now().Format("15:04:05")
	}

	logger.Debug("Processing Message",
		zap.String("type", msg.Type),
		zap.String("room", msg.Room),
		zap.String("nick", msg.Nickname))

	switch msg.Type {
	case "draw_start", "draw_move", "draw_end", "clear_canvas":
		s.handleDraw(msg)
	case "game_score":
		s.handleGameScore(msg)
	case "reaction":
		s.handleReaction(msg)
	case "vote":
		s.handleVoteStart(msg)
	case "vote_answer":
		s.handleVoteAnswer(msg)
	case "quiz_start":
		s.handleQuizStart(msg)
	case "quiz_answer":
		s.handleQuizAnswer(msg)
	case "get_leaderboard":
		s.handleGetLeaderboard(msg)
	case "chat":
		s.handleChat(msg)
	default: // image, voice, join, leave, etc.
		s.handleDefault(msg)
	}
}

// handleDraw
func (s *StateServiceV2) handleDraw(msg models.Message) {
	var clientsToWrite []*models.Client

	s.RoomsMutex.RLock()
	clientsInRoom, ok := s.Rooms[msg.Room]
	if ok {
		for client := range clientsInRoom {
			if client.Nickname != msg.Nickname {
				clientsToWrite = append(clientsToWrite, client)
			}
		}
	}
	s.RoomsMutex.RUnlock()

	for _, client := range clientsToWrite {
		s.safeWriteJSON(client, msg)
	}
}

// handleGameScore
func (s *StateServiceV2) handleGameScore(msg models.Message) {
	newScore := models.GameScore{
		Nickname: msg.Nickname, Avatar: msg.Avatar, Tries: msg.Tries, Time: msg.Time,
	}
	s.UpdateLeaderboard(newScore)
}

// handleReaction
func (s *StateServiceV2) handleReaction(msg models.Message) {
	s.BroadcastToRoom(msg)
}

// handleVoteStart
func (s *StateServiceV2) handleVoteStart(msg models.Message) {
	s.VotesMutex.Lock()
	optionsMap := make(map[string]int)
	for _, opt := range msg.Options {
		optionsMap[opt] = 0
	}
	s.Votes[msg.Room] = &models.Vote{
		Question: msg.Question, Options: optionsMap, Voters: make(map[string]bool),
	}
	s.VotesMutex.Unlock()

	s.AddHistory(msg)
	s.BroadcastToRoom(msg)
}

// handleVoteAnswer
func (s *StateServiceV2) handleVoteAnswer(msg models.Message) {
	s.VotesMutex.Lock()
	currentVote, exists := s.Votes[msg.Room]
	var resultMsg models.Message
	if exists && !currentVote.Voters[msg.Nickname] {
		if _, ok := currentVote.Options[msg.Answer]; ok {
			currentVote.Options[msg.Answer]++
			currentVote.Voters[msg.Nickname] = true
		}
		resultMsg = models.Message{
			Type: "vote_result", Room: msg.Room, Content: msg.Content, Results: currentVote.Options,
		}
	}
	s.VotesMutex.Unlock()

	if resultMsg.Type != "" {
		s.BroadcastToRoom(resultMsg)
	}
}

// handleQuizStart
func (s *StateServiceV2) handleQuizStart(msg models.Message) {
	s.QuizzesMutex.Lock()
	s.Quizzes[msg.Room] = &models.Quiz{
		Question: msg.Question, Answer: msg.Answer, Active: true,
	}
	s.QuizzesMutex.Unlock()

	broadcastMsg := models.Message{
		Type: "quiz_start", Room: msg.Room, Nickname: msg.Nickname, Avatar: msg.Avatar,
		Question: msg.Question, Timestamp: msg.Timestamp,
	}

	s.AddHistory(broadcastMsg)
	s.BroadcastToRoom(broadcastMsg)
}

// handleQuizAnswer
func (s *StateServiceV2) handleQuizAnswer(msg models.Message) {
	s.QuizzesMutex.Lock()
	quiz, exists := s.Quizzes[msg.Room]
	isCorrect := exists && quiz.Active && (quiz.Answer == msg.Answer)
	var correctAnswer string
	if isCorrect {
		quiz.Active = false
	}
	if exists {
		correctAnswer = quiz.Answer
	}
	s.QuizzesMutex.Unlock()

	if isCorrect {
		resultMsg := models.Message{
			Type: "quiz_result", Room: msg.Room, Nickname: msg.Nickname, Avatar: msg.Avatar,
			Content: msg.Content, Answer: correctAnswer, Timestamp: time.Now().Format("15:04:05"),
		}
		s.AddHistory(resultMsg)
		s.BroadcastToRoom(resultMsg)
	}
}

// handleGetLeaderboard
func (s *StateServiceV2) handleGetLeaderboard(msg models.Message) {
	s.broadcastLeaderboard()
}

// handleChat
func (s *StateServiceV2) handleChat(msg models.Message) {
	// 1. "Draw & Guess" Logic
	if msg.Room == "_draw_game_" {
		s.DrawStateMutex.Lock()
		if s.DrawStates[msg.Room] == nil {
			s.DrawStates[msg.Room] = &models.DrawState{}
		}
		state := s.DrawStates[msg.Room]

		// Handle /setword command
		if strings.HasPrefix(msg.Content, "/setword ") {
			word := strings.TrimPrefix(msg.Content, "/setword ")
			logger.Info("Processing /setword", zap.String("word", word), zap.String("nick", msg.Nickname))

			if word != "" {
				state.CurrentWord = word
				state.CurrentDrawer = msg.Nickname
				s.DrawStateMutex.Unlock() // Unlock before sending messages

				// Notify drawer
				drawerMsg := models.Message{
					Type: "new_round_drawer", Room: msg.Room, Content: word,
				}
				// Notify guessers
				guesserMsg := models.Message{
					Type: "new_round_guesser", Room: msg.Room, Nickname: msg.Nickname, Content: strings.Repeat("*", len([]rune(word))),
				}

				var drawerClient *models.Client
				var guesserClients []*models.Client

				s.RoomsMutex.RLock()
				clientsInRoom, ok := s.Rooms[msg.Room]
				if ok {
					for client := range clientsInRoom {
						if client.Nickname == msg.Nickname {
							drawerClient = client
						} else {
							guesserClients = append(guesserClients, client)
						}
					}
				} else {
					logger.Warn("No clients found in _draw_game_ during /setword")
				}
				s.RoomsMutex.RUnlock()

				if drawerClient != nil {
					s.safeWriteJSON(drawerClient, drawerMsg)
					logger.Info("Sent new_round_drawer to", zap.String("nick", drawerClient.Nickname))
				} else {
					logger.Warn("Drawer client not found in room", zap.String("target", msg.Nickname))
				}

				for _, client := range guesserClients {
					s.safeWriteJSON(client, guesserMsg)
				}
				return
			}
		}

		// Handle Guessing
		if state.CurrentWord != "" && msg.Nickname != state.CurrentDrawer && normalize(msg.Content) == normalize(state.CurrentWord) {
			broadcastMsg := models.Message{
				Type: "guess_correct", Room: msg.Room, Nickname: msg.Nickname,
				Content: state.CurrentWord, Timestamp: time.Now().Format("15:04:05"),
			}
			s.BroadcastToRoom(broadcastMsg)
			state.CurrentWord = ""
			state.CurrentDrawer = ""
			s.DrawStateMutex.Unlock()
			return
		}
		s.DrawStateMutex.Unlock()
	}

	// 2. Standard Chat Logic
	if !strings.HasPrefix(msg.Room, "_") {
		// Easter Egg: Gopher Rain
		if msg.Content == "/gopher" {
			broadcastMsg := models.Message{
				Type: "gopher_rain", Room: msg.Room, Nickname: msg.Nickname,
				Content: "Let it rain Gophers!", Timestamp: time.Now().Format("15:04:05"),
			}
			s.BroadcastToRoom(broadcastMsg)
			return
		}

		s.AddHistory(msg)
	}
	s.BroadcastToRoom(msg)
}

// handleDefault
func (s *StateServiceV2) handleDefault(msg models.Message) {
	if !strings.HasPrefix(msg.Room, "_") {
		if msg.Type == "image" || msg.Type == "voice" || msg.Content != "" {
			s.AddHistory(msg)
		}
	}
	s.BroadcastToRoom(msg)
}
