package game

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func TestHandleWSWelcomesValidatedPlayer(t *testing.T) {
	cfg := Config{
		JWTSecret:           "test-secret",
		LobbyID:             "lobby-test",
		PodIP:               "",
		TickRate:            60,
		SnapshotRate:        20,
		MaxPlayers:          10,
		WorldWidth:          4000,
		WorldHeight:         4000,
		GravityAccel:        2400,
		Drag:                0.98,
		TerminalSpeed:       900,
		PlayerRadiusScale:   3,
		NumObjects:          200,
		StartingMass:        10,
		StartingHealth:      100,
		ProjectileSpeed:     1250,
		ProjectileDamage:    28,
		ProjectileRadius:    5,
		ShootCooldown:       250 * time.Millisecond,
		ProjectileTTL:       1200 * time.Millisecond,
		MatchDuration:       time.Minute,
		RespawnDelay:        2 * time.Second,
		BotFillDelay:        time.Hour,
		HealthTickThreshold: 2 * time.Second,
	}

	server := NewServer(cfg, log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.Start(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWS)

	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws?lobby=lobby-test&token=" + url.QueryEscape(signedTestToken(t, cfg.JWTSecret, "player-1", "Pilot", "lobby-test"))

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	var message welcomeMessage
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}

	if message.Type != "welcome" {
		t.Fatalf("message.Type = %q, want welcome", message.Type)
	}
	if message.PlayerID != "player-1" {
		t.Fatalf("message.PlayerID = %q, want player-1", message.PlayerID)
	}
	if message.LobbyID != "lobby-test" {
		t.Fatalf("message.LobbyID = %q, want lobby-test", message.LobbyID)
	}
}

func signedTestToken(t *testing.T, secret, playerID, playerName, lobbyID string) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      playerID,
		"name":     playerName,
		"lobby_id": lobbyID,
		"exp":      time.Now().Add(time.Minute).Unix(),
		"iat":      time.Now().Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return token
}
