package main

import (
	"go-grpc-basic/proto"
	"log"
	"net/http"
	"strings"

	"google.golang.org/grpc"
)

func authHandler(client proto.AuthServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := proto.NewAuthServiceClient(conn)
	http.HandleFunc("/login", authHandler(client))
	log.Println("HTTP gateway running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
