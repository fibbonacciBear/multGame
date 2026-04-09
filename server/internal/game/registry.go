package game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type registryRecord struct {
	IP      string `json:"ip"`
	Port    string `json:"port"`
	State   string `json:"state"`
	Lobbies int    `json:"lobbies"`
}

func (s *Server) initRegistryClient() {
	s.redis = redis.NewClient(&redis.Options{
		Addr:     s.cfg.RedisAddr,
		Password: s.cfg.RedisPassword,
		DB:       s.cfg.RedisDB,
	})
}

func (s *Server) startRegistryLoop(ctx context.Context) {
	if s.redis == nil || s.cfg.PodIP == "" {
		return
	}

	if err := s.refreshRegistry(ctx); err != nil {
		s.logger.Printf("registry refresh failed: %v", err)
	}

	ticker := time.NewTicker(s.cfg.RegistryHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refreshRegistry(ctx); err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Printf("registry heartbeat failed: %v", err)
			}
		}
	}
}

func (s *Server) refreshRegistry(ctx context.Context) error {
	if s.redis == nil || s.cfg.PodIP == "" {
		return nil
	}

	record, err := s.currentRegistryRecord()
	if err != nil {
		return err
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}

	pipe := s.redis.TxPipeline()
	pipe.HSet(ctx, podRegistryKey, s.cfg.PodIP, payload)
	pipe.Set(ctx, heartbeatKey(s.cfg.PodIP), "1", s.cfg.RegistryHeartbeatTTL)
	pipe.HSet(ctx, lobbyKey(s.cfg.LobbyID), "pod_ip", s.cfg.PodIP, "port", s.cfg.Port)
	pipe.Expire(ctx, lobbyKey(s.cfg.LobbyID), s.cfg.LobbyTTL)
	_, err = pipe.Exec(ctx)
	return err
}

func (s *Server) markDraining(ctx context.Context) error {
	if s.redis == nil || s.cfg.PodIP == "" {
		return nil
	}

	record, err := s.currentRegistryRecord()
	if err != nil {
		return err
	}
	record.State = registryStateDraining

	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}

	return s.redis.HSet(ctx, podRegistryKey, s.cfg.PodIP, payload).Err()
}

func (s *Server) cleanupRegistry(ctx context.Context) error {
	if s.redis == nil || s.cfg.PodIP == "" {
		return nil
	}

	pipe := s.redis.TxPipeline()
	pipe.HDel(ctx, podRegistryKey, s.cfg.PodIP)
	pipe.Del(ctx, heartbeatKey(s.cfg.PodIP))
	pipe.Del(ctx, lobbyKey(s.cfg.LobbyID))
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Server) currentRegistryRecord() (registryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := registryStateReady
	if s.draining {
		state = registryStateDraining
	} else if s.activeSlotsLocked() >= s.cfg.MaxPlayers {
		state = registryStateFull
	}

	return registryRecord{
		IP:      s.cfg.PodIP,
		Port:    s.cfg.Port,
		State:   state,
		Lobbies: 1,
	}, nil
}

func (s *Server) tickHealth(now time.Time) {
	s.lastTickAt.Store(now.UnixNano())
}

func (s *Server) tickAge(now time.Time) time.Duration {
	last := s.lastTickAt.Load()
	if last == 0 {
		return 0
	}
	return now.Sub(time.Unix(0, last))
}

func heartbeatKey(ip string) string {
	return fmt.Sprintf("pod:heartbeat:%s", ip)
}

func lobbyKey(lobbyID string) string {
	return fmt.Sprintf("lobby:%s", lobbyID)
}

const (
	podRegistryKey        = "pod:registry"
	registryStateReady    = "ready"
	registryStateFull     = "full"
	registryStateDraining = "draining"
)

func (s *Server) CleanupRegistry(ctx context.Context) error {
	return s.cleanupRegistry(ctx)
}
