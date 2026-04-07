package game

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	mathrand "math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

const (
	worldWidth        = 4000
	worldHeight       = 4000
	gravityAccel      = 2400
	drag              = 0.98
	terminalSpeed     = 900
	playerRadiusScale = 3
	numObjects        = 200
	startingMass      = 10
	startingHealth    = 100
	projectileSpeed   = 1250
	projectileDamage  = 28
	projectileRadius  = 5
	shootCooldown     = 250 * time.Millisecond
	projectileTTL     = 1200 * time.Millisecond
)

var palette = []string{
	"#ff7d61",
	"#68e1fd",
	"#9bff9e",
	"#ffde73",
	"#b29cff",
	"#ff97c9",
}

type Config struct {
	Port                 string
	JWTSecret            string
	ReportSecret         string
	APIServerURL         string
	LobbyID              string
	TickRate             int
	SnapshotRate         int
	MaxPlayers           int
	MatchDuration        time.Duration
	RespawnDelay         time.Duration
	BotFillDelay         time.Duration
	ShutdownDrainTimeout time.Duration
}

func LoadConfig() Config {
	return Config{
		Port:                 envOrDefault("PORT", "8080"),
		JWTSecret:            envOrDefault("JWT_SECRET", "dev-secret"),
		ReportSecret:         envOrDefault("REPORT_SHARED_SECRET", envOrDefault("JWT_SECRET", "dev-secret")),
		APIServerURL:         envOrDefault("API_SERVER_URL", "http://api-server:8081"),
		LobbyID:              envOrDefault("LOBBY_ID", "local-lobby"),
		TickRate:             envInt("TICK_RATE", 60),
		SnapshotRate:         envInt("SNAPSHOT_RATE", 20),
		MaxPlayers:           envInt("MAX_PLAYERS", 10),
		MatchDuration:        envDuration("MATCH_DURATION", 5*time.Minute),
		RespawnDelay:         envDuration("RESPAWN_DELAY", 2*time.Second),
		BotFillDelay:         envDuration("BOT_FILL_DELAY", 5*time.Second),
		ShutdownDrainTimeout: envDuration("SHUTDOWN_DRAIN_TIMEOUT", 30*time.Second),
	}
}

type Server struct {
	cfg      Config
	logger   *log.Logger
	upgrader websocket.Upgrader

	mu       sync.RWMutex
	lobby    *Lobby
	draining bool

	rng        *mathrand.Rand
	httpClient *http.Client
}

type Lobby struct {
	ID         string
	MatchID    string
	MatchStart time.Time
	MatchEnds  time.Time
	MatchOver  bool

	Players     map[string]*Player
	Objects     []*Collectible
	Projectiles []*Projectile
	KillFeed    []KillFeedEntry
}

type Player struct {
	ID            string
	Name          string
	Color         string
	IsBot         bool
	Connected     bool
	Connection    *ClientConnection
	X             float64
	Y             float64
	VX            float64
	VY            float64
	Mass          float64
	Health        float64
	Angle         float64
	Alive         bool
	RespawnAt     time.Time
	LastShotAt    time.Time
	Input         InputState
	Score         int
	Kills         int
	DeathReason   string
	KilledBy      string
	BotTargetX    float64
	BotTargetY    float64
	BotRetargetAt time.Time
}

type ClientConnection struct {
	PlayerID string
	Socket   *websocket.Conn
	Mu       sync.Mutex
}

