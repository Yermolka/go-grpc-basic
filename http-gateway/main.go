package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go-grpc-basic/proto"
	"go-grpc-basic/proto/presence"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	initTemplates()
	auth_host, found := os.LookupEnv("AUTH_HOST")
	if !found {
		auth_host = "localhost"
	}

	auth_port, found := os.LookupEnv("AUTH_PORT")
	if !found {
		auth_port = "50051"
	}

	presence_host, found := os.LookupEnv("PRESENCE_HOST")
	if !found {
		presence_host = "localhost"
	}

	presence_port, found := os.LookupEnv("PRESENCE_PORT")
	if !found {
		presence_port = "50052"
	}

	auth_conn, err := grpc.Dial(strings.Join([]string{auth_host, auth_port}, ":"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer auth_conn.Close()

	presence_conn, err := grpc.Dial(strings.Join([]string{presence_host, presence_port}, ":"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer presence_conn.Close()

	auth_client := proto.NewAuthServiceClient(auth_conn)
	presence_client := presence.NewPresenceServiceClient(presence_conn)

	hub := newHub()
	go hub.run()

	cwd, _ := os.Getwd()
	staticPath := filepath.Join(cwd, "static")
	log.Println("Serving static files from:", staticPath)

	initRoutes(hub, auth_client, presence_client, staticPath)

	log.Println("HTTP gateway running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
