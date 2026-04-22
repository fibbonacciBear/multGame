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
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

const (
	defaultWorldWidth        = 4000
	defaultWorldHeight       = 4000
	defaultGravityAccel      = 2400
	defaultDrag              = 0.98
	defaultTerminalSpeed     = 900
	defaultPlayerRadiusScale = 3
	defaultNumObjects        = 200
	defaultStartingMass      = 10
	defaultStartingHealth    = 100
	defaultHealthBase        = 100
	defaultHealthScale       = 25
	defaultHealthMassScale   = 10
	defaultRadiusBase        = 10
	defaultRadiusScale       = 6
	defaultRadiusMassScale   = 10
	defaultProjectileSpeed   = 1250
	defaultProjectileDamage  = 28
	defaultProjectileRadius  = 5
	defaultShootCooldown     = 250 * time.Millisecond
	defaultProjectileTTL     = 1200 * time.Millisecond
	defaultCrashDamagePct    = 0.9
	defaultCrashKnockback    = 250
	defaultCrashPairCooldown = 500 * time.Millisecond
	defaultKillMassTransfer  = 0.45
	defaultKillHealPct       = 0.2
	defaultRespawnRetention  = 0.45
	defaultSpawnInvuln       = time.Second
	defaultSpawnAttempts     = 20
	defaultPassiveHealPerSec = 2.5
	defaultPassiveHealDelay  = 1500 * time.Millisecond
	defaultDisconnectGrace   = 10 * time.Second
	defaultIntermission      = 10 * time.Second
	projectileTypeRailgun    = "railgun"
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
	Port                         string
	JWTSecret                    string
	ReportSecret                 string
	APIServerURL                 string
	RedisAddr                    string
	RedisPassword                string
	RedisDB                      int
	PodIP                        string
	LobbyID                      string
	TickRate                     int
	SnapshotRate                 int
	MaxPlayers                   int
	WorldWidth                   float64
	WorldHeight                  float64
	GravityAccel                 float64
	Drag                         float64
	TerminalSpeed                float64
	PlayerRadiusScale            float64
	RadiusBase                   float64
	RadiusScale                  float64
	RadiusMassScale              float64
	NumObjects                   int
	StartingMass                 float64
	StartingHealth               float64
	HealthBase                   float64
	HealthScale                  float64
	HealthMassScale              float64
	ProjectileSpeed              float64
	ProjectileDamage             float64
	ProjectileRadius             float64
	ShootCooldown                time.Duration
	ProjectileTTL                time.Duration
	CrashDamagePct               float64
	CrashPairCooldown            time.Duration
	CrashKnockbackImpulse        float64
	KillMassTransferPct          float64
	KillHealPct                  float64
	RespawnRetentionPct          float64
	SpawnInvulnerabilityDuration time.Duration
	SpawnClearanceAttempts       int
	PassiveHealPerSecond         float64
	PassiveHealCombatDelay       time.Duration
	BotDifficultyMode            string
	BotDifficultyDistribution    string
	BotDifficultyProfiles        string
	BotDifficultyAdaptiveLow     string
	BotDifficultyAdaptiveHigh    string
	MatchDuration                time.Duration
	RespawnDelay                 time.Duration
	IntermissionDuration         time.Duration
	DisconnectGracePeriod        time.Duration
	BotFillDelay                 time.Duration
	MaxSpectators                int
	DebugSpectatorGracePeriod    time.Duration
	RegistryHeartbeatInterval    time.Duration
	RegistryHeartbeatTTL         time.Duration
	LobbyTTL                     time.Duration
	HealthTickThreshold          time.Duration
	ShutdownDrainTimeout         time.Duration
	MatchAnalyticsEnabled        bool
	MatchAnalyticsReportRetries  int
	MatchAnalyticsRetryDelay     time.Duration
}

