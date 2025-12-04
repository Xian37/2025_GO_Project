package main

import (
	"chatroom/models"
	"chatroom/service"
	"chatroom/transport"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDrawAndGuessFlow(t *testing.T) {
	// 1. Setup Server
	broadcastChan := make(chan models.Message)
	stateService := service.NewStateService(broadcastChan)
	go stateService.HandleMessageLoop()

	wsHandler := transport.NewWebsocketHandler(stateService)
	ts := httptest.NewServer(http.HandlerFunc(wsHandler.HandleConnections))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	// 2. Connect Clients
	drawerWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Drawer connection failed: %v", err)
	}
	defer drawerWS.Close()

	guesserWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Guesser connection failed: %v", err)
	}
	defer guesserWS.Close()

	// 3. Login / Join Room
	// Drawer
	drawerInit := models.Message{
		Type: "switch", Room: "_draw_game_", Nickname: "Drawer", Avatar: "D",
	}
	if err := drawerWS.WriteJSON(drawerInit); err != nil {
		t.Fatalf("Drawer init failed: %v", err)
	}

	// Guesser
	guesserInit := models.Message{
		Type: "switch", Room: "_draw_game_", Nickname: "Guesser", Avatar: "G",
	}
	if err := guesserWS.WriteJSON(guesserInit); err != nil {
		t.Fatalf("Guesser init failed: %v", err)
	}

	// 4. Test /setword command
	setWordMsg := models.Message{
		Type: "chat", Room: "_draw_game_", Content: "/setword apple", Nickname: "Drawer",
	}
	if err := drawerWS.WriteJSON(setWordMsg); err != nil {
		t.Fatalf("Failed to send /setword: %v", err)
	}

	// Verify Drawer receives "new_round_drawer"
	expectMessage(t, drawerWS, "new_round_drawer", "apple")

	// Verify Guesser receives "new_round_guesser" with masked word
	expectMessage(t, guesserWS, "new_round_guesser", "*****")

	// 5. Test Wrong Guess
	wrongGuess := models.Message{
		Type: "chat", Room: "_draw_game_", Content: "pear", Nickname: "Guesser",
	}
	if err := guesserWS.WriteJSON(wrongGuess); err != nil {
		t.Fatalf("Failed to send wrong guess: %v", err)
	}

	// Both should receive the chat message
	expectMessage(t, drawerWS, "chat", "pear")
	expectMessage(t, guesserWS, "chat", "pear")

	// 6. Test Correct Guess
	correctGuess := models.Message{
		Type: "chat", Room: "_draw_game_", Content: "apple", Nickname: "Guesser",
	}
	if err := guesserWS.WriteJSON(correctGuess); err != nil {
		t.Fatalf("Failed to send correct guess: %v", err)
	}

	// Both should receive "guess_correct"
	expectMessage(t, drawerWS, "guess_correct", "apple")
	expectMessage(t, guesserWS, "guess_correct", "apple")
}

// Helper to read messages until matching type is found or timeout
func expectMessage(t *testing.T, ws *websocket.Conn, expectedType, expectedContentFragment string) {
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for message type: %s", expectedType)
		default:
			var msg models.Message
			if err := ws.ReadJSON(&msg); err != nil {
				t.Fatalf("Read error: %v", err)
			}
			// Skip unrelated messages (like online_count, join/leave from other rooms, or initial switch success)
			if msg.Type == expectedType {
				if expectedContentFragment != "" && !strings.Contains(msg.Content, expectedContentFragment) {
					t.Fatalf("Expected content to contain '%s', got '%s'", expectedContentFragment, msg.Content)
				}
				return // Success
			}
		}
	}
}
