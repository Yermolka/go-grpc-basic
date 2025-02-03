package main

import (
	"encoding/json"
	"go-grpc-basic/proto"
	"go-grpc-basic/proto/presence"
	"log"
	"net/http"
	"strings"
)

func initRoutes(hub *Hub, authClient proto.AuthServiceClient, presenceClient presence.PresenceServiceClient, staticPath string) {
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticPath))))

	http.HandleFunc("/rooms", listRoomsHandler(hub))
	http.HandleFunc("/rooms/create", createRoomHandler(hub))
	http.HandleFunc("/ws", websocketHandler(hub, presenceClient))
	http.HandleFunc("/chat", chatHandler(hub))

	// Public routes
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			loginPage(w, r)
		} else {
			loginHandler(authClient).ServeHTTP(w, r)
		}
	})

	// Protected routes
	http.HandleFunc("/dashboard", authMiddleware(dashboardHandler))
	http.HandleFunc("/logout", logoutHandler)

	// Existing API endpoint
	http.HandleFunc("/api/authenticate", func(w http.ResponseWriter, r *http.Request) {
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

		resp, err := authClient.ValidateToken(r.Context(), &proto.ValidateTokenRequest{Token: tokenParts[1]})
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

	http.HandleFunc("/api/presence", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		userIDs := r.URL.Query()["userIDs"]
		resp, err := presenceClient.GetPresence(r.Context(), &presence.GetPresenceRequest{UserIds: userIDs})
		if err != nil {
			log.Printf("gRPC error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		result := make(map[string]bool, len(resp.Presences))
		for _, pres := range resp.GetPresences() {
			result[pres.UserId] = pres.Online
		}
		json, err := json.Marshal(result)
		if err != nil {
			log.Printf("JSON error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(json)
	}))
}