type Collectible struct {
	ID        string  `json:"id"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Radius    float64 `json:"radius"`
	Toughness float64 `json:"toughness"`
}

type Projectile struct {
	ID        string
	OwnerID   string
	Color     string
	X         float64
	Y         float64
	VX        float64
	VY        float64
	Radius    float64
	Damage    float64
	ExpiresAt time.Time
}

type InputState struct {
	Angle    float64 `json:"angle"`
	Strength float64 `json:"strength"`
	Shoot    bool    `json:"shoot"`
}

type KillFeedEntry struct {
	ID         string `json:"id"`
	KillerName string `json:"killerName"`
	VictimName string `json:"victimName"`
	AtMs       int64  `json:"atMs"`
}

type matchClaims struct {
	PlayerName string `json:"name"`
	LobbyID    string `json:"lobbyId"`
	jwt.RegisteredClaims
}

type welcomeMessage struct {
	Type     string `json:"type"`
	PlayerID string `json:"playerId"`
	LobbyID  string `json:"lobbyId"`
	MatchID  string `json:"matchId"`
}

type serverNoticeMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type snapshotMessage struct {
	Type            string             `json:"type"`
	ServerTime      int64              `json:"serverTime"`
	World           worldBounds        `json:"world"`
	MatchID         string             `json:"matchId"`
	MatchOver       bool               `json:"matchOver"`
	TimeRemainingMs int64              `json:"timeRemainingMs"`
	Players         []snapshotPlayer   `json:"players"`
	Objects         []*Collectible     `json:"objects"`
	Projectiles     []snapshotShot     `json:"projectiles"`
	KillFeed        []KillFeedEntry    `json:"killFeed"`
	Scoreboard      []scoreboardResult `json:"scoreboard"`
	You             *selfState         `json:"you,omitempty"`
	ServerNotice    string             `json:"serverNotice,omitempty"`
}

type worldBounds struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type snapshotPlayer struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	VX          float64 `json:"vx"`
	VY          float64 `json:"vy"`
	Mass        float64 `json:"mass"`
	Radius      float64 `json:"radius"`
	Angle       float64 `json:"angle"`
	Health      float64 `json:"health"`
	IsAlive     bool    `json:"isAlive"`
	RespawnInMs int64   `json:"respawnInMs"`
	IsBot       bool    `json:"isBot"`
	Color       string  `json:"color"`
}

type snapshotShot struct {
	ID      string  `json:"id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Radius  float64 `json:"radius"`
	OwnerID string  `json:"ownerId"`
	Color   string  `json:"color"`
}

type selfState struct {
	PlayerID    string  `json:"playerId"`
	PlayerName  string  `json:"playerName"`
	Score       int     `json:"score"`
	Mass        float64 `json:"mass"`
	Health      float64 `json:"health"`
	Kills       int     `json:"kills"`
	IsAlive     bool    `json:"isAlive"`
	RespawnInMs int64   `json:"respawnInMs"`
	DeathReason string  `json:"deathReason,omitempty"`
	KilledBy    string  `json:"killedBy,omitempty"`
}

type scoreboardResult struct {
	PlayerID   string  `json:"playerId"`
	PlayerName string  `json:"playerName"`
	Kills      int     `json:"kills"`
	FinalMass  float64 `json:"finalMass"`
	MassBonus  int     `json:"massBonus"`
	TotalScore int     `json:"totalScore"`
	IsBot      bool    `json:"isBot"`
}

type leaderboardReport struct {
	LobbyID string                 `json:"lobbyId"`
	Results []leaderboardPlayerHit `json:"results"`
}

type leaderboardPlayerHit struct {
	PlayerID   string  `json:"playerId"`
	PlayerName string  `json:"playerName"`
	Kills      int     `json:"kills"`
	FinalMass  float64 `json:"finalMass"`
}

func NewServer(cfg Config, logger *log.Logger) *Server {
	now := time.Now()

	return &Server{
		cfg:    cfg,
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		lobby: &Lobby{
			ID:         cfg.LobbyID,
			MatchID:    randomID("match"),
			MatchStart: now,
			MatchEnds:  now.Add(cfg.MatchDuration),
			Players:    make(map[string]*Player),
			Objects:    make([]*Collectible, 0, numObjects),
		},
		rng:        mathrand.New(mathrand.NewSource(time.Now().UnixNano())),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *Server) Start(ctx context.Context) {
	s.mu.Lock()
	if len(s.lobby.Objects) == 0 {
		s.spawnObjectsLocked()
	}
	s.mu.Unlock()

	go s.loop(ctx)
}

func (s *Server) loop(ctx context.Context) {
	tick := time.NewTicker(time.Second / time.Duration(s.cfg.TickRate))
	snapshotTicker := time.NewTicker(time.Second / time.Duration(s.cfg.SnapshotRate))
	defer tick.Stop()
	defer snapshotTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			s.step(now)
		case now := <-snapshotTicker.C:
			s.broadcastSnapshots(now)
		}
	}
}

