package game

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	matchMetricsSchemaVersion    = 1
	matchMetricsCollectorVersion = "v1"

	matchEndReasonTimeLimit      = "time_limit"
	matchEndReasonNoHumans       = "no_humans"
	matchEndReasonDrain          = "drain"
	matchEndReasonDebugAbandoned = "debug_abandoned"
)

type matchMetricsReport struct {
	SchemaVersion       int                       `json:"schemaVersion"`
	CollectorVersion    string                    `json:"collectorVersion"`
	MatchID             string                    `json:"matchId"`
	LobbyID             string                    `json:"lobbyId"`
	MatchKind           string                    `json:"matchKind"`
	EndReason           string                    `json:"endReason"`
	DrainFlag           bool                      `json:"drainFlag"`
	IsDebug             bool                      `json:"isDebug"`
	StartedAt           time.Time                 `json:"startedAt"`
	EndedAt             time.Time                 `json:"endedAt"`
	DurationMs          int64                     `json:"durationMs"`
	HumanCount          int                       `json:"humanCount"`
	BotCount            int                       `json:"botCount"`
	PeakConcurrentHuman int                       `json:"peakConcurrentHumans"`
	ConfigSnapshot      map[string]any            `json:"configSnapshot"`
	MatchMetrics        map[string]any            `json:"matchMetrics"`
	Participants        []matchMetricsParticipant `json:"participants"`
	Events              []matchMetricsEvent       `json:"events"`
}

type matchMetricsParticipant struct {
	ParticipantID       string         `json:"participantId"`
	SessionPlayerIDHash string         `json:"sessionPlayerIdHash,omitempty"`
	IsBot               bool           `json:"isBot"`
	BotLevel            string         `json:"botLevel,omitempty"`
	Placement           int            `json:"placement"`
	SummaryMetrics      map[string]any `json:"summaryMetrics"`
}

type matchMetricsEvent struct {
	TimestampMs         int64          `json:"tsMs"`
	EventSeq            int64          `json:"eventSeq"`
	Tick                *int64         `json:"tick,omitempty"`
	EventType           string         `json:"eventType"`
	ActorParticipantID  string         `json:"actorParticipantId,omitempty"`
	TargetParticipantID string         `json:"targetParticipantId,omitempty"`
	Payload             map[string]any `json:"payload"`
}

type MatchMetricsCollector struct {
	matchID        string
	lobbyID        string
	matchKind      MatchKind
	startedAt      time.Time
	tickRate       int
	configSnapshot map[string]any

	finalized            bool
	nextParticipantIndex int
	nextEventSeq         int64
	participantsByPlayer map[string]*matchParticipantMetrics
	events               []matchMetricsEvent

	peakConcurrentHumans int
	totalPickups         int
	totalShots           int
	totalRespawns        int
	totalKills           int
	humanVsHumanKills    int
	humanVsBotKills      int
	botVsHumanKills      int
	botVsBotKills        int
}

type matchParticipantMetrics struct {
	playerID            string
	participantID       string
	sessionPlayerIDHash string
	isBot               bool
	botLevel            BotLevel

	joinedAt        time.Time
	lastConnected   time.Time
	connected       bool
	connectedMs     int64
	lastAliveAt     time.Time
	alive           bool
	aliveMs         int64
	firstCombatMs   int64
	firstKillMs     int64
	firstDeathMs    int64
	kills           int
	deaths          int
	pickups         int
	shotsFired      int
	respawns        int
	finalMass       float64
	peakMass        float64
	massSampleSum   float64
	massSampleCount int
}

func newMatchMetricsCollector(lobby *Lobby, cfg Config, now time.Time) *MatchMetricsCollector {
	return &MatchMetricsCollector{
		matchID:              lobby.MatchID,
		lobbyID:              lobby.ID,
		matchKind:            lobby.MatchKind,
		startedAt:            now,
		tickRate:             cfg.TickRate,
		configSnapshot:       matchMetricsConfigSnapshot(cfg),
		participantsByPlayer: make(map[string]*matchParticipantMetrics),
	}
}

