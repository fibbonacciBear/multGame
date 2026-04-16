package game

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestNewServerStartsIdle(t *testing.T) {
	server := newClassicTestServer()

	if server.lobby.Phase != phaseIdle {
		t.Fatalf("phase = %q, want %q", server.lobby.Phase, phaseIdle)
	}
	if !server.lobby.MatchOver {
		t.Fatalf("MatchOver = false, want true")
	}
	if !server.lobby.MatchStart.IsZero() {
		t.Fatalf("MatchStart = %v, want zero", server.lobby.MatchStart)
	}
	if !server.lobby.MatchEnds.IsZero() {
		t.Fatalf("MatchEnds = %v, want zero", server.lobby.MatchEnds)
	}
}

func TestHandleReadyzUsesConnectedHumansOnly(t *testing.T) {
	server := newClassicTestServer()
	server.cfg.MaxPlayers = 2
	server.lobby.Players["human-1"] = &Player{
		ID:        "human-1",
		Name:      "Pilot",
		Connected: true,
		Alive:     true,
	}
	server.lobby.Players["bot-1"] = &Player{ID: "bot-1", IsBot: true}
	server.lobby.Players["bot-2"] = &Player{ID: "bot-2", IsBot: true}

	recorder := httptest.NewRecorder()
	server.HandleReadyz(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestHandleWSRejectsWhenHumanCapReached(t *testing.T) {
	cfg := Config{
		JWTSecret:           "test-secret",
		LobbyID:             "lobby-test",
		TickRate:            60,
		SnapshotRate:        20,
		MaxPlayers:          1,
		WorldWidth:          1200,
		WorldHeight:         800,
		GravityAccel:        2400,
		Drag:                0.98,
		TerminalSpeed:       900,
		NumObjects:          8,
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
	server.lobby.Players["human-1"] = &Player{
		ID:        "human-1",
		Name:      "Pilot One",
		Connected: true,
		Alive:     true,
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/ws?lobby=lobby-test&token="+url.QueryEscape(signedTestToken(t, cfg.JWTSecret, "human-2", "Pilot Two", "lobby-test")),
		nil,
	)
	recorder := httptest.NewRecorder()

	server.HandleWS(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestStepTransitionsToIdleWhenLastHumanDisconnects(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()
	server.lobby.Phase = phaseActive
	server.lobby.MatchOver = false
	server.lobby.MatchEnds = now.Add(time.Minute)
	server.lobby.Players["human-1"] = &Player{
		ID:             "human-1",
		Name:           "Ghost",
		Connected:      false,
		DisconnectedAt: now,
		Alive:          true,
	}
	server.lobby.Players["bot-1"] = &Player{ID: "bot-1", IsBot: true}
	server.lobby.Projectiles = []*Projectile{{ID: "shot-1"}}
	server.lobby.KillFeed = []KillFeedEntry{{ID: "kill-1"}}

	server.step(now)

	if server.lobby.Phase != phaseIdle {
		t.Fatalf("phase = %q, want %q", server.lobby.Phase, phaseIdle)
	}
	if len(server.lobby.Players) != 0 {
		t.Fatalf("player count = %d, want 0", len(server.lobby.Players))
	}
	if len(server.lobby.Projectiles) != 0 {
		t.Fatalf("projectile count = %d, want 0", len(server.lobby.Projectiles))
	}
	if len(server.lobby.KillFeed) != 0 {
		t.Fatalf("kill feed count = %d, want 0", len(server.lobby.KillFeed))
	}
}

func TestBeginDrainDuringIntermissionCancelsCountdown(t *testing.T) {
	server := newClassicTestServer()
	server.lobby.Phase = phaseIntermission
	server.lobby.MatchOver = true
	server.lobby.IntermissionEnds = time.Now().Add(10 * time.Second)
	server.lobby.Players["human-1"] = &Player{
		ID:        "human-1",
		Name:      "Pilot",
		Connected: true,
		Alive:     true,
	}

	server.BeginDrain("server shutting down")

	if !server.draining {
		t.Fatalf("draining = false, want true")
	}
	if !server.lobby.IntermissionEnds.IsZero() {
		t.Fatalf("IntermissionEnds = %v, want zero", server.lobby.IntermissionEnds)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		server.WaitForDrain(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("WaitForDrain did not return while already match-over")
	}
}

func TestStepDrainingWithNoHumansCompletesImmediately(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()
	server.draining = true
	server.lobby.Phase = phaseActive
	server.lobby.MatchOver = false
	server.lobby.MatchEnds = now.Add(time.Minute)

	server.step(now)

	if !server.lobby.MatchOver {
		t.Fatalf("MatchOver = false, want true")
	}
	if server.lobby.Phase != phaseIdle {
		t.Fatalf("phase = %q, want %q", server.lobby.Phase, phaseIdle)
	}
}

func TestDrainingNaturalCompletionPreservesLeaderboardReporting(t *testing.T) {
	reported := make(chan leaderboardReport, 1)
	server := newClassicTestServer()
	server.cfg.APIServerURL = "http://api.test"
	server.cfg.ReportSecret = "test-secret"
	server.httpClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			defer request.Body.Close()

			if request.URL.String() != "http://api.test/api/leaderboard/report" {
				t.Fatalf("request URL = %q, want leaderboard endpoint", request.URL.String())
			}

			var payload leaderboardReport
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			reported <- payload

			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}
	server.draining = true

	now := time.Now()
	server.lobby.Phase = phaseActive
	server.lobby.MatchOver = false
	server.lobby.MatchEnds = now.Add(-time.Second)
	server.lobby.Players["human-1"] = &Player{
		ID:        "human-1",
		Name:      "Pilot",
		Connected: true,
		Alive:     true,
		Mass:      server.cfg.StartingMass,
		Health:    server.maxHealthForMass(server.cfg.StartingMass),
	}

	server.step(now)

	if !server.lobby.MatchOver {
		t.Fatalf("MatchOver = false, want true")
	}
	if server.lobby.Phase != phaseIntermission {
		t.Fatalf("phase = %q, want %q", server.lobby.Phase, phaseIntermission)
	}
	if !server.lobby.IntermissionEnds.IsZero() {
		t.Fatalf("IntermissionEnds = %v, want zero", server.lobby.IntermissionEnds)
	}

	select {
	case payload := <-reported:
		if len(payload.Results) != 1 {
			t.Fatalf("results count = %d, want 1", len(payload.Results))
		}
		if payload.Results[0].PlayerID != "human-1" {
			t.Fatalf("reported player = %q, want human-1", payload.Results[0].PlayerID)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected leaderboard report to be sent")
	}
}

func TestResetMatchLockedDropsDisconnectedHumans(t *testing.T) {
	server := newClassicTestServer()
	now := time.Now()
	server.lobby.Players["connected"] = &Player{
		ID:        "connected",
		Name:      "Pilot",
		Connected: true,
		Alive:     true,
	}
	server.lobby.Players["disconnected"] = &Player{
		ID:        "disconnected",
		Name:      "Ghost",
		Connected: false,
	}
	server.lobby.Players["bot-1"] = &Player{
		ID:    "bot-1",
		IsBot: true,
	}

	server.resetMatchLocked(now)

	if len(server.lobby.Players) != 1 {
		t.Fatalf("player count = %d, want 1", len(server.lobby.Players))
	}
	if _, ok := server.lobby.Players["connected"]; !ok {
		t.Fatalf("expected connected player to remain")
	}
	if server.lobby.Phase != phaseActive {
		t.Fatalf("phase = %q, want %q", server.lobby.Phase, phaseActive)
	}
}

func TestSnapshotProjectilesIncludeHeadingAndType(t *testing.T) {
	server := newClassicTestServer()
	server.lobby.Projectiles = []*Projectile{{
		ID:      "shot-1",
		OwnerID: "player-1",
		Type:    projectileTypeRailgun,
		Color:   "#68e1fd",
		X:       100,
		Y:       200,
		VX:      1200,
		VY:      100,
		Radius:  server.cfg.ProjectileRadius,
	}}

	shots := server.snapshotProjectilesLocked()

	if len(shots) != 1 {
		t.Fatalf("shots count = %d, want 1", len(shots))
	}
	if shots[0].VX != 1200 || shots[0].VY != 100 {
		t.Fatalf("shot velocity = (%v, %v), want (1200, 100)", shots[0].VX, shots[0].VY)
	}
	if shots[0].Type != projectileTypeRailgun {
		t.Fatalf("shot type = %q, want %q", shots[0].Type, projectileTypeRailgun)
	}
}