func (s *Server) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) HandleReadyz(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.draining || s.activeSlotsLocked() >= s.cfg.MaxPlayers {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) HandleMetrics(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	players := 0
	for _, player := range s.lobby.Players {
		if !player.IsBot && player.Connected {
			players++
		}
	}

	body := fmt.Sprintf(
		"# HELP active_players_per_pod Connected human players in this game-server pod.\n# TYPE active_players_per_pod gauge\nactive_players_per_pod %d\n",
		players,
	)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(body))
}

func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		http.Error(w, "server draining", http.StatusServiceUnavailable)
		return
	}
	s.mu.Unlock()

	tokenString := r.URL.Query().Get("token")
	claims, err := s.parseToken(tokenString)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	socket, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Printf("websocket upgrade failed: %v", err)
		return
	}

	connection := &ClientConnection{
		PlayerID: claims.Subject,
		Socket:   socket,
	}

	s.mu.Lock()
	now := time.Now()
	if s.lobby.MatchOver {
		s.resetMatchLocked(now)
	}
	player := s.upsertHumanPlayerLocked(claims.Subject, claims.PlayerName, connection, now)
	s.mu.Unlock()

	_ = connection.writeJSON(welcomeMessage{
		Type:     "welcome",
		PlayerID: player.ID,
		LobbyID:  s.lobby.ID,
		MatchID:  s.lobby.MatchID,
	})

	go s.readLoop(connection)
}

func (s *Server) BeginDrain(message string) {
	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		return
	}
	s.draining = true
	connections := s.currentConnectionsLocked()
	s.mu.Unlock()

	for _, connection := range connections {
		_ = connection.writeJSON(serverNoticeMessage{
			Type:    "server_notice",
			Message: message,
		})
	}
}

func (s *Server) WaitForDrain(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.RLock()
		done := s.lobby.MatchOver
		s.mu.RUnlock()

		if done {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) CloseAllConnections() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, connection := range s.currentConnectionsLocked() {
		_ = connection.Socket.Close()
	}
}

