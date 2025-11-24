package service

import (
	"chatroom/models"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const leaderboardFile = "leaderboard.json"

// safeWriteJSON safely writes JSON to a client, ignoring closed connections
func safeWriteJSON(client *models.Client, msg models.Message) bool {
	client.Mu.Lock()
	defer client.Mu.Unlock()

	if err := client.Conn.WriteJSON(msg); err != nil {
		// Only log if it's not a typical disconnect error
		if !strings.Contains(err.Error(), "use of closed network connection") &&
			!strings.Contains(err.Error(), "connection was aborted") &&
			!strings.Contains(err.Error(), "broken pipe") {
			log.Printf("WriteJSON error: %v", err)
		}
		return false
	}
	return true
}

// StateService
type StateService struct {
	Rooms     map[string]map[*models.Client]bool
	History   map[string][]models.Message
	Votes     map[string]*models.Vote
	Quizzes   map[string]*models.Quiz
	Broadcast chan models.Message

	Leaderboard   []models.GameScore
	DrawStates    map[string]*models.DrawState
	RoomPasswords map[string]string

	RoomsMutex         sync.RWMutex
	HistoryMutex       sync.RWMutex
	VotesMutex         sync.RWMutex
	QuizzesMutex       sync.RWMutex
	LeaderboardMutex   sync.RWMutex
	DrawStateMutex     sync.RWMutex
	RoomPasswordsMutex sync.RWMutex
}

// NewStateService
func NewStateService(broadcastChan chan models.Message) *StateService {
	s := &StateService{
		Rooms:     make(map[string]map[*models.Client]bool),
		History:   make(map[string][]models.Message),
		Votes:     make(map[string]*models.Vote),
		Quizzes:   make(map[string]*models.Quiz),
		Broadcast: broadcastChan,

		Leaderboard:   make([]models.GameScore, 0),
		DrawStates:    make(map[string]*models.DrawState),
		RoomPasswords: make(map[string]string),
	}
	s.loadLeaderboard()
	return s
}

// RegisterClient
func (s *StateService) RegisterClient(client *models.Client) {
	s.RoomsMutex.Lock()
	if s.Rooms[client.Room] == nil {
		s.Rooms[client.Room] = make(map[*models.Client]bool)
	}
	s.Rooms[client.Room][client] = true
	s.RoomsMutex.Unlock()

	if !strings.HasPrefix(client.Room, "_") {
		s.BroadcastRoomList()
	}
}

// UnregisterClient
func (s *StateService) UnregisterClient(client *models.Client) string {
	roomToUpdate := client.Room

	s.RoomsMutex.Lock()
	delete(s.Rooms[roomToUpdate], client)
	roomIsEmpty := len(s.Rooms[roomToUpdate]) == 0
	if roomIsEmpty {
		delete(s.Rooms, roomToUpdate)
		if roomToUpdate == "_draw_game_" {
			s.DrawStateMutex.Lock()
			delete(s.DrawStates, roomToUpdate)
			s.DrawStateMutex.Unlock()
		}
	}
	s.RoomsMutex.Unlock()

	if roomIsEmpty {
		s.RoomPasswordsMutex.Lock()
		delete(s.RoomPasswords, roomToUpdate)
		s.RoomPasswordsMutex.Unlock()
	}

	if !strings.HasPrefix(roomToUpdate, "_") {
		leaveMsg := models.Message{
			Type: "leave", Room: roomToUpdate, Content: client.Nickname + " 離開了聊天室", Timestamp: time.Now().Format("15:04"),
		}
		s.Broadcast <- leaveMsg
		s.BroadcastRoomList()
		go s.BroadcastOnlineCount()
	}
	return roomToUpdate
}

// SwitchRoom
func (s *StateService) SwitchRoom(client *models.Client, newRoom, password string) (string, error) {
	oldRoom := client.Room
	isSwitchingToGame := strings.HasPrefix(newRoom, "_")
	isSwitchingFromGame := strings.HasPrefix(oldRoom, "_")

	s.RoomPasswordsMutex.RLock()
	expectedPassword, passwordRequired := s.RoomPasswords[newRoom]
	s.RoomPasswordsMutex.RUnlock()

	s.RoomsMutex.RLock()
	_, roomExists := s.Rooms[newRoom]
	s.RoomsMutex.RUnlock()

	isNewRoom := !roomExists

	if passwordRequired {
		if password != expectedPassword {
			errorType := "wrong_password"
			if password == "" {
				errorType = "password_required"
			}
			return "", errors.New(errorType)
		}
	} else if isNewRoom && password != "" {
		s.RoomPasswordsMutex.Lock()
		s.RoomPasswords[newRoom] = password
		s.RoomPasswordsMutex.Unlock()
	}

	if !isSwitchingFromGame {
		leaveMsg := models.Message{
			Type: "leave", Room: oldRoom, Content: client.Nickname + " 離開了聊天室", Timestamp: time.Now().Format("15:04"),
		}
		s.Broadcast <- leaveMsg
	}

	s.RoomsMutex.Lock()
	delete(s.Rooms[oldRoom], client)
	if len(s.Rooms[oldRoom]) == 0 {
		delete(s.Rooms, oldRoom)
		s.RoomPasswordsMutex.Lock()
		delete(s.RoomPasswords, oldRoom)
		s.RoomPasswordsMutex.Unlock()
	}
	client.Room = newRoom
	if s.Rooms[newRoom] == nil {
		s.Rooms[newRoom] = make(map[*models.Client]bool)
	}
	s.Rooms[newRoom][client] = true
	s.RoomsMutex.Unlock()

	if !isSwitchingFromGame || !isSwitchingToGame {
		s.BroadcastRoomList()
		go s.BroadcastOnlineCount()
	}

	return oldRoom, nil
}

// SendHistory
func (s *StateService) SendHistory(client *models.Client) {
	if strings.HasPrefix(client.Room, "_") {
		return
	}

	s.HistoryMutex.RLock()
	history, ok := s.History[client.Room]
	s.HistoryMutex.RUnlock()

	if ok && len(history) > 0 {
		for _, msg := range history {
			if !safeWriteJSON(client, msg) {
				break
			}
		}
	}
}

// GetLeaderboardJSON
func (s *StateService) GetLeaderboardJSON() ([]byte, error) {
	s.LeaderboardMutex.RLock()
	defer s.LeaderboardMutex.RUnlock()
	return json.Marshal(s.Leaderboard)
}

func (s *StateService) BroadcastToRoom(msg models.Message) {
	s.RoomsMutex.RLock() //
	clientsInRoom, ok := s.Rooms[msg.Room]
	s.RoomsMutex.RUnlock()

	if !ok {
		return
	}

	for client := range clientsInRoom {
		safeWriteJSON(client, msg)
	}
}

func (s *StateService) BroadcastToRoomExcept(msg models.Message, except *models.Client) {
	s.RoomsMutex.RLock()
	clientsInRoom, ok := s.Rooms[msg.Room]
	s.RoomsMutex.RUnlock()

	if !ok {
		return
	}

	for client := range clientsInRoom {
		if client == except {
			continue
		}
		safeWriteJSON(client, msg)
	}
}

func (s *StateService) BroadcastOnlineCount() {
	s.RoomsMutex.RLock()
	allClients := make(map[*models.Client]bool)
	for roomName, roomClients := range s.Rooms {
		if !strings.HasPrefix(roomName, "_") {
			for client := range roomClients {
				allClients[client] = true
			}
		}
	}
	count := len(allClients)
	s.RoomsMutex.RUnlock()

	msg := models.Message{
		Type:    "online_count",
		Content: fmt.Sprintf("%d", count),
	}

	for client := range allClients {
		safeWriteJSON(client, msg)
	}
}

func (s *StateService) BroadcastRoomList() {
	s.RoomsMutex.RLock()
	s.RoomPasswordsMutex.RLock()

	roomInfo := make(map[string]bool)

	for roomName := range s.Rooms {
		if !strings.HasPrefix(roomName, "_") {
			_, hasPassword := s.RoomPasswords[roomName]
			roomInfo[roomName] = hasPassword
		}
	}

	if _, exists := roomInfo["lobby"]; !exists {
		roomInfo["lobby"] = false
	}

	s.RoomPasswordsMutex.RUnlock()
	s.RoomsMutex.RUnlock()

	msg := models.Message{
		Type:     "room_list",
		RoomInfo: roomInfo,
	}

	var clientsToWrite []*models.Client
	s.RoomsMutex.RLock()
	for roomName, room := range s.Rooms {
		if !strings.HasPrefix(roomName, "_") {
			for client := range room {
				clientsToWrite = append(clientsToWrite, client)
			}
		}
	}
	s.RoomsMutex.RUnlock()

	for _, client := range clientsToWrite {
		safeWriteJSON(client, msg)
	}
}

func (s *StateService) loadLeaderboard() {
	s.LeaderboardMutex.Lock()
	defer s.LeaderboardMutex.Unlock()
	file, err := os.ReadFile(leaderboardFile)
	if err != nil {
		log.Println("No leaderboard file found, starting fresh.")
		return
	}
	err = json.Unmarshal(file, &s.Leaderboard)
	if err != nil {
		log.Println("Error parsing leaderboard file:", err)
		s.Leaderboard = make([]models.GameScore, 0)
	}
}

func (s *StateService) saveLeaderboard() {
	s.LeaderboardMutex.Lock()
	defer s.LeaderboardMutex.Unlock()
	file, err := json.MarshalIndent(s.Leaderboard, "", "  ")
	if err != nil {
		log.Println("Error marshalling leaderboard:", err)
		return
	}
	err = os.WriteFile(leaderboardFile, file, 0644)
	if err != nil {
		log.Println("Error saving leaderboard file:", err)
	}
}

func (s *StateService) broadcastLeaderboard() {
	scoresJSON, err := s.GetLeaderboardJSON() //
	if err != nil {
		log.Println("Error marshalling leaderboard broadcast:", err)
		return
	}
	msg := models.Message{Type: "leaderboard_update", Content: string(scoresJSON), Room: "_game_"} //

	//
	//
	s.BroadcastToRoom(msg)
}

func (s *StateService) updateLeaderboard(score models.GameScore) {
	s.LeaderboardMutex.Lock()
	s.Leaderboard = append(s.Leaderboard, score)
	sort.Slice(s.Leaderboard, func(i, j int) bool {
		if s.Leaderboard[i].Tries != s.Leaderboard[j].Tries {
			return s.Leaderboard[i].Tries < s.Leaderboard[j].Tries
		}
		return s.Leaderboard[i].Time < s.Leaderboard[j].Time
	})
	if len(s.Leaderboard) > 10 {
		s.Leaderboard = s.Leaderboard[:10]
	}
	s.LeaderboardMutex.Unlock()

	s.saveLeaderboard()
	s.broadcastLeaderboard()

	announceMsg := models.Message{
		Type: "chat", Room: "lobby", Nickname: "🏆 系統", Avatar: "🏆",
		Content:   fmt.Sprintf("%s 在猜數字遊戲中獲勝了 (猜 %d 次, %d 秒)！", score.Nickname, score.Tries, score.Time),
		Timestamp: time.Now().Format("15:04:05"),
	}
	s.Broadcast <- announceMsg
}