func matchMetricsConfigSnapshot(cfg Config) map[string]any {
	return map[string]any{
		"matchDurationMs":       cfg.MatchDuration.Milliseconds(),
		"maxPlayers":            cfg.MaxPlayers,
		"worldWidth":            cfg.WorldWidth,
		"worldHeight":           cfg.WorldHeight,
		"tickRate":              cfg.TickRate,
		"botDifficultyMode":     cfg.BotDifficultyMode,
		"botDifficultyProfiles": cfg.BotDifficultyProfiles,
	}
}

func (s *Server) beginMatchAnalyticsLocked(now time.Time) {
	if !s.cfg.MatchAnalyticsEnabled {
		s.matchMetrics = nil
		return
	}
	s.matchMetrics = newMatchMetricsCollector(s.lobby, s.cfg, now)
}

func (s *Server) registerMatchParticipantLocked(player *Player, now time.Time) {
	if s.matchMetrics == nil {
		return
	}
	s.matchMetrics.RegisterParticipant(player, s.sessionPlayerIDHash(player), now)
}

func (c *MatchMetricsCollector) RegisterParticipant(player *Player, sessionHash string, now time.Time) {
	if c == nil || player == nil || c.finalized {
		return
	}
	participant := c.participantsByPlayer[player.ID]
	if participant == nil {
		c.nextParticipantIndex++
		participant = &matchParticipantMetrics{
			playerID:            player.ID,
			participantID:       fmt.Sprintf("participant-%03d", c.nextParticipantIndex),
			sessionPlayerIDHash: sessionHash,
			isBot:               player.IsBot,
			botLevel:            player.BotLevel,
			joinedAt:            now,
			finalMass:           player.Mass,
			peakMass:            player.Mass,
		}
		c.participantsByPlayer[player.ID] = participant
	}
	participant.isBot = player.IsBot
	participant.botLevel = player.BotLevel
	participant.finalMass = player.Mass
	participant.peakMass = math.Max(participant.peakMass, player.Mass)
	if !player.IsBot && player.Connected && !participant.connected {
		participant.connected = true
		participant.lastConnected = now
	}
	if player.Alive && !participant.alive {
		participant.alive = true
		participant.lastAliveAt = now
	}
}

func (s *Server) sessionPlayerIDHash(player *Player) string {
	if player == nil || player.IsBot || strings.TrimSpace(player.ID) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.ReportSecret))
	_, _ = mac.Write([]byte(player.ID))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *MatchMetricsCollector) Sample(players map[string]*Player, now time.Time) {
	if c == nil || c.finalized {
		return
	}
	humans := 0
	for _, player := range players {
		if player == nil {
			continue
		}
		participant := c.participantsByPlayer[player.ID]
		if participant == nil {
			continue
		}
		if !player.IsBot && player.Connected {
			humans++
		}
		participant.finalMass = player.Mass
		participant.peakMass = math.Max(participant.peakMass, player.Mass)
		participant.massSampleSum += player.Mass
		participant.massSampleCount++
	}
	if humans > c.peakConcurrentHumans {
		c.peakConcurrentHumans = humans
	}
}

func (c *MatchMetricsCollector) OnDisconnect(player *Player, now time.Time) {
	participant := c.participant(player)
	if participant == nil {
		return
	}
	if participant.connected {
		participant.connectedMs += maxDurationMs(now.Sub(participant.lastConnected))
		participant.connected = false
	}
	if participant.alive {
		participant.aliveMs += maxDurationMs(now.Sub(participant.lastAliveAt))
		participant.alive = false
	}
	c.addEvent(now, "disconnect", participant.participantID, "", nil)
}

func (c *MatchMetricsCollector) OnPickup(player *Player, massGain, healthGain float64, now time.Time) {
	participant := c.participant(player)
	if participant == nil {
		return
	}
	participant.pickups++
	c.totalPickups++
	c.addEvent(now, "pickup", participant.participantID, "", map[string]any{
		"massGain":   massGain,
		"healthGain": healthGain,
	})
}

