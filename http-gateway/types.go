package main

import (
	"html/template"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Room struct {
	ID         string
	Name       string
	Password   string
	MaxMembers int
	Members    map[*Client]bool
	CreatedBy  string
	CreatedAt  time.Time
}

type Client struct {
	conn        *websocket.Conn
	send        chan []byte
	username    string
	currentRoom *Room
}

type ChatMessage struct {
	Username string `json:"username"`
	Message  string `json:"message"`
	Time     int64  `json:"time"`
}

type BroadcastMessage struct {
	RoomID  string
	Message []byte
}

type Hub struct {
	rooms      map[string]*Room
	mu         sync.RWMutex // Add mutex for concurrent access
	register   chan *Client
	unregister chan *Client
	broadcast  chan BroadcastMessage
}

type RoomRequest struct {
	Name       string `json:"name"`
	Password   string `json:"password"`
	MaxMembers int    `json:"max_members,string"`
}

type JoinRequest struct {
	RoomID   string `json:"room_id"`
	Password string `json:"password"`
}

type PageData struct {
	Title    string        // Page title for <title> tag
	Content  template.HTML // HTML content for the main body
	Data     interface{}   // Additional page-specific data
	Username string        // Optional: logged-in username
	// Add other common fields here
}
