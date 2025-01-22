package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-grpc-basic/proto"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

var templates *template.Template

func initTemplates() {
	templates = template.Must(template.New("").
		ParseGlob("../templates/*.html"))
}

func renderTemplate(w http.ResponseWriter, name string, data PageData) {
	content := templates.Lookup(name + "_content")
	if content == nil {
		log.Printf("Template %s_content not found", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Execute content template to string
	var contentBuf bytes.Buffer
	if err := content.Execute(&contentBuf, data); err != nil {
		log.Printf("Content template error: %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Create final data with rendered content
	finalData := PageData{
		Title:   data.Title,
		Content: template.HTML(contentBuf.String()),
	}

	// Execute base template
	err := templates.ExecuteTemplate(w, "base", finalData)
	if err != nil {
		log.Printf("Base template error: %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
	}
}

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

var store = sessions.NewCookieStore([]byte("super-secret-key"))

func loginHandler(client proto.AuthServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		password := r.FormValue("password")

		resp, err := client.AuthenticateUser(r.Context(), &proto.AuthenticateUserRequest{
			Username: username,
			Password: password,
		})
		if err != nil {
			http.Error(w, "Authentication failed", http.StatusInternalServerError)
			return
		}

		if !resp.Success {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		session, _ := store.Get(r, "session-name")
		session.Values["authenticated"] = true
		session.Values["username"] = username
		session.Save(r, w)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	session.Values["authenticated"] = false
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session-name")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session-name")
	renderTemplate(w, "dashboard", PageData{
		Title: "Dashboard",
		Data: struct{ Username string }{
			Username: session.Values["username"].(string),
		},
	})
}

func websocketHandler(hub *Hub) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "session-name")
		username := session.Values["username"].(string)

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

func loginPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login", PageData{
		Title: "Login Page",
	})
}

func chatHandler(hub *Hub) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "chat", PageData{
			Title: "Chat",
		})
	})
}

func main() {
	initTemplates()

	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := proto.NewAuthServiceClient(conn)

	hub := newHub()
	go hub.run()

	cwd, _ := os.Getwd()
	staticPath := filepath.Join(cwd, "../static")
	log.Println("Serving static files from:", staticPath)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath))))

	http.HandleFunc("/rooms", listRoomsHandler(hub))
	http.HandleFunc("/rooms/create", createRoomHandler(hub))
	http.HandleFunc("/ws", websocketHandler(hub))
	http.HandleFunc("/chat", chatHandler(hub))

	// Public routes
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			loginPage(w, r)
		} else {
			loginHandler(client).ServeHTTP(w, r)
		}
	})

	// Protected routes
	http.HandleFunc("/dashboard", authMiddleware(dashboardHandler))
	http.HandleFunc("/logout", logoutHandler)

	// Existing API endpoint
	http.HandleFunc("/authenticate", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || strings.ToLower(tokenParts[0]) != "bearer" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		resp, err := client.ValidateToken(r.Context(), &proto.ValidateTokenRequest{Token: tokenParts[1]})
		if err != nil {
			log.Printf("gRPC error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !resp.Valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authenticated!"))
	})

	log.Println("HTTP gateway running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