func (c *MatchMetricsCollector) OnShot(player *Player, now time.Time) {
	participant := c.participant(player)
	if participant == nil {
		return
	}
	participant.shotsFired++
	c.totalShots++
}

func (c *MatchMetricsCollector) OnRespawn(player *Player, now time.Time) {
	participant := c.participant(player)
	if participant == nil {
		return
	}
	participant.respawns++
	c.totalRespawns++
	if !participant.alive && (participant.isBot || participant.connected) {
		participant.alive = true
		participant.lastAliveAt = now
	}
	c.addEvent(now, "respawn", participant.participantID, "", nil)
}

func (c *MatchMetricsCollector) OnKill(victim *Player, killer *Player, reason string, now time.Time) {
	victimMetrics := c.participant(victim)
	if victimMetrics == nil {
		return
	}
	victimMetrics.deaths++
	if victimMetrics.firstDeathMs == 0 {
		victimMetrics.firstDeathMs = c.elapsedMs(now)
	}
	if victimMetrics.firstCombatMs == 0 {
		victimMetrics.firstCombatMs = c.elapsedMs(now)
	}
	if victimMetrics.alive {
		victimMetrics.aliveMs += maxDurationMs(now.Sub(victimMetrics.lastAliveAt))
		victimMetrics.alive = false
	}

	var killerID string
	killerMetrics := c.participant(killer)
	if killerMetrics != nil && killer.ID != victim.ID {
		killerMetrics.kills++
		if killerMetrics.firstKillMs == 0 {
			killerMetrics.firstKillMs = c.elapsedMs(now)
		}
		if killerMetrics.firstCombatMs == 0 {
			killerMetrics.firstCombatMs = c.elapsedMs(now)
		}
		killerID = killerMetrics.participantID
		c.recordKillSplit(killerMetrics, victimMetrics)
	}
	c.totalKills++
	c.addEvent(now, "kill", killerID, victimMetrics.participantID, map[string]any{"reason": reason})
	c.addEvent(now, "death", victimMetrics.participantID, killerID, map[string]any{"reason": reason})
}

func (c *MatchMetricsCollector) Finalize(
	players map[string]*Player,
	scoreboard []scoreboardResult,
	endReason string,
	drain bool,
	now time.Time,
) *matchMetricsReport {
	if c == nil || c.finalized {
		return nil
	}
	c.finalized = true
	for _, player := range players {
		if player == nil {
			continue
		}
		if participant := c.participantsByPlayer[player.ID]; participant != nil {
			participant.finalMass = player.Mass
			participant.peakMass = math.Max(participant.peakMass, player.Mass)
		}
	}
	for _, participant := range c.participantsByPlayer {
		if participant.connected {
			participant.connectedMs += maxDurationMs(now.Sub(participant.lastConnected))
			participant.connected = false
		}
		if participant.alive {
			participant.aliveMs += maxDurationMs(now.Sub(participant.lastAliveAt))
			participant.alive = false
		}
	}

	participants := c.finalParticipants(c.placementByPlayer(scoreboard))
	humanCount, botCount := 0, 0
	for _, participant := range participants {
		if participant.IsBot {
			botCount++
		} else {
			humanCount++
		}
	}
	events := append([]matchMetricsEvent(nil), c.events...)
	return &matchMetricsReport{
		SchemaVersion:       matchMetricsSchemaVersion,
		CollectorVersion:    matchMetricsCollectorVersion,
		MatchID:             c.matchID,
		LobbyID:             c.lobbyID,
		MatchKind:           string(c.matchKind),
		EndReason:           endReason,
		DrainFlag:           drain,
		IsDebug:             c.matchKind == matchKindDebugBotSim,
		StartedAt:           c.startedAt,
		EndedAt:             now,
		DurationMs:          maxDurationMs(now.Sub(c.startedAt)),
		HumanCount:          humanCount,
		BotCount:            botCount,
		PeakConcurrentHuman: c.peakConcurrentHumans,
		ConfigSnapshot:      copyMap(c.configSnapshot),
		MatchMetrics: map[string]any{
			"totalKills":        c.totalKills,
			"humanVsHumanKills": c.humanVsHumanKills,
			"humanVsBotKills":   c.humanVsBotKills,
			"botVsHumanKills":   c.botVsHumanKills,
			"botVsBotKills":     c.botVsBotKills,
			"totalPickups":      c.totalPickups,
			"totalShots":        c.totalShots,
			"totalRespawns":     c.totalRespawns,
		},
		Participants: participants,
		Events:       events,
	}
}

