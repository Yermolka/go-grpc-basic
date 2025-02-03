package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go-grpc-basic/proto/presence"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func newHub() *Hub {
	return &Hub{
		rooms:      make(map[string]*Room),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan BroadcastMessage),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			if room := client.currentRoom; room != nil {
				room.Members[client] = true
				// Notify room about new member
				joinMsg := map[string]interface{}{
					"type":    "system",
					"content": fmt.Sprintf("%s joined the room", client.username),
				}
				jsonMsg, _ := json.Marshal(joinMsg)
				h.broadcastToRoom(room.ID, jsonMsg)
			}

		case client := <-h.unregister:
			if room := client.currentRoom; room != nil {
				if _, ok := room.Members[client]; ok {
					delete(room.Members, client)
					close(client.send)
					// Notify room about member leaving
					leaveMsg := map[string]interface{}{
						"type":    "system",
						"content": fmt.Sprintf("%s left the room", client.username),
					}
					jsonMsg, _ := json.Marshal(leaveMsg)
					h.broadcastToRoom(room.ID, jsonMsg)
				}
				if len(room.Members) == 0 {
					h.mu.Lock()
					delete(h.rooms, room.ID)
					h.mu.Unlock()
				}
			}

		case msg := <-h.broadcast:
			h.broadcastToRoom(msg.RoomID, msg.Message)
		}
	}
}

func (h *Hub) broadcastToRoom(roomID string, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if room, exists := h.rooms[roomID]; exists {
		for client := range room.Members {
			select {
			case client.send <- message:
			default:
				close(client.send)
				delete(room.Members, client)
			}
		}
	}
}

func createRoomHandler(hub *Hub) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session-name")
		username := session.Values["username"].(string)

		var req RoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Validate input
		if req.Name == "" || req.MaxMembers < 2 {
			http.Error(w, "Invalid room parameters", http.StatusBadRequest)
			return
		}

		// Create room directly instead of through channel
		roomID := uuid.New().String()
		room := &Room{
			ID:         roomID,
			Name:       req.Name,
			Password:   req.Password,
			MaxMembers: req.MaxMembers,
			Members:    make(map[*Client]bool),
			CreatedBy:  username,
			CreatedAt:  time.Now(),
		}

		// Add to hub synchronously
		hub.mu.Lock()
		hub.rooms[roomID] = room
		hub.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":   roomID,
			"name": req.Name,
		})
	})
}

func listRoomsHandler(hub *Hub) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		hub.mu.RLock()
		defer hub.mu.RUnlock()

		rooms := make([]map[string]interface{}, 0, len(hub.rooms))
		for _, room := range hub.rooms {
			rooms = append(rooms, map[string]interface{}{
				"id":           room.ID,
				"name":         room.Name,
				"members":      len(room.Members),
				"max_members":  room.MaxMembers,
				"has_password": room.Password != "",
				"created_by":   room.CreatedBy,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rooms)
	})
}

func websocketHandler(hub *Hub, presenceClient presence.PresenceServiceClient) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session-name")
		username := session.Values["username"].(string)
		userID := session.Values["userID"].(string)

		conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
		if err != nil {
			log.Println("WebSocket upgrade failed:", err)
			return
		}

		log.Printf("User %s upgraded to WS", username)

		client := &Client{
			conn:     conn,
			send:     make(chan []byte, 256),
			username: username,
		}

		// Handle initial join request
		_, message, err := conn.ReadMessage()
		if err != nil {
			conn.Close()
			return
		}

		var joinReq JoinRequest
		if err := json.Unmarshal(message, &joinReq); err != nil {
			conn.WriteJSON(map[string]string{"error": "Invalid join request"})
			conn.Close()
			return
		}

		room, exists := hub.rooms[joinReq.RoomID]
		if !exists {
			conn.WriteJSON(map[string]string{"error": "Room not found"})
			conn.Close()
			return
		}

		if room.Password != "" && room.Password != joinReq.Password {
			conn.WriteJSON(map[string]string{"error": "Invalid password"})
			conn.Close()
			return
		}

		if len(room.Members) >= room.MaxMembers {
			conn.WriteJSON(map[string]string{"error": "Room is full"})
			conn.Close()
			return
		}

		client.currentRoom = room
		hub.register <- client

		go client.writePump()
		go client.readPump(hub)

		sessionID := uuid.New().String()
		_, err = presenceClient.UpdatePresence(r.Context(), &presence.UpdatePresenceRequest{
			UserId:    userID,
			Online:    true,
			SessionId: sessionID,
		})
		if err != nil {
			log.Printf("Error updating presence: %v", err)
		}

		client.conn.SetCloseHandler(func(code int, text string) error {
			_, err := presenceClient.UpdatePresence(context.Background(), &presence.UpdatePresenceRequest{
				UserId:    userID,
				Online:    false,
				SessionId: sessionID,
			})
			if err != nil {
				log.Printf("Error closing presence: %v", err)
			}
			return nil
		})
	})
}

func (c *Client) readPump(hub *Hub) {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msgData struct {
			Action  string `json:"action"`
			Content string `json:"content"`
		}

		if err := json.Unmarshal(message, &msgData); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}

		switch msgData.Action {
		case "join":
			// Handle join logic
		case "message":
			// Create structured message
			chatMsg := map[string]interface{}{
				"type":     "chat",
				"username": c.username,
				"message":  msgData.Content,
				"time":     time.Now().UnixMilli(),
			}

			jsonMsg, err := json.Marshal(chatMsg)
			if err != nil {
				log.Printf("Error marshaling message: %v", err)
				continue
			}

			hub.broadcast <- BroadcastMessage{
				RoomID:  c.currentRoom.ID,
				Message: jsonMsg,
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(15 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func chatHandler(hub *Hub) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "chat", PageData{
			Title: "Chat",
		})
	})
}
