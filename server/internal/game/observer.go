package game

import (
	mathrand "math/rand"
	"sort"
	"time"
)

const (
	defaultMaxSpectators       = 8
	defaultDebugSpectatorGrace = 5 * time.Second
)

type SessionMode string

const (
	sessionModePlayer          SessionMode = "player"
	sessionModeSpectator       SessionMode = "spectator"
	sessionModeDebugSimulation SessionMode = "debug_simulation"
)

type MatchKind string

const (
	matchKindNormal      MatchKind = "normal"
	matchKindDebugBotSim MatchKind = "debug_bot_sim"
)

func normalizeSessionMode(mode SessionMode) SessionMode {
	switch mode {
	case sessionModeSpectator, sessionModeDebugSimulation:
		return mode
	default:
		return sessionModePlayer
	}
}

func normalizeMatchKind(kind MatchKind) MatchKind {
	if kind == matchKindDebugBotSim {
		return kind
	}
	return matchKindNormal
}

func (s *Server) rngLocked() *mathrand.Rand {
	if s.lobby != nil && s.lobby.MatchKind == matchKindDebugBotSim && s.matchRNG != nil {
		return s.matchRNG
	}
	return s.rng
}

func (s *Server) randomIntnLocked(limit int) int {
	if limit <= 0 {
		return 0
	}
	return s.rngLocked().Intn(limit)
}

func (s *Server) randomInt31Locked() int32 {
	return s.rngLocked().Int31()
}

func (s *Server) connectedGameplayHumansLocked() int {
	humans := 0
	for _, player := range s.lobby.Players {
		if !player.IsBot && player.Connected {
			humans++
		}
	}
	return humans
}

func (s *Server) connectedSpectatorsLocked() int {
	return len(s.spectators)
}

func (s *Server) connectedDebugSpectatorsLocked() int {
	if s.lobby.MatchKind != matchKindDebugBotSim || s.lobby.DebugSessionID == "" {
		return 0
	}
	count := 0
	for _, connection := range s.spectators {
		if connection.SessionMode == sessionModeDebugSimulation && connection.DebugSessionID == s.lobby.DebugSessionID {
			count++
		}
	}
	return count
}

func (s *Server) connectedObserversForIdleLocked(now time.Time) int {
	if count := s.connectedSpectatorsLocked(); count > 0 {
		return count
	}
	if s.lobby.MatchKind == matchKindDebugBotSim &&
		!s.lobby.DebugSpectatorGraceUntil.IsZero() &&
		now.Before(s.lobby.DebugSpectatorGraceUntil) {
		return 1
	}
	return 0
}

func (s *Server) allConnectionsForNoticeCloseLocked() []*ClientConnection {
	connections := make([]*ClientConnection, 0, len(s.lobby.Players)+len(s.spectators))
	for _, player := range s.lobby.Players {
		if player.Connection != nil && player.Connected {
			connections = append(connections, player.Connection)
		}
	}
	for _, connection := range s.spectators {
		connections = append(connections, connection)
	}
	return connections
}

func (s *Server) canAdmitSpectatorLocked(viewerID string) bool {
	if existing, ok := s.spectators[viewerID]; ok && existing != nil {
		return true
	}
	return len(s.spectators) < s.cfg.MaxSpectators
}

func (s *Server) registerSpectatorLocked(connection *ClientConnection) {
	s.spectators[connection.ViewerID] = connection
	if connection.SessionMode == sessionModeDebugSimulation && connection.DebugSessionID == s.lobby.DebugSessionID {
		s.lobby.DebugSpectatorGraceUntil = time.Time{}
	}
	s.scheduleRegistryRefresh()
}

func (s *Server) removeSpectatorLocked(connection *ClientConnection, now time.Time) {
	current, ok := s.spectators[connection.ViewerID]
	if ok && current == connection {
		delete(s.spectators, connection.ViewerID)
	}
	s.syncDebugSpectatorGraceLocked(now)
	s.scheduleRegistryRefresh()
}

