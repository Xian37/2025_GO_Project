package models

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Client
type Client struct {
	Conn     *websocket.Conn
	Mu       sync.Mutex //
	Nickname string
	Room     string
	Avatar   string
}

// Message
type Message struct {
	Room       string         `json:"room"`
	Nickname   string         `json:"nickname"`
	Avatar     string         `json:"avatar"`
	Content    string         `json:"content,omitempty"`
	Type       string         `json:"type"`
	Question   string         `json:"question,omitempty"`
	Answer     string         `json:"answer,omitempty"`
	Timestamp  string         `json:"timestamp,omitempty"`
	Emoji      string         `json:"emoji,omitempty"`
	Options    []string       `json:"options,omitempty"`
	Results    map[string]int `json:"results,omitempty"`
	Transcript string         `json:"transcript,omitempty"`
	Tries      int            `json:"tries,omitempty"`
	Time       int            `json:"time,omitempty"`
	X          float64        `json:"x,omitempty"`
	Y          float64        `json:"y,omitempty"`
	Color      string         `json:"color,omitempty"`
	LineWidth  int            `json:"lineWidth,omitempty"`
	Password   string         `json:"password,omitempty"`
}

// Quiz
type Quiz struct {
	Question string
	Answer   string
	Active   bool
}

// Vote
type Vote struct {
	Question string
	Options  map[string]int
	Voters   map[string]bool
}

// GameScore
type GameScore struct {
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	Tries    int    `json:"tries"`
	Time     int    `json:"time"`
}

// DrawState
type DrawState struct {
	CurrentWord   string
	CurrentDrawer string
}
