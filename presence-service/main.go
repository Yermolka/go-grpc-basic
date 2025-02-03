package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"go-grpc-basic/proto/presence"
)

type presenceServer struct {
	presence.UnimplementedPresenceServiceServer
	mu       sync.RWMutex
	presence map[string]*presence.UserPresence // user_id -> presence
	sessions map[string]string                 // session_id -> user_id
}

func (s *presenceServer) UpdatePresence(ctx context.Context, req *presence.UpdatePresenceRequest) (*presence.UpdatePresenceResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	if existing, exists := s.presence[req.UserId]; exists {
		existing.ActiveConnections++
		if req.Online {
			existing.LastActive = now
			existing.Online = true
		}
	} else {
		s.presence[req.UserId] = &presence.UserPresence{
			UserId:            req.UserId,
			Online:            req.Online,
			LastActive:        now,
			ActiveConnections: 1,
		}
	}

	s.sessions[req.SessionId] = req.UserId
	return &presence.UpdatePresenceResponse{Success: true}, nil
}

func (s *presenceServer) GetPresence(ctx context.Context, req *presence.GetPresenceRequest) (*presence.GetPresenceResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*presence.UserPresence, 0, len(req.UserIds))
	for _, uid := range req.UserIds {
		if p, exists := s.presence[uid]; exists {
			result = append(result, p)
		}
	}
	return &presence.GetPresenceResponse{Presences: result}, nil
}

func (s *presenceServer) StreamPresence(req *presence.StreamPresenceRequest, stream presence.PresenceService_StreamPresenceServer) error {
	// Implement presence streaming logic
	// (This would typically use a pub/sub system in production)
	return nil
}

func main() {
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	presence.RegisterPresenceServiceServer(s, &presenceServer{
		presence: make(map[string]*presence.UserPresence),
		sessions: make(map[string]string),
	})

	log.Println("Presence service running on :50052")
	log.Fatal(s.Serve(lis))
}