func (s *Server) syncDebugSpectatorGraceLocked(now time.Time) {
	if s.lobby.MatchKind != matchKindDebugBotSim {
		s.lobby.DebugSpectatorGraceUntil = time.Time{}
		return
	}
	if s.connectedDebugSpectatorsLocked() > 0 {
		s.lobby.DebugSpectatorGraceUntil = time.Time{}
		return
	}
	if s.lobby.DebugSpectatorGraceUntil.IsZero() {
		s.lobby.DebugSpectatorGraceUntil = now.Add(s.cfg.DebugSpectatorGracePeriod)
	}
}

func (s *Server) shouldKeepObservedSessionAliveLocked(now time.Time) bool {
	return s.connectedObserversForIdleLocked(now) > 0
}

func (s *Server) shouldCollectGameplayMetricsLocked() bool {
	return s.lobby.MatchKind != matchKindDebugBotSim
}

func (s *Server) clearDebugMatchStateLocked() {
	s.lobby.MatchKind = matchKindNormal
	s.lobby.DebugSessionID = ""
	s.lobby.DebugBotCount = 0
	s.lobby.DebugSpectatorGraceUntil = time.Time{}
	s.matchRNG = nil
	s.scheduleRegistryRefresh()
}

func (s *Server) startDebugMatchLocked(now time.Time, debugSessionID string, botCount int, seed *int64) {
	s.lobby.MatchKind = matchKindDebugBotSim
	s.lobby.DebugSessionID = debugSessionID
	s.lobby.DebugBotCount = s.clampDebugBotCountLocked(botCount)
	s.lobby.DebugSpectatorGraceUntil = time.Time{}
	if seed != nil {
		s.matchRNG = mathrand.New(mathrand.NewSource(*seed))
	} else {
		s.matchRNG = mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	}
	s.resetMatchLocked(now)
	s.maintainDebugBotCountLocked(now)
	s.scheduleRegistryRefresh()
}

func (s *Server) clampDebugBotCountLocked(botCount int) int {
	if botCount <= 0 {
		botCount = 1
	}
	if s.cfg.MaxPlayers > 0 && botCount > s.cfg.MaxPlayers {
		botCount = s.cfg.MaxPlayers
	}
	return botCount
}

func (s *Server) maintainDebugBotCountLocked(now time.Time) {
	if s.lobby.MatchKind != matchKindDebugBotSim {
		return
	}

	target := s.clampDebugBotCountLocked(s.lobby.DebugBotCount)
	s.lobby.DebugBotCount = target

	botIDs := make([]string, 0)
	for id, player := range s.lobby.Players {
		if player.IsBot {
			botIDs = append(botIDs, id)
		}
	}

	sort.Strings(botIDs)
	for len(botIDs) > target {
		id := botIDs[len(botIDs)-1]
		delete(s.lobby.Players, id)
		botIDs = botIDs[:len(botIDs)-1]
	}

	for len(botIDs) < target {
		bot := s.newBotLocked(now)
		s.lobby.Players[bot.ID] = bot
		botIDs = append(botIDs, bot.ID)
	}
}

func (s *Server) chooseDefaultCameraTargetLocked(preferBots bool) string {
	type candidate struct {
		id    string
		name  string
		bot   bool
		alive bool
	}

	candidates := make([]candidate, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		candidates = append(candidates, candidate{
			id:    player.ID,
			name:  player.Name,
			bot:   player.IsBot,
			alive: player.Alive,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if preferBots && candidates[i].bot != candidates[j].bot {
			return candidates[i].bot
		}
		if candidates[i].alive != candidates[j].alive {
			return candidates[i].alive
		}
		if candidates[i].name != candidates[j].name {
			return candidates[i].name < candidates[j].name
		}
		return candidates[i].id < candidates[j].id
	})

	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].id
}
