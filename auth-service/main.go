package main

import (
	"context"
	"go-grpc-basic/proto"
	"log"
	"net"

	"google.golang.org/grpc"
)

type server struct {
	proto.UnimplementedAuthServiceServer
}

func (s *server) AuthenticateUser(ctx context.Context, req *proto.AuthenticateUserRequest) (*proto.AuthenticateUserResponse, error) {
	// Simple hardcoded authentication for example
	valid := req.Username == "admin" && req.Password == "password"
	return &proto.AuthenticateUserResponse{Success: valid}, nil
}

func (s *server) ValidateToken(ctx context.Context, req *proto.ValidateTokenRequest) (*proto.ValidateTokenResponse, error) {
	return &proto.ValidateTokenResponse{Valid: req.Token == "valid-token"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	proto.RegisterAuthServiceServer(s, &server{})
	log.Println("Auth service running on :50051")
	log.Fatal(s.Serve(lis))
}