func LoadConfig() Config {
	startingMass := envFloat("STARTING_MASS", defaultStartingMass)
	if os.Getenv("HEALTH_BASE") == "" && os.Getenv("STARTING_HEALTH") != "" {
		log.Printf("warning: STARTING_HEALTH is deprecated; use HEALTH_BASE instead")
	}
	healthBase := envFloat("HEALTH_BASE", envFloat("STARTING_HEALTH", defaultHealthBase))
	radiusBase := envFloat("RADIUS_BASE", 0)
	if radiusBase <= 0 {
		if os.Getenv("PLAYER_RADIUS_SCALE") != "" {
			log.Printf("warning: PLAYER_RADIUS_SCALE is deprecated; use RADIUS_BASE/RADIUS_SCALE/RADIUS_MASS_SCALE instead")
		}
		legacyScale := envFloat("PLAYER_RADIUS_SCALE", defaultPlayerRadiusScale)
		radiusBase = math.Sqrt(math.Max(startingMass, 1)) * legacyScale
	}

	cfg := Config{
		Port:                         envOrDefault("PORT", "8080"),
		JWTSecret:                    envOrDefault("JWT_SECRET", "dev-secret"),
		ReportSecret:                 envOrDefault("REPORT_SHARED_SECRET", envOrDefault("JWT_SECRET", "dev-secret")),
		APIServerURL:                 envOrDefault("API_SERVER_URL", "http://api-server:8081"),
		RedisAddr:                    envOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:                os.Getenv("REDIS_PASSWORD"),
		RedisDB:                      envInt("REDIS_DB", 0),
		PodIP:                        defaultPodIP(),
		LobbyID:                      defaultLobbyID(),
		TickRate:                     envInt("TICK_RATE", 60),
		SnapshotRate:                 envInt("SNAPSHOT_RATE", 20),
		MaxPlayers:                   envInt("MAX_PLAYERS", 10),
		WorldWidth:                   envFloat("WORLD_WIDTH", defaultWorldWidth),
		WorldHeight:                  envFloat("WORLD_HEIGHT", defaultWorldHeight),
		GravityAccel:                 envFloat("GRAVITY_ACCEL", defaultGravityAccel),
		Drag:                         envFloat("DRAG", defaultDrag),
		TerminalSpeed:                envFloat("TERMINAL_SPEED", defaultTerminalSpeed),
		PlayerRadiusScale:            envFloat("PLAYER_RADIUS_SCALE", defaultPlayerRadiusScale),
		RadiusBase:                   radiusBase,
		RadiusScale:                  envFloat("RADIUS_SCALE", defaultRadiusScale),
		RadiusMassScale:              envFloat("RADIUS_MASS_SCALE", defaultRadiusMassScale),
		NumObjects:                   envInt("NUM_OBJECTS", defaultNumObjects),
		StartingMass:                 startingMass,
		StartingHealth:               envFloat("STARTING_HEALTH", defaultStartingHealth),
		HealthBase:                   healthBase,
		HealthScale:                  envFloat("HEALTH_SCALE", defaultHealthScale),
		HealthMassScale:              envFloat("HEALTH_MASS_SCALE", defaultHealthMassScale),
		ProjectileSpeed:              envFloat("PROJECTILE_SPEED", defaultProjectileSpeed),
		ProjectileDamage:             envFloat("PROJECTILE_DAMAGE", defaultProjectileDamage),
		ProjectileRadius:             envFloat("PROJECTILE_RADIUS", defaultProjectileRadius),
		ShootCooldown:                envDuration("SHOOT_COOLDOWN", defaultShootCooldown),
		ProjectileTTL:                envDuration("PROJECTILE_TTL", defaultProjectileTTL),
		CrashDamagePct:               envFloat("CRASH_DAMAGE_PCT", defaultCrashDamagePct),
		CrashPairCooldown:            envDuration("CRASH_PAIR_COOLDOWN", defaultCrashPairCooldown),
		CrashKnockbackImpulse:        envFloat("CRASH_KNOCKBACK_IMPULSE", defaultCrashKnockback),
		KillMassTransferPct:          envFloat("KILL_MASS_TRANSFER_PCT", defaultKillMassTransfer),
		KillHealPct:                  envFloat("KILL_HEAL_PCT", defaultKillHealPct),
		RespawnRetentionPct:          envFloat("RESPAWN_RETENTION_PCT", defaultRespawnRetention),
		SpawnInvulnerabilityDuration: envDuration("SPAWN_INVULNERABILITY_DURATION", defaultSpawnInvuln),
		SpawnClearanceAttempts:       envInt("SPAWN_CLEARANCE_ATTEMPTS", defaultSpawnAttempts),
		PassiveHealPerSecond:         envFloat("PASSIVE_HEAL_PER_SECOND", defaultPassiveHealPerSec),
		PassiveHealCombatDelay:       envDuration("PASSIVE_HEAL_COMBAT_DELAY", defaultPassiveHealDelay),
		BotDifficultyMode:            envOrDefault("BOT_DIFFICULTY_MODE", "weighted"),
		BotDifficultyDistribution:    envOrDefault("BOT_DIFFICULTY_DISTRIBUTION", "L0:10,L1:30,L2:40,L3:20"),
		BotDifficultyProfiles:        os.Getenv("BOT_DIFFICULTY_PROFILES"),
		BotDifficultyAdaptiveLow:     envOrDefault("BOT_DIFFICULTY_ADAPTIVE_LOW", string(BotLevelEvasive)),
		BotDifficultyAdaptiveHigh:    envOrDefault("BOT_DIFFICULTY_ADAPTIVE_HIGH", string(BotLevelFull)),
		MatchDuration:                envDuration("MATCH_DURATION", 5*time.Minute),
		RespawnDelay:                 envDuration("RESPAWN_DELAY", 2*time.Second),
		IntermissionDuration:         envDuration("INTERMISSION_DURATION", defaultIntermission),
		DisconnectGracePeriod:        envDuration("DISCONNECT_GRACE_PERIOD", defaultDisconnectGrace),
		BotFillDelay:                 envDuration("BOT_FILL_DELAY", 0),
		MaxSpectators:                envInt("MAX_SPECTATORS", defaultMaxSpectators),
		DebugSpectatorGracePeriod:    envDuration("DEBUG_SPECTATOR_GRACE_PERIOD", defaultDebugSpectatorGrace),
		RegistryHeartbeatInterval:    envDuration("REGISTRY_HEARTBEAT_INTERVAL", 5*time.Second),
		RegistryHeartbeatTTL:         registryHeartbeatTTL(),
		LobbyTTL:                     envDuration("LOBBY_TTL", 330*time.Second),
		HealthTickThreshold:          envDuration("HEALTH_TICK_THRESHOLD", 2*time.Second),
		ShutdownDrainTimeout:         envDuration("SHUTDOWN_DRAIN_TIMEOUT", 30*time.Second),
		MatchAnalyticsEnabled:        envBool("MATCH_ANALYTICS_ENABLED", false),
		MatchAnalyticsReportRetries:  envInt("MATCH_ANALYTICS_REPORT_RETRIES", 2),
		MatchAnalyticsRetryDelay:     envDuration("MATCH_ANALYTICS_RETRY_DELAY", 500*time.Millisecond),
	}
	return normalizeConfig(cfg)
}

type Server struct {
	cfg      Config
	logger   *log.Logger
	upgrader websocket.Upgrader

	mu         sync.RWMutex
	lobby      *Lobby
	spectators map[string]*ClientConnection
	draining   bool

	rng          *mathrand.Rand
	matchRNG     *mathrand.Rand
	botProfiles  map[BotLevel]botProfile
	httpClient   *http.Client
	redis        *redis.Client
	lastTickAt   atomic.Int64
	matchMetrics *MatchMetricsCollector
}

type LobbyPhase string

const (
	phaseIdle         LobbyPhase = "idle"
	phaseActive       LobbyPhase = "active"
	phaseIntermission LobbyPhase = "intermission"
)

type Lobby struct {
	ID                       string
	MatchID                  string
	MatchKind                MatchKind
	Phase                    LobbyPhase
	MatchStart               time.Time
	MatchEnds                time.Time
	IntermissionEnds         time.Time
	MatchOver                bool
	DebugSessionID           string
	DebugBotCount            int
	DebugSpectatorGraceUntil time.Time

	Players     map[string]*Player
	CrashPairs  map[string]time.Time
	Objects     []*Collectible
	Projectiles []*Projectile
	KillFeed    []KillFeedEntry
}

