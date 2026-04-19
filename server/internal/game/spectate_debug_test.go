package game

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestHandleWSRejectsWhenSpectatorCapReached(t *testing.T) {
	server := newClassicTestServer()
	server.cfg.JWTSecret = "test-secret"
	server.cfg.MaxSpectators = 1
	server.spectators["viewer-1"] = &ClientConnection{
		ViewerID:    "viewer-1",
		SessionMode: sessionModeSpectator,
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/ws?lobby=lobby-test&token="+url.QueryEscape(signedSessionToken(t, server.cfg.JWTSecret, jwt.MapClaims{
			"sub":          "viewer-2",
			"lobby_id":     "lobby-test",
			"session_mode": string(sessionModeSpectator),
			"exp":          time.Now().Add(time.Minute).Unix(),
			"iat":          time.Now().Unix(),
		})),
		nil,
	)
	recorder := httptest.NewRecorder()

	server.HandleWS(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestUpdateGaugesSkipsMassObservationsForDebugMatches(t *testing.T) {
	server := newClassicTestServer()
	server.lobby.MatchKind = matchKindDebugBotSim
	server.lobby.Players["bot-1"] = &Player{
		ID:        "bot-1",
		Name:      "Bot",
		IsBot:     true,
		Alive:     true,
		Mass:      64,
		Health:    server.maxHealthForMass(64),
		Connected: false,
	}

	before := histogramSampleCount(t, PlayerMass)
	server.updateGauges()
	after := histogramSampleCount(t, PlayerMass)

	if after != before {
		t.Fatalf("PlayerMass sample count = %d, want %d", after, before)
	}
}

func TestDebugCombatMetricsAreSuppressed(t *testing.T) {
	server := newClassicTestServer()
	server.lobby.MatchKind = matchKindDebugBotSim
	now := time.Now()
	left := &Player{
		ID:     "left",
		Name:   "Left",
		Alive:  true,
		Mass:   40,
		Health: 1,
		X:      100,
		Y:      100,
	}
	right := &Player{
		ID:     "right",
		Name:   "Right",
		Alive:  true,
		Mass:   40,
		Health: server.maxHealthForMass(40),
		X:      110,
		Y:      100,
	}
	server.lobby.Players[left.ID] = left
	server.lobby.Players[right.ID] = right

	beforeCrashContacts := counterValue(t, CrashContacts)
	damage := make(map[string]float64)
	sources := make(map[string]combatSource)
	combatants := make(map[string]struct{})
	knockbacks := make([]crashPair, 0)
	server.collectCrashDamageLocked(now, damage, sources, combatants, &knockbacks)
	afterCrashContacts := counterValue(t, CrashContacts)

	if len(knockbacks) == 0 {
		t.Fatalf("expected crash detection to produce knockback entries")
	}
	if afterCrashContacts != beforeCrashContacts {
		t.Fatalf("CrashContacts = %v, want %v", afterCrashContacts, beforeCrashContacts)
	}

	beforeCrashLethals := counterValue(t, CrashLethalOutcomes)
	beforePlayerKills := counterValue(t, PlayerKills)
	server.applyCombatResolutionLocked(now, map[string]float64{
		left.ID: server.maxHealthForMass(left.Mass),
	}, map[string]combatSource{
		left.ID: {
			killerID: right.ID,
			kind:     combatSourceCrash,
			reason:   "rammed by Right",
		},
	}, map[string]struct{}{
		left.ID:  {},
		right.ID: {},
	}, nil)
	afterCrashLethals := counterValue(t, CrashLethalOutcomes)
	afterPlayerKills := counterValue(t, PlayerKills)

	if afterCrashLethals != beforeCrashLethals {
		t.Fatalf("CrashLethalOutcomes = %v, want %v", afterCrashLethals, beforeCrashLethals)
	}
	if afterPlayerKills != beforePlayerKills {
		t.Fatalf("PlayerKills = %v, want %v", afterPlayerKills, beforePlayerKills)
	}
}

func signedSessionToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return token
}

func counterValue(t *testing.T, metric prometheus.Counter) float64 {
	t.Helper()

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("metric.Write() error = %v", err)
	}
	return dtoMetric.GetCounter().GetValue()
}

func histogramSampleCount(t *testing.T, metric prometheus.Histogram) uint64 {
	t.Helper()

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("metric.Write() error = %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleCount()
}
