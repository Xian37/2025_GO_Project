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
	Level    int    `json:"level"`
	Exp      int    `json:"exp"`
	Title    string `json:"title"`
}

// ReplyTo 引用訊息結構
type ReplyTo struct {
	Timestamp string `json:"timestamp"`
	Nickname  string `json:"nickname"`
	Content   string `json:"content"`
}

// Message
type Message struct {
	Room       string          `json:"room"`
	Nickname   string          `json:"nickname"`
	Avatar     string          `json:"avatar"`
	UserId     string          `json:"userId,omitempty"`
	Content    string          `json:"content,omitempty"`
	Type       string          `json:"type"`
	Question   string          `json:"question,omitempty"`
	Answer     string          `json:"answer,omitempty"`
	Timestamp  string          `json:"timestamp,omitempty"`
	Emoji      string          `json:"emoji,omitempty"`
	Options    []string        `json:"options,omitempty"`
	Results    map[string]int  `json:"results,omitempty"`
	Transcript string          `json:"transcript,omitempty"`
	Tries      int             `json:"tries,omitempty"`
	Time       int             `json:"time,omitempty"`
	X          float64         `json:"x,omitempty"`
	Y          float64         `json:"y,omitempty"`
	Color      string          `json:"color,omitempty"`
	LineWidth  int             `json:"lineWidth,omitempty"`
	Password   string          `json:"password,omitempty"`
	RoomInfo   map[string]bool `json:"roomInfo,omitempty"`
	ReplyTo    *ReplyTo        `json:"replyTo,omitempty"` // 引用訊息
	Level      int             `json:"level,omitempty"`
	Exp        int             `json:"exp,omitempty"`
	Title      string          `json:"title,omitempty"`
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

// UserProfile 用戶資料
type UserProfile struct {
	Nickname string   `json:"nickname"`
	Avatar   string   `json:"avatar"`
	Level    int      `json:"level"`
	Exp      int      `json:"exp"`
	Title    string   `json:"title"`
	Badges   []string `json:"badges"`
	TotalMsg int      `json:"totalMsg"`
	JoinDate string   `json:"joinDate"`
	LastSeen string   `json:"lastSeen"`
}

// Achievement 成就
type Achievement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Unlocked    bool   `json:"unlocked"`
	Progress    int    `json:"progress"`
	Target      int    `json:"target"`
}
