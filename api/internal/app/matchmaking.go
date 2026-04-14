package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

const podRegistryKey = "pod:registry"
const registryStateReady = "ready"

type lobbyAssignment struct {
	LobbyID      string
	PodIP        string
	Port         string
	TickRate     int
	SnapshotRate int
	MaxPlayers   int
}

type registryRecord struct {
	IP      string `json:"ip"`
	Port    string `json:"port"`
	State   string `json:"state"`
	Lobbies int    `json:"lobbies"`
}

func (s *Server) selectLobbyAssignment(ctx context.Context) (lobbyAssignment, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		lobbies, err := s.loadLobbyAssignments(ctx)
		if err != nil {
			lastErr = err
		} else if len(lobbies) == 0 {
			lastErr = fmt.Errorf("no lobby assignments")
		} else {
			readyPods, err := s.loadReadyPods(ctx)
			if err != nil {
				lastErr = err
			} else {
				candidates := make([]lobbyAssignment, 0, len(lobbies))
				for _, assignment := range lobbies {
					record, ok := readyPods[assignment.PodIP]
					if !ok || record.State != registryStateReady {
						continue
					}
					if assignment.Port == "" {
						assignment.Port = record.Port
					}
					candidates = append(candidates, assignment)
				}

				if len(candidates) > 0 {
					rng := rand.New(rand.NewSource(time.Now().UnixNano()))
					return candidates[rng.Intn(len(candidates))], nil
				}
				lastErr = fmt.Errorf("no ready pods")
			}
		}

		if attempt < 4 {
			select {
			case <-ctx.Done():
				return lobbyAssignment{}, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("game servers starting up")
	}
	return lobbyAssignment{}, lastErr
}

func (s *Server) loadReadyPods(ctx context.Context) (map[string]registryRecord, error) {
	values, err := s.redis.HGetAll(ctx, podRegistryKey).Result()
	if err != nil {
		return nil, err
	}

	records := make(map[string]registryRecord, len(values))
	for ip, payload := range values {
		var record registryRecord
		if err := json.Unmarshal([]byte(payload), &record); err != nil {
			s.logger.Printf("invalid pod registry payload for %s: %v", ip, err)
			continue
		}
		if record.IP == "" {
			record.IP = ip
		}
		records[ip] = record
	}
	return records, nil
}

func (s *Server) loadLobbyAssignments(ctx context.Context) ([]lobbyAssignment, error) {
	assignments := make([]lobbyAssignment, 0)
	err := scanKeys(ctx, s.redis, "lobby:*", func(key string) error {
		values, err := s.redis.HGetAll(ctx, key).Result()
		if err != nil || len(values) == 0 {
			return nil
		}

		lobbyID := strings.TrimPrefix(key, "lobby:")
		assignments = append(assignments, lobbyAssignment{
			LobbyID:      lobbyID,
			PodIP:        values["pod_ip"],
			Port:         values["port"],
			TickRate:     envIntValue(values["tick_rate"], 60),
			SnapshotRate: envIntValue(values["snapshot_rate"], 20),
			MaxPlayers:   envIntValue(values["max_players"], 10),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return assignments, nil
}

func buildWSURL(cfg Config, lobbyID, token string) string {
	if cfg.WSRouterBaseURL != "" {
		return strings.TrimRight(cfg.WSRouterBaseURL, "/") + "/" + token
	}

	base := strings.TrimRight(cfg.PublicGameWSURL, "?")
	return fmt.Sprintf("%s?lobby=%s&token=%s", base, lobbyID, token)
}

func startCleanupTicker(ctx context.Context, interval time.Duration, fn func(context.Context) error, logger func(error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := fn(ctx); err != nil {
				logger(err)
			}
		}
	}
}