func (s *Server) readLoop(connection *ClientConnection) {
	defer func() {
		s.mu.Lock()
		if player, ok := s.lobby.Players[connection.PlayerID]; ok {
			player.Connected = false
			player.Connection = nil
		}
		s.mu.Unlock()
		_ = connection.Socket.Close()
	}()

	for {
		var input InputState
		if err := connection.Socket.ReadJSON(&input); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && err != io.EOF {
				s.logger.Printf("read failed for %s: %v", connection.PlayerID, err)
			}
			return
		}

		s.mu.Lock()
		if player, ok := s.lobby.Players[connection.PlayerID]; ok {
			player.Input = InputState{
				Angle:    input.Angle,
				Strength: clamp(input.Strength, 0, 1),
				Shoot:    input.Shoot,
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) step(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.draining && s.lobby.MatchOver {
		return
	}

	if !s.lobby.MatchOver {
		s.fillBotsLocked(now)
		s.updateBotsLocked(now)
		s.updatePlayersLocked(now)
		s.updateProjectilesLocked(now)
		s.resolveObjectCollisionsLocked(now)
		s.resolvePlayerCollisionsLocked(now)
		s.resolveProjectileCollisionsLocked(now)
		if now.After(s.lobby.MatchEnds) {
			s.finishMatchLocked()
			go s.reportLeaderboard(s.scoreboardLocked())
		}
		s.handleRespawnsLocked(now)
	}
}

type snapshotDelivery struct {
	connection *ClientConnection
	self       *selfState
}

func (s *Server) broadcastSnapshots(now time.Time) {
	s.mu.RLock()
	deliveries := make([]snapshotDelivery, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		if player.Connected && player.Connection != nil {
			deliveries = append(deliveries, snapshotDelivery{
				connection: player.Connection,
				self:       buildSelfState(player, now),
			})
		}
	}

	scoreboard := s.scoreboardLocked()
	messageBase := snapshotMessage{
		Type:            "snapshot",
		ServerTime:      now.UnixMilli(),
		World:           worldBounds{Width: worldWidth, Height: worldHeight},
		MatchID:         s.lobby.MatchID,
		MatchOver:       s.lobby.MatchOver,
		TimeRemainingMs: maxInt64(s.lobby.MatchEnds.Sub(now).Milliseconds(), 0),
		Players:         s.snapshotPlayersLocked(now),
		Objects:         append([]*Collectible(nil), s.lobby.Objects...),
		Projectiles:     s.snapshotProjectilesLocked(),
		KillFeed:        append([]KillFeedEntry(nil), s.lobby.KillFeed...),
		Scoreboard:      scoreboard,
	}
	serverNotice := ""
	if s.draining {
		serverNotice = "server shutting down"
	}
	s.mu.RUnlock()

	for _, delivery := range deliveries {
		msg := messageBase
		msg.ServerNotice = serverNotice
		msg.You = delivery.self
		_ = delivery.connection.writeJSON(msg)
	}
}

func (s *Server) fillBotsLocked(now time.Time) {
	humans := 0
	total := 0
	for _, player := range s.lobby.Players {
		if player.IsBot {
			total++
			continue
		}
		if player.Connected {
			humans++
			total++
		}
	}

	if humans == 0 || now.Before(s.lobby.MatchStart.Add(s.cfg.BotFillDelay)) {
		return
	}

	for total < s.cfg.MaxPlayers {
		bot := s.newBotLocked(now)
		s.lobby.Players[bot.ID] = bot
		total++
	}
}

func (s *Server) updateBotsLocked(now time.Time) {
	livePlayers := make([]*Player, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		if player.Alive {
			livePlayers = append(livePlayers, player)
		}
	}

	for _, player := range s.lobby.Players {
		if !player.IsBot || !player.Alive {
			continue
		}

		target, distance := s.closestTargetLocked(player, livePlayers)
		if target == nil || now.After(player.BotRetargetAt) {
			player.BotTargetX = s.randFloat(200, worldWidth-200)
			player.BotTargetY = s.randFloat(200, worldHeight-200)
			player.BotRetargetAt = now.Add(1800 * time.Millisecond)
		}

		targetX := player.BotTargetX
		targetY := player.BotTargetY
		shoot := false

		if target != nil {
			if target.Mass < player.Mass*0.9 {
				targetX = target.X
				targetY = target.Y
				shoot = distance < 650
			} else if target.Mass > player.Mass*1.15 {
				targetX = player.X - (target.X - player.X)
				targetY = player.Y - (target.Y - player.Y)
			}
		}

		angle := math.Atan2(targetY-player.Y, targetX-player.X)
		player.Input = InputState{
			Angle:    angle,
			Strength: 0.9,
			Shoot:    shoot,
		}
	}
}

func (s *Server) updatePlayersLocked(now time.Time) {
	dt := 1.0 / float64(s.cfg.TickRate)

	for _, player := range s.lobby.Players {
		if !player.Alive {
			continue
		}

		player.Angle = player.Input.Angle
		accel := gravityAccel * clamp(player.Input.Strength, 0, 1) * dt
		player.VX += math.Cos(player.Input.Angle) * accel
		player.VY += math.Sin(player.Input.Angle) * accel
		player.VX *= drag
		player.VY *= drag

		speed := math.Hypot(player.VX, player.VY)
		if speed > terminalSpeed {
			scale := terminalSpeed / speed
			player.VX *= scale
			player.VY *= scale
		}

		player.X += player.VX * dt
		player.Y += player.VY * dt
		s.clampPlayerToWorldLocked(player)

		if player.Input.Shoot && now.Sub(player.LastShotAt) >= shootCooldown {
			s.spawnProjectileLocked(player, now)
			player.LastShotAt = now
		}
	}
}

func (s *Server) updateProjectilesLocked(now time.Time) {
	dt := 1.0 / float64(s.cfg.TickRate)
	projectiles := s.lobby.Projectiles[:0]

	for _, projectile := range s.lobby.Projectiles {
		if now.After(projectile.ExpiresAt) {
			continue
		}

		projectile.X += projectile.VX * dt
		projectile.Y += projectile.VY * dt
		if projectile.X < 0 || projectile.X > worldWidth || projectile.Y < 0 || projectile.Y > worldHeight {
			continue
		}

		projectiles = append(projectiles, projectile)
	}

	s.lobby.Projectiles = projectiles
}

func (s *Server) resolveObjectCollisionsLocked(now time.Time) {
	for _, player := range s.lobby.Players {
		if !player.Alive {
			continue
		}

		speed := math.Hypot(player.VX, player.VY)
		energy := 0.5 * player.Mass * speed * speed
		playerRadius := radiusForMass(player.Mass)

		for index := range s.lobby.Objects {
			object := s.lobby.Objects[index]
			dx := player.X - object.X
			dy := player.Y - object.Y
			if math.Hypot(dx, dy) >= playerRadius+object.Radius {
				continue
			}

			if energy > object.Toughness {
				player.Mass += object.Toughness / 50
				player.Health = math.Min(startingHealth, player.Health+4)
				s.lobby.Objects[index] = s.spawnObjectLocked()
			} else {
				s.killPlayerLocked(player, nil, "crashed into a dense shard", now)
			}
		}
	}
}

func (s *Server) resolvePlayerCollisionsLocked(now time.Time) {
	livePlayers := make([]*Player, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		if player.Alive {
			livePlayers = append(livePlayers, player)
		}
	}

	for i := 0; i < len(livePlayers); i++ {
		for j := i + 1; j < len(livePlayers); j++ {
			left := livePlayers[i]
			right := livePlayers[j]
			if !left.Alive || !right.Alive {
				continue
			}

			minDistance := radiusForMass(left.Mass) + radiusForMass(right.Mass)
			if math.Hypot(left.X-right.X, left.Y-right.Y) >= minDistance {
				continue
			}

			winner := left
			loser := right
			if right.Mass > left.Mass {
				winner = right
				loser = left
			} else if math.Abs(right.Mass-left.Mass) < 0.01 {
				if math.Hypot(right.VX, right.VY) > math.Hypot(left.VX, left.VY) {
					winner = right
					loser = left
				}
			}

			winner.Mass += loser.Mass * 0.35
			winner.Health = math.Min(startingHealth, winner.Health+8)
			winner.Score++
			winner.Kills++
			s.killPlayerLocked(loser, winner, fmt.Sprintf("rammed by %s", winner.Name), now)
		}
	}
}

func (s *Server) resolveProjectileCollisionsLocked(now time.Time) {
	projectiles := s.lobby.Projectiles[:0]

	for _, projectile := range s.lobby.Projectiles {
		hit := false
		for _, player := range s.lobby.Players {
			if !player.Alive || player.ID == projectile.OwnerID {
				continue
			}

			minDistance := radiusForMass(player.Mass) + projectile.Radius
			if math.Hypot(player.X-projectile.X, player.Y-projectile.Y) >= minDistance {
				continue
			}

			player.Health -= projectile.Damage
			player.Mass = math.Max(startingMass*0.55, player.Mass-projectile.Damage/25)
			if player.Health <= 0 {
				owner := s.lobby.Players[projectile.OwnerID]
				if owner != nil {
					owner.Score++
					owner.Kills++
					s.killPlayerLocked(player, owner, fmt.Sprintf("shot down by %s", owner.Name), now)
				} else {
					s.killPlayerLocked(player, nil, "shot down", now)
				}
			}
			hit = true
			break
		}

		if !hit {
			projectiles = append(projectiles, projectile)
		}
	}

	s.lobby.Projectiles = projectiles
}

func (s *Server) handleRespawnsLocked(now time.Time) {
	for _, player := range s.lobby.Players {
		if player.Alive || player.RespawnAt.IsZero() || now.Before(player.RespawnAt) {
			continue
		}

		player.Alive = true
		player.Health = startingHealth
		player.Mass = startingMass
		player.VX = 0
		player.VY = 0
		player.KilledBy = ""
		player.DeathReason = ""
		player.RespawnAt = time.Time{}
		s.spawnPlayerAtRandomPositionLocked(player)
	}
}

func (s *Server) finishMatchLocked() {
	s.lobby.MatchOver = true
}

func (s *Server) reportLeaderboard(results []scoreboardResult) {
	payload := leaderboardReport{
		LobbyID: s.lobby.ID,
		Results: make([]leaderboardPlayerHit, 0, len(results)),
	}

	for _, result := range results {
		if result.IsBot {
			continue
		}
		payload.Results = append(payload.Results, leaderboardPlayerHit{
			PlayerID:   result.PlayerID,
			PlayerName: result.PlayerName,
			Kills:      result.Kills,
			FinalMass:  result.FinalMass,
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		s.logger.Printf("leaderboard payload failed: %v", err)
		return
	}

	request, err := http.NewRequest(http.MethodPost, s.cfg.APIServerURL+"/api/leaderboard/report", bytes.NewReader(body))
	if err != nil {
		s.logger.Printf("leaderboard request failed: %v", err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Report-Signature", signPayload(body, s.cfg.ReportSecret))

	response, err := s.httpClient.Do(request)
	if err != nil {
		s.logger.Printf("leaderboard report failed: %v", err)
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		s.logger.Printf("leaderboard report rejected: %s", response.Status)
	}
}

func (s *Server) currentConnectionsLocked() []*ClientConnection {
	connections := make([]*ClientConnection, 0)
	for _, player := range s.lobby.Players {
		if player.Connection != nil && player.Connected {
			connections = append(connections, player.Connection)
		}
	}
	return connections
}

func (s *Server) activeSlotsLocked() int {
	total := 0
	for _, player := range s.lobby.Players {
		if player.Connected || player.IsBot {
			total++
		}
	}
	return total
}

func (s *Server) upsertHumanPlayerLocked(id, name string, connection *ClientConnection, now time.Time) *Player {
	player, ok := s.lobby.Players[id]
	if !ok {
		if s.activeSlotsLocked() >= s.cfg.MaxPlayers {
			s.removeOneBotLocked()
		}
		player = &Player{
			ID:    id,
			Name:  sanitizeName(name),
			Color: s.randomColor(),
		}
		player.Health = startingHealth
		player.Mass = startingMass
		player.Alive = true
		s.spawnPlayerAtRandomPositionLocked(player)
		s.lobby.Players[player.ID] = player
	}

	player.Name = sanitizeName(name)
	player.Connected = true
	player.Connection = connection
	player.IsBot = false
	player.Alive = true
	if player.Mass < startingMass {
		player.Mass = startingMass
	}
	if player.Health <= 0 {
		player.Health = startingHealth
	}
	if player.X == 0 && player.Y == 0 {
		s.spawnPlayerAtRandomPositionLocked(player)
	}
	player.RespawnAt = time.Time{}
	player.DeathReason = ""
	player.KilledBy = ""
	player.LastShotAt = now.Add(-shootCooldown)

	return player
}

func (s *Server) removeOneBotLocked() {
	for id, player := range s.lobby.Players {
		if player.IsBot {
			delete(s.lobby.Players, id)
			return
		}
	}
}

func (s *Server) newBotLocked(now time.Time) *Player {
	bot := &Player{
		ID:         randomID("bot"),
		Name:       fmt.Sprintf("Bot-%02d", s.rng.Intn(90)+10),
		Color:      s.randomColor(),
		IsBot:      true,
		Alive:      true,
		Connected:  false,
		Mass:       startingMass,
		Health:     startingHealth,
		LastShotAt: now.Add(-shootCooldown),
	}
	s.spawnPlayerAtRandomPositionLocked(bot)
	return bot
}

func (s *Server) spawnObjectsLocked() {
	s.lobby.Objects = s.lobby.Objects[:0]
	for i := 0; i < numObjects; i++ {
		s.lobby.Objects = append(s.lobby.Objects, s.spawnObjectLocked())
	}
}

func (s *Server) spawnObjectLocked() *Collectible {
	radius := s.randFloat(5, 20)
	return &Collectible{
		ID:        randomID("obj"),
		X:         s.randFloat(radius, worldWidth-radius),
		Y:         s.randFloat(radius, worldHeight-radius),
		Radius:    radius,
		Toughness: s.randFloat(50, 500),
	}
}

func (s *Server) spawnProjectileLocked(player *Player, now time.Time) {
	shot := &Projectile{
		ID:        randomID("shot"),
		OwnerID:   player.ID,
		Color:     player.Color,
		X:         player.X + math.Cos(player.Angle)*(radiusForMass(player.Mass)+10),
		Y:         player.Y + math.Sin(player.Angle)*(radiusForMass(player.Mass)+10),
		VX:        player.VX + math.Cos(player.Angle)*projectileSpeed,
		VY:        player.VY + math.Sin(player.Angle)*projectileSpeed,
		Radius:    projectileRadius,
		Damage:    projectileDamage,
		ExpiresAt: now.Add(projectileTTL),
	}
	s.lobby.Projectiles = append(s.lobby.Projectiles, shot)
}

func (s *Server) clampPlayerToWorldLocked(player *Player) {
	radius := radiusForMass(player.Mass)
	if player.X-radius < 0 {
		player.X = radius
		player.VX = 0
	}
	if player.X+radius > worldWidth {
		player.X = worldWidth - radius
		player.VX = 0
	}
	if player.Y-radius < 0 {
		player.Y = radius
		player.VY = 0
	}
	if player.Y+radius > worldHeight {
		player.Y = worldHeight - radius
		player.VY = 0
	}
}

func (s *Server) spawnPlayerAtRandomPositionLocked(player *Player) {
	radius := radiusForMass(math.Max(player.Mass, startingMass))
	player.X = s.randFloat(radius+20, worldWidth-radius-20)
	player.Y = s.randFloat(radius+20, worldHeight-radius-20)
}

func (s *Server) killPlayerLocked(player *Player, killer *Player, reason string, now time.Time) {
	if !player.Alive {
		return
	}

	player.Alive = false
	player.Health = 0
	player.RespawnAt = now.Add(s.cfg.RespawnDelay)
	player.VX = 0
	player.VY = 0
	player.DeathReason = reason
	if killer != nil {
		player.KilledBy = killer.Name
		s.lobby.KillFeed = append([]KillFeedEntry{{
			ID:         randomID("kill"),
			KillerName: killer.Name,
			VictimName: player.Name,
			AtMs:       now.UnixMilli(),
		}}, s.lobby.KillFeed...)
		if len(s.lobby.KillFeed) > 8 {
			s.lobby.KillFeed = s.lobby.KillFeed[:8]
		}
	}
}

func (s *Server) closestTargetLocked(source *Player, players []*Player) (*Player, float64) {
	var target *Player
	bestDistance := math.MaxFloat64
	for _, candidate := range players {
		if candidate.ID == source.ID || !candidate.Alive {
			continue
		}
		distance := math.Hypot(candidate.X-source.X, candidate.Y-source.Y)
		if distance < bestDistance {
			bestDistance = distance
			target = candidate
		}
	}
	return target, bestDistance
}

func (s *Server) resetMatchLocked(now time.Time) {
	s.lobby.MatchID = randomID("match")
	s.lobby.MatchStart = now
	s.lobby.MatchEnds = now.Add(s.cfg.MatchDuration)
	s.lobby.MatchOver = false
	s.lobby.Projectiles = nil
	s.lobby.KillFeed = nil
	s.spawnObjectsLocked()

	for id, player := range s.lobby.Players {
		if player.IsBot {
			delete(s.lobby.Players, id)
			continue
		}
		player.Score = 0
		player.Kills = 0
		player.Mass = startingMass
		player.Health = startingHealth
		player.Alive = true
		player.RespawnAt = time.Time{}
		player.DeathReason = ""
		player.KilledBy = ""
		player.VX = 0
		player.VY = 0
		s.spawnPlayerAtRandomPositionLocked(player)
	}
}

func (s *Server) scoreboardLocked() []scoreboardResult {
	results := make([]scoreboardResult, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		finalMass := player.Mass
		massBonus := int(math.Floor(finalMass / 50))
		totalScore := player.Kills + massBonus
		results = append(results, scoreboardResult{
			PlayerID:   player.ID,
			PlayerName: player.Name,
			Kills:      player.Kills,
			FinalMass:  finalMass,
			MassBonus:  massBonus,
			TotalScore: totalScore,
			IsBot:      player.IsBot,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].TotalScore == results[j].TotalScore {
			return results[i].FinalMass > results[j].FinalMass
		}
		return results[i].TotalScore > results[j].TotalScore
	})
	return results
}

func (s *Server) snapshotPlayersLocked(now time.Time) []snapshotPlayer {
	players := make([]snapshotPlayer, 0, len(s.lobby.Players))
	for _, player := range s.lobby.Players {
		respawnIn := int64(0)
		if !player.RespawnAt.IsZero() && now.Before(player.RespawnAt) {
			respawnIn = player.RespawnAt.Sub(now).Milliseconds()
		}
		players = append(players, snapshotPlayer{
			ID:          player.ID,
			Name:        player.Name,
			X:           player.X,
			Y:           player.Y,
			VX:          player.VX,
			VY:          player.VY,
			Mass:        player.Mass,
			Radius:      radiusForMass(player.Mass),
			Angle:       player.Angle,
			Health:      player.Health,
			IsAlive:     player.Alive,
			RespawnInMs: respawnIn,
			IsBot:       player.IsBot,
			Color:       player.Color,
		})
	}
	return players
}

func (s *Server) snapshotProjectilesLocked() []snapshotShot {
	shots := make([]snapshotShot, 0, len(s.lobby.Projectiles))
	for _, projectile := range s.lobby.Projectiles {
		shots = append(shots, snapshotShot{
			ID:      projectile.ID,
			X:       projectile.X,
			Y:       projectile.Y,
			Radius:  projectile.Radius,
			OwnerID: projectile.OwnerID,
			Color:   projectile.Color,
		})
	}
	return shots
}

func buildSelfState(player *Player, now time.Time) *selfState {
	respawnIn := int64(0)
	if !player.RespawnAt.IsZero() && now.Before(player.RespawnAt) {
		respawnIn = player.RespawnAt.Sub(now).Milliseconds()
	}

	return &selfState{
		PlayerID:    player.ID,
		PlayerName:  player.Name,
		Score:       player.Score,
		Mass:        player.Mass,
		Health:      player.Health,
		Kills:       player.Kills,
		IsAlive:     player.Alive,
		RespawnInMs: respawnIn,
		DeathReason: player.DeathReason,
		KilledBy:    player.KilledBy,
	}
}

func (s *Server) parseToken(tokenString string) (*matchClaims, error) {
	claims := &matchClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims.LobbyID != s.cfg.LobbyID {
		return nil, fmt.Errorf("unexpected lobby")
	}
	return claims, nil
}

func (connection *ClientConnection) writeJSON(payload any) error {
	connection.Mu.Lock()
	defer connection.Mu.Unlock()

	_ = connection.Socket.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return connection.Socket.WriteJSON(payload)
}

func radiusForMass(mass float64) float64 {
	return math.Sqrt(math.Max(mass, 1)) * playerRadiusScale
}

func randomID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

func sanitizeName(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "Pilot"
	}
	if len(trimmed) > 18 {
		return trimmed[:18]
	}
	return trimmed
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (s *Server) randFloat(min, max float64) float64 {
	return min + s.rng.Float64()*(max-min)
}

func (s *Server) randomColor() string {
	return palette[s.rng.Intn(len(palette))]
}

func maxInt64(value, min int64) int64 {
	if value < min {
		return min
	}
	return value
}

func signPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
