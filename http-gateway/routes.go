package main

import (
	"go-grpc-basic/proto"
	"log"
	"net/http"
	"strings"
)

func initRoutes(hub *Hub, client proto.AuthServiceClient, staticPath string) {
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
}
