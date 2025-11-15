package transport

import (
	"chatroom/models"  //
	"chatroom/service" //
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const maxMessageSize = 5 * 1024 * 1024

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WebsocketHandler
type WebsocketHandler struct {
	Service *service.StateService
}

// NewWebsocketHandler
func NewWebsocketHandler(s *service.StateService) *WebsocketHandler {
	return &WebsocketHandler{
		Service: s,
	}
}

// HandleConnections
func (h *WebsocketHandler) HandleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer ws.Close()

	ws.SetReadLimit(maxMessageSize)

	var initMsg models.Message
	err = ws.ReadJSON(&initMsg)
	if err != nil {
		log.Println("Init message read error:", err)
		return
	}

	client := &models.Client{
		Conn:     ws,
		Nickname: initMsg.Nickname,
		Room:     initMsg.Room,
		Avatar:   initMsg.Avatar,
	}

	h.Service.RegisterClient(client)

	if !strings.HasPrefix(client.Room, "_") {
		h.Service.SendHistory(client)

		joinMsg := models.Message{
			Type: "join", Room: client.Room, Content: client.Nickname + " 加入了聊天室", Timestamp: time.Now().Format("15:04"),
		}
		h.Service.Broadcast <- joinMsg
		go h.Service.BroadcastOnlineCount()
	}

	h.readLoop(client)
}

// readLoop
func (h *WebsocketHandler) readLoop(client *models.Client) {
	for {
		var msg models.Message
		err := client.Conn.ReadJSON(&msg)
		if err != nil {
			h.Service.UnregisterClient(client)
			break
		}

		msg.Avatar = client.Avatar
		msg.Nickname = client.Nickname

		if msg.Type != "switch" {
			msg.Room = client.Room
		}

		if msg.Type == "get_leaderboard" {
			scoresJSON, err := h.Service.GetLeaderboardJSON()
			if err != nil {
				log.Println("Error marshalling leaderboard:", err)
				continue
			}
			resp := models.Message{Type: "leaderboard_update", Content: string(scoresJSON)}
			client.Mu.Lock()
			if err := client.Conn.WriteJSON(resp); err != nil {
				log.Println("WriteJSON error (get_leaderboard):", err)
			}
			client.Mu.Unlock()
			continue
		}

		if msg.Type == "switch" {
			oldRoom, err := h.Service.SwitchRoom(client, msg.Room, msg.Password)
			if err != nil {
				errorMsg := models.Message{Type: err.Error(), Room: msg.Room}
				client.Mu.Lock()
				if err := client.Conn.WriteJSON(errorMsg); err != nil {
					log.Println("WriteJSON error (password error):", err)
				}
				client.Mu.Unlock()
				continue
			}

			client.Room = msg.Room

			if !strings.HasPrefix(msg.Room, "_") {
				h.Service.SendHistory(client)
				joinMsg := models.Message{
					Type: "join", Room: msg.Room, Content: client.Nickname + " 加入了聊天室", Timestamp: time.Now().Format("15:04"),
				}
				h.Service.Broadcast <- joinMsg
			}

			confirmMsg := models.Message{Type: "switch_success", Room: msg.Room, Content: oldRoom}
			client.Mu.Lock()
			if err := client.Conn.WriteJSON(confirmMsg); err != nil {
				log.Println("WriteJSON error (switch_success):", err)
			}
			client.Mu.Unlock()
			continue
		}

		if msg.Type == "chat" && msg.Room == "_draw_game_" && strings.HasPrefix(msg.Content, "/setword ") {

			word := strings.TrimSpace(strings.TrimPrefix(msg.Content, "/setword "))

			// 1.
			var broadcastMsg *models.Message

			h.Service.DrawStateMutex.Lock() // <--
			if h.Service.DrawStates[msg.Room] == nil {
				h.Service.DrawStates[msg.Room] = &models.DrawState{}
			}
			state := h.Service.DrawStates[msg.Room]

			if word != "" && (state.CurrentDrawer == "" || state.CurrentDrawer == client.Nickname) {
				state.CurrentWord = word
				state.CurrentDrawer = client.Nickname

				//
				client.Mu.Lock()
				if err := client.Conn.WriteJSON(models.Message{Type: "new_round_drawer", Content: word, Room: msg.Room}); err != nil {
					log.Println("WriteJSON error (draw drawer):", err)
				}
				client.Mu.Unlock()

				//
				blanks := strings.Repeat("_ ", len([]rune(word)))
				broadcastMsg = &models.Message{ //
					Type: "new_round_guesser", Room: msg.Room, Content: blanks, Nickname: client.Nickname,
				}
			}
			h.Service.DrawStateMutex.Unlock() // <--

			// 2.
			if broadcastMsg != nil {
				h.Service.BroadcastToRoomExcept(*broadcastMsg, client) // <--
			}
			continue
		}

		if msg.Type == "game_score" {
			msg.Nickname = client.Nickname
			msg.Avatar = client.Avatar
		}

		h.Service.Broadcast <- msg
	}
}