type matchParticipantPlacement struct {
	playerID       string
	participantID  string
	totalScore     int
	finalMass      float64
	scoreboardRank int
}

func (c *MatchMetricsCollector) placementByPlayer(scoreboard []scoreboardResult) map[string]int {
	scoreboardByPlayer := make(map[string]scoreboardResult, len(scoreboard))
	scoreboardRankByPlayer := make(map[string]int, len(scoreboard))
	for index, result := range scoreboard {
		scoreboardByPlayer[result.PlayerID] = result
		scoreboardRankByPlayer[result.PlayerID] = index
	}

	entries := make([]matchParticipantPlacement, 0, len(c.participantsByPlayer))
	missingScoreboardRank := len(scoreboard) + len(c.participantsByPlayer)
	for _, participant := range c.participantsByPlayer {
		totalScore := participant.kills + int(math.Floor(participant.finalMass/50))
		finalMass := participant.finalMass
		scoreboardRank := missingScoreboardRank
		if result, ok := scoreboardByPlayer[participant.playerID]; ok {
			totalScore = result.TotalScore
			finalMass = result.FinalMass
			scoreboardRank = scoreboardRankByPlayer[participant.playerID]
		}
		entries = append(entries, matchParticipantPlacement{
			playerID:       participant.playerID,
			participantID:  participant.participantID,
			totalScore:     totalScore,
			finalMass:      finalMass,
			scoreboardRank: scoreboardRank,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].totalScore != entries[j].totalScore {
			return entries[i].totalScore > entries[j].totalScore
		}
		if entries[i].finalMass != entries[j].finalMass {
			return entries[i].finalMass > entries[j].finalMass
		}
		if entries[i].scoreboardRank != entries[j].scoreboardRank {
			return entries[i].scoreboardRank < entries[j].scoreboardRank
		}
		return entries[i].participantID < entries[j].participantID
	})

	placements := make(map[string]int, len(entries))
	for index, entry := range entries {
		placements[entry.playerID] = index + 1
	}
	return placements
}

func (c *MatchMetricsCollector) finalParticipants(placementByPlayer map[string]int) []matchMetricsParticipant {
	participants := make([]*matchParticipantMetrics, 0, len(c.participantsByPlayer))
	for _, participant := range c.participantsByPlayer {
		participants = append(participants, participant)
	}
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].participantID < participants[j].participantID
	})
	reportParticipants := make([]matchMetricsParticipant, 0, len(participants))
	for _, participant := range participants {
		averageMass := participant.finalMass
		if participant.massSampleCount > 0 {
			averageMass = participant.massSampleSum / float64(participant.massSampleCount)
		}
		reportParticipants = append(reportParticipants, matchMetricsParticipant{
			ParticipantID:       participant.participantID,
			SessionPlayerIDHash: participant.sessionPlayerIDHash,
			IsBot:               participant.isBot,
			BotLevel:            string(participant.botLevel),
			Placement:           placementByPlayer[participant.playerID],
			SummaryMetrics: map[string]any{
				"kills":               participant.kills,
				"deaths":              participant.deaths,
				"pickups":             participant.pickups,
				"shotsFired":          participant.shotsFired,
				"respawns":            participant.respawns,
				"connectedDurationMs": participant.connectedMs,
				"aliveDurationMs":     participant.aliveMs,
				"finalMass":           participant.finalMass,
				"peakMass":            participant.peakMass,
				"averageMass":         averageMass,
				"firstKillMs":         participant.firstKillMs,
				"firstDeathMs":        participant.firstDeathMs,
				"firstCombatMs":       participant.firstCombatMs,
			},
		})
	}
	return reportParticipants
}

