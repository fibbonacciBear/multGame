package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

const leaderboardIndexKey = "leaderboard:index"

type Config struct {
	Port                    string
	RedisAddr               string
	RedisPassword           string
	RedisDB                 int
	JWTSecret               string
	ReportSecret            string
	WSRouterBaseURL         string
	PublicGameWSURL         string
	LobbyID                 string
	TokenTTL                time.Duration
	RegistryCleanupInterval time.Duration
}

type Server struct {
	cfg    Config
	logger *log.Logger
	redis  *redis.Client
}

type leaderboardEntry struct {
	PlayerName string `json:"playerName"`
	Kills      int    `json:"kills"`
	MassBonus  int    `json:"massBonus"`
	TotalScore int    `json:"totalScore"`
}

type leaderboardReport struct {
	LobbyID string                   `json:"lobbyId"`
	Results []leaderboardReportEntry `json:"results"`
}

type leaderboardReportEntry struct {
	PlayerID   string  `json:"playerId"`
	PlayerName string  `json:"playerName"`
	Kills      int     `json:"kills"`
	FinalMass  float64 `json:"finalMass"`
}

func LoadConfig() Config {
	return Config{
		Port:                    envOrDefault("PORT", "8081"),
		RedisAddr:               envOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:           os.Getenv("REDIS_PASSWORD"),
		RedisDB:                 envInt("REDIS_DB", 0),
		JWTSecret:               envOrDefault("JWT_SECRET", "dev-secret"),
		ReportSecret:            envOrDefault("REPORT_SHARED_SECRET", envOrDefault("JWT_SECRET", "dev-secret")),
		WSRouterBaseURL:         envOrDefault("WS_ROUTER_BASE_URL", ""),
		PublicGameWSURL:         envOrDefault("PUBLIC_GAME_WS_URL", "ws://localhost:8080/ws"),
		LobbyID:                 envOrDefault("LOBBY_ID", "local-lobby"),
		TokenTTL:                envDuration("TOKEN_TTL", 10*time.Minute),
		RegistryCleanupInterval: envDuration("REGISTRY_CLEANUP_INTERVAL", 15*time.Second),
	}
}

func NewServer(ctx context.Context, cfg Config, logger *log.Logger) (*Server, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	server := &Server{
		cfg:    cfg,
		logger: logger,
		redis:  client,
	}
	go server.startRegistryCleanup(ctx)
	return server, nil
}

func (s *Server) WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) HandleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		PlayerName string `json:"playerName"`
		Region     string `json:"region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	playerName := sanitizeName(payload.PlayerName)
	playerID := randomID("player")
	now := time.Now()
	assignment, err := s.selectLobbyAssignment(r.Context())
	if err != nil {
		http.Error(w, "game servers starting up, retry in a few seconds", http.StatusServiceUnavailable)
		return
	}

	claims := jwt.MapClaims{
		"sub":      playerID,
		"name":     playerName,
		"lobby_id": assignment.LobbyID,
		"pod_ip":   assignment.PodIP,
		"exp":      now.Add(s.cfg.TokenTTL).Unix(),
		"iat":      now.Unix(),
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		http.Error(w, "failed to sign token", http.StatusInternalServerError)
		return
	}

	wsURL := buildWSURL(s.cfg, assignment.LobbyID, token)
	writeJSON(w, http.StatusOK, map[string]string{
		"wsUrl":   wsURL,
		"lobbyId": assignment.LobbyID,
		"token":   token,
	})
}

func (s *Server) HandleLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := envIntValue(r.URL.Query().Get("limit"), 10)
	if limit <= 0 {
		limit = 10
	}

	ctx := r.Context()
	keys, err := s.redis.ZRevRange(ctx, leaderboardIndexKey, 0, int64(limit-1)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		http.Error(w, "failed to load leaderboard", http.StatusInternalServerError)
		return
	}

	entries := make([]leaderboardEntry, 0, len(keys))
	for _, key := range keys {
		hash, err := s.redis.HGetAll(ctx, leaderboardEntryKey(key)).Result()
		if err != nil || len(hash) == 0 {
			continue
		}
		entries = append(entries, leaderboardEntry{
			PlayerName: hash["playerName"],
			Kills:      envIntValue(hash["kills"], 0),
			MassBonus:  envIntValue(hash["massBonus"], 0),
			TotalScore: envIntValue(hash["totalScore"], 0),
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) HandleLeaderboardReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	if !verifySignature(body, r.Header.Get("X-Report-Signature"), s.cfg.ReportSecret) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var payload leaderboardReport
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	for _, result := range payload.Results {
		playerName := sanitizeName(result.PlayerName)
		member := strings.TrimSpace(result.PlayerID)
		if member == "" {
			http.Error(w, "missing player id", http.StatusBadRequest)
			return
		}
		massBonus := int(math.Floor(result.FinalMass / 50))
		totalScore := result.Kills + massBonus

		existing, err := s.redis.ZScore(ctx, leaderboardIndexKey, member).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			http.Error(w, "failed to update leaderboard", http.StatusInternalServerError)
			return
		}

		if err == nil && existing >= float64(totalScore) {
			continue
		}

		entryKey := leaderboardEntryKey(member)
		pipe := s.redis.TxPipeline()
		pipe.HSet(ctx, entryKey, map[string]any{
			"playerName": playerName,
			"kills":      result.Kills,
			"massBonus":  massBonus,
			"totalScore": totalScore,
		})
		pipe.ZAdd(ctx, leaderboardIndexKey, redis.Z{
			Score:  float64(totalScore),
			Member: member,
		})
		if _, err := pipe.Exec(ctx); err != nil {
			http.Error(w, "failed to persist leaderboard", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stored"})
}

func leaderboardEntryKey(member string) string {
	return fmt.Sprintf("leaderboard:entry:%s", member)
}

func verifySignature(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}
	expected := signPayload(payload, secret)
	return hmac.Equal([]byte(expected), []byte(signature))
}

func signPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
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

func randomID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
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
	return envIntValue(value, fallback)
}

func envIntValue(value string, fallback int) int {
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
