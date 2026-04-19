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

const (
	registryStateReady    = "ready"
	registryStateFull     = "full"
	registryStateDraining = "draining"
)

type assignmentRequest struct {
	Mode           sessionMode
	LobbyID        string
	DebugSessionID string
	DebugStart     bool
}

type lobbyAssignment struct {
	LobbyID         string
	PodIP           string
	Port            string
	TickRate        int
	SnapshotRate    int
	MaxPlayers      int
	Phase           string
	MatchOver       bool
	ConnectedHumans int
	SpectatorCount  int
	MaxSpectators   int
	MatchKind       string
	DebugSessionID  string
}

type registryRecord struct {
	IP      string `json:"ip"`
	Port    string `json:"port"`
	State   string `json:"state"`
	Lobbies int    `json:"lobbies"`
}

func (s *Server) selectLobbyAssignment(ctx context.Context, request assignmentRequest) (lobbyAssignment, error) {
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
				if assignment, ok := s.pickLobbyAssignment(lobbies, readyPods, request); ok {
					return assignment, nil
				}
				lastErr = fmt.Errorf("no matching lobby assignments")
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

func (s *Server) pickLobbyAssignment(
	lobbies []lobbyAssignment,
	readyPods map[string]registryRecord,
	request assignmentRequest,
) (lobbyAssignment, bool) {
	switch request.Mode {
	case sessionModePlayer:
		return pickRandomAssignment(lobbies, readyPods, func(assignment lobbyAssignment, record registryRecord) bool {
			return record.State == registryStateReady && assignment.MatchKind != matchKindDebugBotSim
		})
	case sessionModeSpectator:
		if request.LobbyID != "" {
			if assignment, ok := pickTargetedAssignment(lobbies, readyPods, request.LobbyID, func(assignment lobbyAssignment, record registryRecord) bool {
				return record.State != registryStateDraining &&
					assignment.MatchKind != matchKindDebugBotSim &&
					canAdmitSpectators(assignment)
			}); ok {
				return assignment, true
			}
		}
		return pickBestRankedAssignment(lobbies, readyPods, func(assignment lobbyAssignment, record registryRecord) (int, bool) {
			if record.State == registryStateDraining || assignment.MatchKind == matchKindDebugBotSim || !canAdmitSpectators(assignment) {
				return 0, false
			}
			switch strings.ToLower(strings.TrimSpace(assignment.Phase)) {
			case "active", "intermission":
				return 2, true
			case "idle":
				return 1, true
			default:
				return 0, false
			}
		})
	case sessionModeDebugSimulation:
		if request.DebugStart {
			return pickRandomAssignment(lobbies, readyPods, func(assignment lobbyAssignment, record registryRecord) bool {
				return record.State != registryStateDraining &&
					strings.EqualFold(assignment.Phase, "idle") &&
					assignment.MatchKind != matchKindDebugBotSim &&
					canAdmitSpectators(assignment)
			})
		}
		if request.LobbyID == "" || request.DebugSessionID == "" {
			return lobbyAssignment{}, false
		}
		return pickTargetedAssignment(lobbies, readyPods, request.LobbyID, func(assignment lobbyAssignment, record registryRecord) bool {
			return record.State != registryStateDraining &&
				assignment.MatchKind == matchKindDebugBotSim &&
				assignment.DebugSessionID == request.DebugSessionID
		})
	default:
		return lobbyAssignment{}, false
	}
}

func pickRandomAssignment(
	lobbies []lobbyAssignment,
	readyPods map[string]registryRecord,
	allow func(lobbyAssignment, registryRecord) bool,
) (lobbyAssignment, bool) {
	candidates := make([]lobbyAssignment, 0, len(lobbies))
	for _, assignment := range lobbies {
		record, ok := readyPods[assignment.PodIP]
		if !ok || !allow(assignment, record) {
			continue
		}
		candidates = append(candidates, finalizeAssignment(assignment, record))
	}
	if len(candidates) == 0 {
		return lobbyAssignment{}, false
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return candidates[rng.Intn(len(candidates))], true
}

func pickBestRankedAssignment(
	lobbies []lobbyAssignment,
	readyPods map[string]registryRecord,
	rank func(lobbyAssignment, registryRecord) (int, bool),
) (lobbyAssignment, bool) {
	bestRank := -1
	candidates := make([]lobbyAssignment, 0, len(lobbies))
	for _, assignment := range lobbies {
		record, ok := readyPods[assignment.PodIP]
		if !ok {
			continue
		}
		currentRank, allowed := rank(assignment, record)
		if !allowed {
			continue
		}
		finalized := finalizeAssignment(assignment, record)
		if currentRank > bestRank {
			bestRank = currentRank
			candidates = []lobbyAssignment{finalized}
			continue
		}
		if currentRank == bestRank {
			candidates = append(candidates, finalized)
		}
	}
	if len(candidates) == 0 {
		return lobbyAssignment{}, false
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return candidates[rng.Intn(len(candidates))], true
}

func pickTargetedAssignment(
	lobbies []lobbyAssignment,
	readyPods map[string]registryRecord,
	lobbyID string,
	allow func(lobbyAssignment, registryRecord) bool,
) (lobbyAssignment, bool) {
	for _, assignment := range lobbies {
		if assignment.LobbyID != lobbyID {
			continue
		}
		record, ok := readyPods[assignment.PodIP]
		if !ok || !allow(assignment, record) {
			return lobbyAssignment{}, false
		}
		return finalizeAssignment(assignment, record), true
	}
	return lobbyAssignment{}, false
}

func finalizeAssignment(assignment lobbyAssignment, record registryRecord) lobbyAssignment {
	if assignment.Port == "" {
		assignment.Port = record.Port
	}
	return assignment
}

func canAdmitSpectators(assignment lobbyAssignment) bool {
	return assignment.MaxSpectators > 0 && assignment.SpectatorCount < assignment.MaxSpectators
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
			LobbyID:         lobbyID,
			PodIP:           values["pod_ip"],
			Port:            values["port"],
			TickRate:        envIntValue(values["tick_rate"], 60),
			SnapshotRate:    envIntValue(values["snapshot_rate"], 20),
			MaxPlayers:      envIntValue(values["max_players"], 10),
			Phase:           values["phase"],
			MatchOver:       strings.EqualFold(values["match_over"], "true"),
			ConnectedHumans: envIntValue(values["connected_humans"], 0),
			SpectatorCount:  envIntValue(values["spectator_count"], 0),
			MaxSpectators:   envIntValue(values["max_spectators"], 0),
			MatchKind:       values["match_kind"],
			DebugSessionID:  values["debug_session_id"],
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
