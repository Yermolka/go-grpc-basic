package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"go-grpc-basic/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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

	initRoutes(hub, client, staticPath)

	log.Println("HTTP gateway running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