type Player struct {
	ID                          string
	Name                        string
	Color                       string
	SpriteVariant               int
	IsBot                       bool
	Connected                   bool
	Connection                  *ClientConnection
	DisconnectedAt              time.Time
	X                           float64
	Y                           float64
	VX                          float64
	VY                          float64
	Mass                        float64
	Health                      float64
	Angle                       float64
	Alive                       bool
	RespawnAt                   time.Time
	LastShotAt                  time.Time
	Input                       InputState
	Score                       int
	Kills                       int
	DeathReason                 string
	KilledBy                    string
	BotTargetX                  float64
	BotTargetY                  float64
	BotCollectibleTargetID      string
	BotCollectibleCooldownUntil time.Time
	BotCornerRecovering         bool
	BotRetargetAt               time.Time
	BotLastProgressAt           time.Time
	BotLastTargetDistance       float64
	BotLevel                    BotLevel
	LastCombatAt                time.Time
	PreDeathMass                float64
	SpawnInvulnerableUntil      time.Time
	PendingSpawnSeparation      bool
	LastPickupFeedbackSeq       int64
	LastPickupMassGain          float64
	LastPickupHealthGain        float64
}

type ClientConnection struct {
	ViewerID       string
	PlayerID       string
	SessionMode    SessionMode
	DebugSessionID string
	Socket         *websocket.Conn
	Mu             sync.Mutex
}

type Collectible struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Radius float64 `json:"radius"`
	Mass   float64 `json:"mass"`
}

type Projectile struct {
	ID        string
	OwnerID   string
	Type      string
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
	PlayerName           string      `json:"name"`
	LobbyID              string      `json:"lobby_id"`
	PodIP                string      `json:"pod_ip"`
	SessionMode          SessionMode `json:"session_mode"`
	DebugSimulationStart bool        `json:"debug_simulation_start"`
	DebugSessionID       string      `json:"debug_session_id"`
	DebugBotCount        int         `json:"debug_bot_count"`
	DebugSeed            *int64      `json:"debug_seed"`
	jwt.RegisteredClaims
}

type welcomeMessage struct {
	Type           string      `json:"type"`
	SessionMode    SessionMode `json:"sessionMode"`
	ViewerID       string      `json:"viewerId"`
	PlayerID       string      `json:"playerId,omitempty"`
	LobbyID        string      `json:"lobbyId"`
	MatchID        string      `json:"matchId"`
	Phase          LobbyPhase  `json:"phase"`
	MatchKind      MatchKind   `json:"matchKind"`
	CameraTargetID string      `json:"cameraTargetId,omitempty"`
	DebugSessionID string      `json:"debugSessionId,omitempty"`
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
	Phase           LobbyPhase         `json:"phase"`
	MatchKind       MatchKind          `json:"matchKind"`
	DebugSessionID  string             `json:"debugSessionId,omitempty"`
	MatchOver       bool               `json:"matchOver"`
	TimeRemainingMs int64              `json:"timeRemainingMs"`
	IntermissionMs  int64              `json:"intermissionRemainingMs"`
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
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	SpriteVariant int     `json:"spriteVariant"`
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	VX            float64 `json:"vx"`
	VY            float64 `json:"vy"`
	Mass          float64 `json:"mass"`
	Radius        float64 `json:"radius"`
	Angle         float64 `json:"angle"`
	Health        float64 `json:"health"`
	MaxHealth     float64 `json:"maxHealth"`
	IsAlive       bool    `json:"isAlive"`
	RespawnInMs   int64   `json:"respawnInMs"`
	IsBot         bool    `json:"isBot"`
	Color         string  `json:"color"`
}