func (c *MatchMetricsCollector) participant(player *Player) *matchParticipantMetrics {
	if c == nil || player == nil || c.finalized {
		return nil
	}
	return c.participantsByPlayer[player.ID]
}

func (c *MatchMetricsCollector) recordKillSplit(killer, victim *matchParticipantMetrics) {
	switch {
	case !killer.isBot && !victim.isBot:
		c.humanVsHumanKills++
	case !killer.isBot && victim.isBot:
		c.humanVsBotKills++
	case killer.isBot && !victim.isBot:
		c.botVsHumanKills++
	default:
		c.botVsBotKills++
	}
}

func (c *MatchMetricsCollector) addEvent(now time.Time, eventType, actorID, targetID string, payload map[string]any) {
	if c == nil || c.finalized {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	c.nextEventSeq++
	c.events = append(c.events, matchMetricsEvent{
		TimestampMs:         c.elapsedMs(now),
		EventSeq:            c.nextEventSeq,
		Tick:                c.tickAt(now),
		EventType:           eventType,
		ActorParticipantID:  actorID,
		TargetParticipantID: targetID,
		Payload:             payload,
	})
}

func (c *MatchMetricsCollector) elapsedMs(now time.Time) int64 {
	return maxDurationMs(now.Sub(c.startedAt))
}

func (c *MatchMetricsCollector) tickAt(now time.Time) *int64 {
	if c.tickRate <= 0 {
		return nil
	}
	tick := c.elapsedMs(now) * int64(c.tickRate) / 1000
	return &tick
}

func maxDurationMs(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return duration.Milliseconds()
}

func copyMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (s *Server) sampleMatchAnalyticsLocked(now time.Time) {
	if s.matchMetrics != nil {
		s.matchMetrics.Sample(s.lobby.Players, now)
	}
}

func (s *Server) finalizeMatchAnalyticsLocked(endReason string, now time.Time, drain bool) {
	if s.matchMetrics == nil {
		return
	}
	scoreboard := s.scoreboardLocked()
	report := s.matchMetrics.Finalize(s.lobby.Players, scoreboard, endReason, drain, now)
	if report == nil {
		return
	}
	go s.reportMatchMetrics(*report)
}

func (s *Server) reportMatchMetrics(report matchMetricsReport) {
	body, err := json.Marshal(report)
	if err != nil {
		s.logMatchMetrics("match metrics payload failed: %v", err)
		MatchMetricsReportsDropped.Inc()
		return
	}
	url := strings.TrimRight(s.cfg.APIServerURL, "/") + "/api/match-metrics/report"
	attempts := s.cfg.MatchAnalyticsReportRetries + 1
	if attempts <= 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		statusCode, err := s.postMatchMetricsReport(url, body)
		if err == nil && statusCode >= 200 && statusCode < 300 {
			MatchMetricsReportsSent.Inc()
			return
		}
		if !shouldRetryMatchMetrics(statusCode, err) {
			if err != nil {
				s.logMatchMetrics("match metrics report failed permanently: %v", err)
			} else {
				s.logMatchMetrics("match metrics report rejected permanently: status=%d", statusCode)
			}
			MatchMetricsReportsDropped.Inc()
			return
		}
		if attempt < attempts {
			time.Sleep(s.cfg.MatchAnalyticsRetryDelay)
		}
	}
	s.logMatchMetrics("match metrics report exhausted retries for match %s", report.MatchID)
	MatchMetricsReportsDropped.Inc()
}

func (s *Server) logMatchMetrics(format string, args ...any) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Printf(format, args...)
}

func (s *Server) postMatchMetricsReport(url string, body []byte) (int, error) {
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Report-Signature", signPayload(body, s.cfg.ReportSecret))

	response, err := s.httpClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func shouldRetryMatchMetrics(statusCode int, err error) bool {
	if err != nil {
		return true
	}
	return statusCode >= 500
}