type snapshotShot struct {
	ID      string  `json:"id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	VX      float64 `json:"vx"`
	VY      float64 `json:"vy"`
	Radius  float64 `json:"radius"`
	OwnerID string  `json:"ownerId"`
	Type    string  `json:"type"`
	Color   string  `json:"color"`
}

type pickupFeedbackState struct {
	Sequence   int64   `json:"sequence"`
	MassGain   float64 `json:"massGain"`
	HealthGain float64 `json:"healthGain"`
}

type selfState struct {
	PlayerID       string               `json:"playerId"`
	PlayerName     string               `json:"playerName"`
	Score          int                  `json:"score"`
	Mass           float64              `json:"mass"`
	Health         float64              `json:"health"`
	MaxHealth      float64              `json:"maxHealth"`
	Kills          int                  `json:"kills"`
	IsAlive        bool                 `json:"isAlive"`
	RespawnInMs    int64                `json:"respawnInMs"`
	DeathReason    string               `json:"deathReason,omitempty"`
	KilledBy       string               `json:"killedBy,omitempty"`
	PickupFeedback *pickupFeedbackState `json:"pickupFeedback,omitempty"`
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
	cfg = normalizeConfig(cfg)
	botProfiles, err := loadBotProfiles(cfg.BotDifficultyProfiles)
	if err != nil {
		logger.Printf("invalid BOT_DIFFICULTY_PROFILES: %v; using built-in profiles", err)
		botProfiles = defaultBotProfiles()
	}

	server := &Server{
		cfg:    cfg,
		logger: logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		lobby: &Lobby{
			ID:         cfg.LobbyID,
			MatchID:    randomID("match"),
			MatchKind:  matchKindNormal,
			Phase:      phaseIdle,
			MatchOver:  true,
			Players:    make(map[string]*Player),
			CrashPairs: make(map[string]time.Time),
			Objects:    make([]*Collectible, 0, cfg.NumObjects),
		},
		spectators:  make(map[string]*ClientConnection),
		rng:         mathrand.New(mathrand.NewSource(time.Now().UnixNano())),
		botProfiles: botProfiles,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
	}
	server.initRegistryClient()
	server.lastTickAt.Store(now.UnixNano())
	return server
}

func (s *Server) Start(ctx context.Context) {
	go s.startRegistryLoop(ctx)
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
	if age := s.tickAge(time.Now()); age > s.cfg.HealthTickThreshold {
		http.Error(w, "game loop unhealthy", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) HandleReadyz(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.draining || s.connectedGameplayHumansLocked() >= s.cfg.MaxPlayers {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) updateGauges() {
	humans := 0
	total := 0
	for _, player := range s.lobby.Players {
		total++
		if !player.IsBot && player.Connected {
			humans++
		}
		if player.Alive && s.shouldCollectGameplayMetricsLocked() {
			PlayerMass.Observe(player.Mass)
		}
	}
	ActivePlayers.Set(float64(humans))
	LobbyPlayerCount.Set(float64(total))
	SpectatorConnections.Set(float64(s.connectedSpectatorsLocked()))
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
	lobbyID := r.URL.Query().Get("lobby")
	if lobbyID == "" {
		lobbyID = s.cfg.LobbyID
	}
	claims, err := s.parseToken(tokenString, lobbyID)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if strings.TrimSpace(claims.Subject) == "" {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	mode := normalizeSessionMode(claims.SessionMode)

	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		http.Error(w, "server draining", http.StatusServiceUnavailable)
		return
	}
	switch mode {
	case sessionModePlayer:
		if s.lobby.MatchKind == matchKindDebugBotSim {
			s.mu.Unlock()
			http.Error(w, "debug simulation active", http.StatusServiceUnavailable)
			return
		}
		if !s.canAdmitHumanLocked(claims.Subject) {
			s.mu.Unlock()
			http.Error(w, "lobby full", http.StatusServiceUnavailable)
			return
		}
	case sessionModeSpectator:
		if s.lobby.MatchKind == matchKindDebugBotSim {
			s.mu.Unlock()
			http.Error(w, "debug session unavailable", http.StatusServiceUnavailable)
			return
		}
		if !s.canAdmitSpectatorLocked(claims.Subject) {
			s.mu.Unlock()
			http.Error(w, "spectator lobby full", http.StatusServiceUnavailable)
			return
		}
	case sessionModeDebugSimulation:
		if claims.DebugSessionID == "" {
			s.mu.Unlock()
			http.Error(w, "debug session unavailable", http.StatusUnauthorized)
			return
		}
		if !s.canAdmitSpectatorLocked(claims.Subject) {
			s.mu.Unlock()
			http.Error(w, "spectator lobby full", http.StatusServiceUnavailable)
			return
		}
	default:
		s.mu.Unlock()
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	s.mu.Unlock()

	socket, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Printf("websocket upgrade failed: %v", err)
		return
	}

	connection := &ClientConnection{
		ViewerID:       claims.Subject,
		PlayerID:       claims.Subject,
		SessionMode:    mode,
		DebugSessionID: claims.DebugSessionID,
		Socket:         socket,
	}

	s.mu.Lock()
	now := time.Now()
	if s.draining {
		s.mu.Unlock()
		s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "server draining")
		return
	}
	welcome := welcomeMessage{
		Type:           "welcome",
		SessionMode:    mode,
		ViewerID:       connection.ViewerID,
		LobbyID:        s.lobby.ID,
		MatchID:        s.lobby.MatchID,
		Phase:          s.lobby.Phase,
		MatchKind:      s.lobby.MatchKind,
		DebugSessionID: s.lobby.DebugSessionID,
	}
	switch mode {
	case sessionModePlayer:
		if s.lobby.MatchKind == matchKindDebugBotSim {
			s.mu.Unlock()
			s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "debug simulation active")
			return
		}
		if !s.canAdmitHumanLocked(claims.Subject) {
			s.mu.Unlock()
			s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "lobby full")
			return
		}
		s.clearDebugMatchStateLocked()
		if s.lobby.Phase == phaseIdle {
			s.resetMatchLocked(now)
		}
		player := s.upsertHumanPlayerLocked(claims.Subject, claims.PlayerName, connection, now)
		s.registerMatchParticipantLocked(player, now)
		connection.PlayerID = player.ID
		welcome.PlayerID = player.ID
		welcome.MatchID = s.lobby.MatchID
		welcome.Phase = s.lobby.Phase
		welcome.MatchKind = s.lobby.MatchKind
		welcome.CameraTargetID = player.ID
	case sessionModeSpectator:
		if s.lobby.MatchKind == matchKindDebugBotSim {
			s.mu.Unlock()
			s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "debug session unavailable")
			return
		}
		if !s.canAdmitSpectatorLocked(connection.ViewerID) {
			s.mu.Unlock()
			s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "spectator lobby full")
			return
		}
		s.registerSpectatorLocked(connection)
		welcome.CameraTargetID = s.chooseDefaultCameraTargetLocked(false)
	case sessionModeDebugSimulation:
		if !s.canAdmitSpectatorLocked(connection.ViewerID) {
			s.mu.Unlock()
			s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "spectator lobby full")
			return
		}
		if claims.DebugSimulationStart {
			if s.lobby.Phase != phaseIdle || s.lobby.MatchKind == matchKindDebugBotSim {
				s.mu.Unlock()
				s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "debug lobby unavailable")
				return
			}
			s.startDebugMatchLocked(now, claims.DebugSessionID, claims.DebugBotCount, claims.DebugSeed)
		} else if s.lobby.MatchKind != matchKindDebugBotSim || s.lobby.DebugSessionID != claims.DebugSessionID {
			s.mu.Unlock()
			s.closeSocketWithReason(socket, websocket.CloseTryAgainLater, "debug session unavailable")
			return
		}
		s.registerSpectatorLocked(connection)
		welcome.MatchID = s.lobby.MatchID
		welcome.Phase = s.lobby.Phase
		welcome.MatchKind = s.lobby.MatchKind
		welcome.DebugSessionID = s.lobby.DebugSessionID
		welcome.CameraTargetID = s.chooseDefaultCameraTargetLocked(true)
	default:
		s.mu.Unlock()
		s.closeSocketWithReason(socket, websocket.ClosePolicyViolation, "invalid session mode")
		return
	}
	s.mu.Unlock()
	WSConnectionsOpened.Inc()
	s.scheduleRegistryRefresh()

	_ = connection.writeJSON(welcome)

	go s.readLoop(connection)
}

func (s *Server) BeginDrain(message string) {
	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		return
	}
	s.draining = true
	if s.lobby.Phase == phaseIntermission {
		s.lobby.IntermissionEnds = time.Time{}
	}
	if s.connectedGameplayHumansLocked() == 0 {
		s.markDrainCompleteLocked(time.Now())
	}
	connections := s.allConnectionsForNoticeCloseLocked()
	s.mu.Unlock()

	_ = s.markDraining(context.Background())

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

	for _, connection := range s.allConnectionsForNoticeCloseLocked() {
		_ = connection.Socket.Close()
	}
}

func (s *Server) readLoop(connection *ClientConnection) {
	defer func() {
		WSConnectionsClosed.Inc()
		s.mu.Lock()
		now := time.Now()
		if connection.SessionMode == sessionModePlayer {
			if player, ok := s.lobby.Players[connection.PlayerID]; ok {
				if player.Connection == connection {
					if s.matchMetrics != nil {
						s.matchMetrics.OnDisconnect(player, now)
					}
					player.Connected = false
					player.Connection = nil
					player.DisconnectedAt = now
					player.Input = InputState{}
				}
			}
		} else {
			s.removeSpectatorLocked(connection, now)
		}
		s.mu.Unlock()
		s.scheduleRegistryRefresh()
		_ = connection.Socket.Close()
	}()

	for {
		if connection.SessionMode != sessionModePlayer {
			if _, _, err := connection.Socket.ReadMessage(); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && err != io.EOF {
					s.logger.Printf("read failed for spectator %s: %v", connection.ViewerID, err)
				}
				return
			}
			WSMessagesReceived.Inc()
			continue
		}

		var input InputState
		if err := connection.Socket.ReadJSON(&input); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) && err != io.EOF {
				s.logger.Printf("read failed for %s: %v", connection.PlayerID, err)
			}
			return
		}
		WSMessagesReceived.Inc()

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
	start := time.Now()
	defer func() { TickDuration.Observe(time.Since(start).Seconds()) }()

	s.tickHealth(now)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.evictDisconnectedPlayersLocked(now)
	s.syncDebugSpectatorGraceLocked(now)
	if s.draining && s.connectedGameplayHumansLocked() == 0 {
		s.markDrainCompleteLocked(now)
		s.updateGauges()
		return
	}
	if s.lobby.MatchKind == matchKindDebugBotSim &&
		s.connectedDebugSpectatorsLocked() == 0 &&
		!s.lobby.DebugSpectatorGraceUntil.IsZero() &&
		!now.Before(s.lobby.DebugSpectatorGraceUntil) {
		s.finalizeMatchAnalyticsLocked(matchEndReasonDebugAbandoned, now, false)
		s.transitionToIdleLocked()
		s.updateGauges()
		return
	}
	if !s.draining &&
		s.connectedGameplayHumansLocked() == 0 &&
		(s.lobby.Phase == phaseActive || s.lobby.Phase == phaseIntermission) &&
		!s.shouldKeepObservedSessionAliveLocked(now) {
		s.finalizeMatchAnalyticsLocked(matchEndReasonNoHumans, now, false)
		s.transitionToIdleLocked()
		s.updateGauges()
		return
	}

	switch s.lobby.Phase {
	case phaseActive:
		s.fillBotsLocked(now)
		s.updateBotsLocked(now)
		s.updatePlayersLocked(now)
		s.resolveExpiredSpawnSeparationsLocked(now)
		s.updateProjectilesLocked(now)
		s.resolveObjectCollisionsLocked(now)
		s.resolveCombatLocked(now)
		s.applyPassiveHealingLocked(now)
		if now.After(s.lobby.MatchEnds) {
			scoreboard := s.scoreboardLocked()
			s.sampleMatchAnalyticsLocked(now)
			s.finalizeMatchAnalyticsLocked(matchEndReasonTimeLimit, now, s.draining)
			s.finishMatchLocked(now)
			if s.lobby.MatchKind != matchKindDebugBotSim {
				go s.reportLeaderboard(scoreboard)
			}
			if s.draining {
				s.lobby.IntermissionEnds = time.Time{}
			}
		}
		s.handleRespawnsLocked(now)
		s.sampleMatchAnalyticsLocked(now)
	case phaseIntermission:
		if !s.draining && !s.lobby.IntermissionEnds.IsZero() && !now.Before(s.lobby.IntermissionEnds) {
			if s.connectedGameplayHumansLocked() > 0 {
				if s.lobby.MatchKind != matchKindDebugBotSim {
					s.clearDebugMatchStateLocked()
				}
				s.resetMatchLocked(now)
			} else {
				s.finalizeMatchAnalyticsLocked(matchEndReasonNoHumans, now, false)
				s.transitionToIdleLocked()
			}
		}
	}

	s.updateGauges()
}

type snapshotDelivery struct {
	connection *ClientConnection
	self       *selfState
}

func (s *Server) broadcastSnapshots(now time.Time) {
	broadcastStart := time.Now()
	defer func() { SnapshotBroadcastDuration.Observe(time.Since(broadcastStart).Seconds()) }()

	s.mu.RLock()
	deliveries := make([]snapshotDelivery, 0, len(s.lobby.Players)+len(s.spectators))
	for _, player := range s.lobby.Players {
		if player.Connected && player.Connection != nil {
			deliveries = append(deliveries, snapshotDelivery{
				connection: player.Connection,
				self:       s.buildSelfState(player, now),
			})
		}
	}
	for _, connection := range s.spectators {
		deliveries = append(deliveries, snapshotDelivery{
			connection: connection,
		})
	}

	scoreboard := s.scoreboardLocked()
	messageBase := snapshotMessage{
		Type:            "snapshot",
		ServerTime:      now.UnixMilli(),
		World:           worldBounds{Width: s.cfg.WorldWidth, Height: s.cfg.WorldHeight},
		MatchID:         s.lobby.MatchID,
		Phase:           s.lobby.Phase,
		MatchKind:       s.lobby.MatchKind,
		DebugSessionID:  s.lobby.DebugSessionID,
		MatchOver:       s.lobby.MatchOver,
		TimeRemainingMs: s.matchTimeRemainingLocked(now),
		IntermissionMs:  s.intermissionRemainingLocked(now),
		Players:         s.snapshotPlayersLocked(now),
		Objects:         append([]*Collectible(nil), s.lobby.Objects...),
		Projectiles:     s.snapshotProjectilesLocked(),
		KillFeed:        append([]KillFeedEntry{}, s.lobby.KillFeed...),
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
	if s.lobby.MatchKind == matchKindDebugBotSim {
		s.maintainDebugBotCountLocked(now)
		return
	}

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
		s.registerMatchParticipantLocked(bot, now)
		total++
	}
}

func (s *Server) updatePlayersLocked(now time.Time) {
	dt := 1.0 / float64(s.cfg.TickRate)

	for _, player := range s.lobby.Players {
		if !player.Alive {
			continue
		}

		player.Angle = player.Input.Angle
		accel := s.cfg.GravityAccel * clamp(player.Input.Strength, 0, 1) * dt
		player.VX += math.Cos(player.Input.Angle) * accel
		player.VY += math.Sin(player.Input.Angle) * accel
		player.VX *= s.cfg.Drag
		player.VY *= s.cfg.Drag

		speed := math.Hypot(player.VX, player.VY)
		if speed > s.cfg.TerminalSpeed {
			scale := s.cfg.TerminalSpeed / speed
			player.VX *= scale
			player.VY *= scale
		}

		player.X += player.VX * dt
		player.Y += player.VY * dt
		s.clampPlayerToWorldLocked(player)

		if player.Input.Shoot && !s.isInvulnerable(player, now) && now.Sub(player.LastShotAt) >= s.cfg.ShootCooldown {
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
		if projectile.X < 0 || projectile.X > s.cfg.WorldWidth || projectile.Y < 0 || projectile.Y > s.cfg.WorldHeight {
			continue
		}

		projectiles = append(projectiles, projectile)
	}

	s.lobby.Projectiles = projectiles
}

func (s *Server) finishMatchLocked(now time.Time) {
	s.lobby.Phase = phaseIntermission
	s.lobby.MatchOver = true
	s.lobby.IntermissionEnds = now.Add(s.cfg.IntermissionDuration)
	if s.lobby.MatchKind != matchKindDebugBotSim {
		MatchesCompleted.Inc()
	}
	s.scheduleRegistryRefresh()
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

func (s *Server) activeSlotsLocked() int {
	total := 0
	for _, player := range s.lobby.Players {
		if player.Connected || player.IsBot {
			total++
		}
	}
	return total
}

func (s *Server) canAdmitHumanLocked(id string) bool {
	if s.lobby.MatchKind == matchKindDebugBotSim {
		return false
	}
	player, ok := s.lobby.Players[id]
	if ok && !player.IsBot {
		return true
	}
	return s.connectedGameplayHumansLocked() < s.cfg.MaxPlayers
}

func (s *Server) closeSocketWithReason(socket *websocket.Conn, code int, reason string) {
	deadline := time.Now().Add(250 * time.Millisecond)
	_ = socket.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), deadline)
	_ = socket.Close()
}

func (s *Server) upsertHumanPlayerLocked(id, name string, connection *ClientConnection, now time.Time) *Player {
	player, ok := s.lobby.Players[id]
	spawnedNow := false
	usedSpawnFallback := false
	if !ok {
		if s.activeSlotsLocked() >= s.cfg.MaxPlayers {
			s.removeOneBotLocked()
		}
		player = &Player{
			ID:            id,
			Name:          sanitizeName(name),
			Color:         s.randomColor(),
			SpriteVariant: s.randomSpriteVariant(),
		}
		player.Mass = s.cfg.StartingMass
		player.Health = s.maxHealthForMass(player.Mass)
		player.Alive = true
		usedSpawnFallback = s.spawnPlayerAtRandomPositionLocked(player)
		spawnedNow = true
		s.lobby.Players[player.ID] = player
	}

	player.Name = sanitizeName(name)
	player.Connected = true
	player.Connection = connection
	player.DisconnectedAt = time.Time{}
	player.IsBot = false
	player.Alive = true
	if player.Mass < s.cfg.StartingMass {
		player.Mass = s.cfg.StartingMass
	}
	if player.Health <= 0 {
		player.Health = s.maxHealthForMass(player.Mass)
	}
	if player.X == 0 && player.Y == 0 {
		usedSpawnFallback = s.spawnPlayerAtRandomPositionLocked(player)
		spawnedNow = true
	}
	player.RespawnAt = time.Time{}
	player.DeathReason = ""
	player.KilledBy = ""
	if spawnedNow {
		s.applySpawnSafetyLocked(player, now, usedSpawnFallback)
	}
	player.LastShotAt = now.Add(-s.cfg.ShootCooldown)

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

func (s *Server) transitionToIdleLocked() {
	s.lobby.Phase = phaseIdle
	s.lobby.MatchOver = true
	s.lobby.MatchStart = time.Time{}
	s.lobby.MatchEnds = time.Time{}
	s.lobby.IntermissionEnds = time.Time{}
	s.lobby.Projectiles = nil
	s.lobby.CrashPairs = make(map[string]time.Time)
	s.lobby.KillFeed = s.lobby.KillFeed[:0]
	s.lobby.Objects = s.lobby.Objects[:0]
	s.clearDebugMatchStateLocked()

	for id, player := range s.lobby.Players {
		if player.IsBot || !player.Connected {
			delete(s.lobby.Players, id)
		}
	}
	s.scheduleRegistryRefresh()
}

func (s *Server) markDrainCompleteLocked(now time.Time) {
	s.finalizeMatchAnalyticsLocked(matchEndReasonDrain, now, true)
	if s.connectedGameplayHumansLocked() == 0 {
		s.lobby.Phase = phaseIdle
		s.clearDebugMatchStateLocked()
	} else {
		s.lobby.Phase = phaseIntermission
	}
	s.lobby.MatchOver = true
	s.lobby.IntermissionEnds = time.Time{}
	s.scheduleRegistryRefresh()
}

func (s *Server) spawnObjectsLocked() {
	s.lobby.Objects = s.lobby.Objects[:0]
	for i := 0; i < s.cfg.NumObjects; i++ {
		s.lobby.Objects = append(s.lobby.Objects, s.spawnObjectLocked())
	}
}

func (s *Server) spawnProjectileLocked(player *Player, now time.Time) {
	shot := &Projectile{
		ID:        randomID("shot"),
		OwnerID:   player.ID,
		Type:      projectileTypeRailgun,
		Color:     player.Color,
		X:         player.X + math.Cos(player.Angle)*(s.radiusForMass(player.Mass)+10),
		Y:         player.Y + math.Sin(player.Angle)*(s.radiusForMass(player.Mass)+10),
		VX:        player.VX + math.Cos(player.Angle)*s.cfg.ProjectileSpeed,
		VY:        player.VY + math.Sin(player.Angle)*s.cfg.ProjectileSpeed,
		Radius:    s.cfg.ProjectileRadius,
		Damage:    s.cfg.ProjectileDamage,
		ExpiresAt: now.Add(s.cfg.ProjectileTTL),
	}
	s.lobby.Projectiles = append(s.lobby.Projectiles, shot)
	if s.matchMetrics != nil {
		s.matchMetrics.OnShot(player, now)
	}
}

func (s *Server) applySpawnSafetyLocked(player *Player, now time.Time, usedFallback bool) {
	player.SpawnInvulnerableUntil = now.Add(s.cfg.SpawnInvulnerabilityDuration)
	player.PendingSpawnSeparation = usedFallback
}

func (s *Server) clampPlayerToWorldLocked(player *Player) {
	radius := s.radiusForMass(player.Mass)
	if player.X-radius < 0 {
		player.X = radius
		player.VX = 0
	}
	if player.X+radius > s.cfg.WorldWidth {
		player.X = s.cfg.WorldWidth - radius
		player.VX = 0
	}
	if player.Y-radius < 0 {
		player.Y = radius
		player.VY = 0
	}
	if player.Y+radius > s.cfg.WorldHeight {
		player.Y = s.cfg.WorldHeight - radius
		player.VY = 0
	}
}

func (s *Server) killPlayerLocked(player *Player, killer *Player, reason string, now time.Time) {
	if !player.Alive {
		return
	}

	if reason == "" {
		reason = "destroyed"
	}
	if s.matchMetrics != nil {
		s.matchMetrics.OnKill(player, killer, reason, now)
	}

	player.Alive = false
	player.PreDeathMass = player.Mass
	player.Health = 0
	player.RespawnAt = now.Add(s.cfg.RespawnDelay)
	player.VX = 0
	player.VY = 0
	player.DeathReason = reason
	player.SpawnInvulnerableUntil = time.Time{}
	player.PendingSpawnSeparation = false
	if s.shouldCollectGameplayMetricsLocked() {
		PlayerKills.Inc()
	}
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

func (s *Server) resetMatchLocked(now time.Time) {
	s.lobby.MatchKind = normalizeMatchKind(s.lobby.MatchKind)
	if s.lobby.MatchKind != matchKindDebugBotSim {
		s.clearDebugMatchStateLocked()
	}
	s.lobby.MatchID = randomID("match")
	s.lobby.Phase = phaseActive
	s.lobby.MatchStart = now
	s.lobby.MatchEnds = now.Add(s.cfg.MatchDuration)
	s.lobby.IntermissionEnds = time.Time{}
	s.lobby.MatchOver = false
	s.lobby.Projectiles = nil
	s.lobby.CrashPairs = make(map[string]time.Time)
	s.lobby.KillFeed = s.lobby.KillFeed[:0]
	s.spawnObjectsLocked()
	s.beginMatchAnalyticsLocked(now)

	for id, player := range s.lobby.Players {
		if player.IsBot || !player.Connected {
			delete(s.lobby.Players, id)
			continue
		}
		player.Score = 0
		player.Kills = 0
		player.Mass = s.cfg.StartingMass
		player.Health = s.maxHealthForMass(player.Mass)
		player.Alive = true
		player.RespawnAt = time.Time{}
		player.DeathReason = ""
		player.KilledBy = ""
		player.PreDeathMass = 0
		player.VX = 0
		player.VY = 0
		usedFallback := s.spawnPlayerAtRandomPositionLocked(player)
		s.applySpawnSafetyLocked(player, now, usedFallback)
		s.registerMatchParticipantLocked(player, now)
	}
	s.scheduleRegistryRefresh()
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
			ID:            player.ID,
			Name:          player.Name,
			SpriteVariant: player.SpriteVariant,
			X:             player.X,
			Y:             player.Y,
			VX:            player.VX,
			VY:            player.VY,
			Mass:          player.Mass,
			Radius:        s.radiusForMass(player.Mass),
			Angle:         player.Angle,
			Health:        player.Health,
			MaxHealth:     s.maxHealthForMass(player.Mass),
			IsAlive:       player.Alive,
			RespawnInMs:   respawnIn,
			IsBot:         player.IsBot,
			Color:         player.Color,
		})
	}
	sort.Slice(players, func(i, j int) bool {
		if players[i].IsBot != players[j].IsBot {
			return !players[i].IsBot
		}
		if players[i].IsAlive != players[j].IsAlive {
			return players[i].IsAlive
		}
		if players[i].Name != players[j].Name {
			return players[i].Name < players[j].Name
		}
		return players[i].ID < players[j].ID
	})
	return players
}

func (s *Server) snapshotProjectilesLocked() []snapshotShot {
	shots := make([]snapshotShot, 0, len(s.lobby.Projectiles))
	for _, projectile := range s.lobby.Projectiles {
		projectileType := projectile.Type
		if projectileType == "" {
			projectileType = projectileTypeRailgun
		}
		shots = append(shots, snapshotShot{
			ID:      projectile.ID,
			X:       projectile.X,
			Y:       projectile.Y,
			VX:      projectile.VX,
			VY:      projectile.VY,
			Radius:  projectile.Radius,
			OwnerID: projectile.OwnerID,
			Type:    projectileType,
			Color:   projectile.Color,
		})
	}
	return shots
}

func (s *Server) buildSelfState(player *Player, now time.Time) *selfState {
	respawnIn := int64(0)
	if !player.RespawnAt.IsZero() && now.Before(player.RespawnAt) {
		respawnIn = player.RespawnAt.Sub(now).Milliseconds()
	}

	var pickupFeedback *pickupFeedbackState
	if player.LastPickupFeedbackSeq > 0 {
		pickupFeedback = &pickupFeedbackState{
			Sequence:   player.LastPickupFeedbackSeq,
			MassGain:   player.LastPickupMassGain,
			HealthGain: player.LastPickupHealthGain,
		}
	}

	return &selfState{
		PlayerID:       player.ID,
		PlayerName:     player.Name,
		Score:          player.Score,
		Mass:           player.Mass,
		Health:         player.Health,
		MaxHealth:      s.maxHealthForMass(player.Mass),
		Kills:          player.Kills,
		IsAlive:        player.Alive,
		RespawnInMs:    respawnIn,
		DeathReason:    player.DeathReason,
		KilledBy:       player.KilledBy,
		PickupFeedback: pickupFeedback,
	}
}

func (s *Server) matchTimeRemainingLocked(now time.Time) int64 {
	if s.lobby.Phase != phaseActive || s.lobby.MatchEnds.IsZero() {
		return 0
	}
	return maxInt64(s.lobby.MatchEnds.Sub(now).Milliseconds(), 0)
}

func (s *Server) intermissionRemainingLocked(now time.Time) int64 {
	if s.lobby.Phase != phaseIntermission || s.lobby.IntermissionEnds.IsZero() || !now.Before(s.lobby.IntermissionEnds) {
		return 0
	}
	return s.lobby.IntermissionEnds.Sub(now).Milliseconds()
}

func (s *Server) parseToken(tokenString, expectedLobby string) (*matchClaims, error) {
	claims := &matchClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims.LobbyID != expectedLobby {
		return nil, fmt.Errorf("unexpected lobby")
	}
	if claims.PodIP != "" && s.cfg.PodIP != "" && claims.PodIP != s.cfg.PodIP {
		return nil, fmt.Errorf("unexpected pod")
	}
	claims.SessionMode = normalizeSessionMode(claims.SessionMode)
	return claims, nil
}

func (connection *ClientConnection) writeJSON(payload any) error {
	connection.Mu.Lock()
	defer connection.Mu.Unlock()

	_ = connection.Socket.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return connection.Socket.WriteJSON(payload)
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
	return min + s.rngLocked().Float64()*(max-min)
}

func (s *Server) randomColor() string {
	return palette[s.randomIntnLocked(len(palette))]
}

func (s *Server) randomSpriteVariant() int {
	return int(s.randomInt31Locked())
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

func (s *Server) evictDisconnectedPlayersLocked(now time.Time) {
	grace := s.cfg.DisconnectGracePeriod

	evicted := make(map[string]struct{})
	for id, player := range s.lobby.Players {
		if player.IsBot || player.Connected {
			continue
		}
		if player.DisconnectedAt.IsZero() {
			player.DisconnectedAt = now
		}
		if now.Sub(player.DisconnectedAt) < grace {
			continue
		}
		evicted[id] = struct{}{}
		delete(s.lobby.Players, id)
	}

	if len(evicted) == 0 {
		return
	}

	filteredProjectiles := s.lobby.Projectiles[:0]
	for _, projectile := range s.lobby.Projectiles {
		if _, removed := evicted[projectile.OwnerID]; removed {
			continue
		}
		filteredProjectiles = append(filteredProjectiles, projectile)
	}
	s.lobby.Projectiles = filteredProjectiles

	for key := range s.lobby.CrashPairs {
		ids := strings.SplitN(key, "|", 2)
		if len(ids) != 2 {
			continue
		}
		if _, removed := evicted[ids[0]]; removed {
			delete(s.lobby.CrashPairs, key)
			continue
		}
		if _, removed := evicted[ids[1]]; removed {
			delete(s.lobby.CrashPairs, key)
		}
	}
}

func normalizeConfig(cfg Config) Config {
	if cfg.StartingMass <= 0 {
		cfg.StartingMass = defaultStartingMass
	}
	if cfg.StartingHealth <= 0 {
		cfg.StartingHealth = defaultStartingHealth
	}

	if cfg.HealthBase <= 0 {
		cfg.HealthBase = cfg.StartingHealth
	}
	if cfg.HealthScale == 0 {
		cfg.HealthScale = defaultHealthScale
	}
	if cfg.HealthMassScale <= 0 {
		cfg.HealthMassScale = defaultHealthMassScale
	}

	if cfg.RadiusBase <= 0 {
		if cfg.PlayerRadiusScale > 0 {
			cfg.RadiusBase = math.Sqrt(math.Max(cfg.StartingMass, 1)) * cfg.PlayerRadiusScale
		} else {
			cfg.RadiusBase = defaultRadiusBase
		}
	}
	if cfg.RadiusScale == 0 {
		cfg.RadiusScale = defaultRadiusScale
	}
	if cfg.RadiusMassScale <= 0 {
		cfg.RadiusMassScale = defaultRadiusMassScale
	}

	if cfg.CrashDamagePct <= 0 {
		cfg.CrashDamagePct = defaultCrashDamagePct
	}
	if cfg.CrashPairCooldown <= 0 {
		cfg.CrashPairCooldown = defaultCrashPairCooldown
	}
	if cfg.CrashKnockbackImpulse == 0 {
		cfg.CrashKnockbackImpulse = defaultCrashKnockback
	}
	if cfg.KillMassTransferPct <= 0 {
		cfg.KillMassTransferPct = defaultKillMassTransfer
	}
	if cfg.KillHealPct <= 0 {
		cfg.KillHealPct = defaultKillHealPct
	}
	if cfg.RespawnRetentionPct <= 0 {
		cfg.RespawnRetentionPct = defaultRespawnRetention
	}
	if cfg.SpawnInvulnerabilityDuration <= 0 {
		cfg.SpawnInvulnerabilityDuration = defaultSpawnInvuln
	}
	if cfg.SpawnClearanceAttempts <= 0 {
		cfg.SpawnClearanceAttempts = defaultSpawnAttempts
	}
	if cfg.PassiveHealPerSecond <= 0 {
		cfg.PassiveHealPerSecond = defaultPassiveHealPerSec
	}
	if cfg.PassiveHealCombatDelay < 0 {
		cfg.PassiveHealCombatDelay = 0
	} else if cfg.PassiveHealCombatDelay == 0 {
		cfg.PassiveHealCombatDelay = defaultPassiveHealDelay
	}
	if cfg.BotDifficultyMode == "" {
		cfg.BotDifficultyMode = "weighted"
	}
	if cfg.BotDifficultyDistribution == "" {
		cfg.BotDifficultyDistribution = "L0:10,L1:30,L2:40,L3:20"
	}
	if cfg.BotDifficultyAdaptiveLow == "" {
		cfg.BotDifficultyAdaptiveLow = string(BotLevelEvasive)
	}
	if cfg.BotDifficultyAdaptiveHigh == "" {
		cfg.BotDifficultyAdaptiveHigh = string(BotLevelFull)
	}
	if cfg.IntermissionDuration <= 0 {
		cfg.IntermissionDuration = defaultIntermission
	}
	if cfg.DisconnectGracePeriod <= 0 {
		cfg.DisconnectGracePeriod = defaultDisconnectGrace
	}
	if cfg.MaxSpectators <= 0 {
		cfg.MaxSpectators = defaultMaxSpectators
	}
	if cfg.DebugSpectatorGracePeriod <= 0 {
		cfg.DebugSpectatorGracePeriod = defaultDebugSpectatorGrace
	}
	if cfg.MatchAnalyticsReportRetries < 0 {
		cfg.MatchAnalyticsReportRetries = 0
	}
	if cfg.MatchAnalyticsRetryDelay <= 0 {
		cfg.MatchAnalyticsRetryDelay = 500 * time.Millisecond
	}

	return cfg
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

func envFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
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

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
