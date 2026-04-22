package game

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMatchAnalyticsReportsTimeLimitCompletion(t *testing.T) {
	reported := make(chan matchMetricsReport, 1)
	server := newClassicTestServer()
	server.cfg.MatchAnalyticsEnabled = true
	server.cfg.APIServerURL = "http://api.test"
	server.cfg.ReportSecret = "test-secret"
	server.httpClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			defer request.Body.Close()
			switch request.URL.String() {
			case "http://api.test/api/match-metrics/report":
				var payload matchMetricsReport
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				reported <- payload
			case "http://api.test/api/leaderboard/report":
			default:
				t.Fatalf("unexpected report URL %q", request.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":"accepted"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	now := time.Now()
	server.lobby.Players["human-1"] = &Player{
		ID:        "human-1",
		Name:      "Pilot",
		Connected: true,
		Alive:     true,
		Mass:      server.cfg.StartingMass,
		Health:    server.maxHealthForMass(server.cfg.StartingMass),
	}
	server.resetMatchLocked(now.Add(-time.Minute))
	server.lobby.MatchEnds = now.Add(-time.Second)

	server.step(now)

	select {
	case payload := <-reported:
		if payload.MatchID == "" {
			t.Fatalf("missing match id")
		}
		if payload.EndReason != matchEndReasonTimeLimit {
			t.Fatalf("end reason = %q, want %q", payload.EndReason, matchEndReasonTimeLimit)
		}
		if len(payload.Participants) == 0 {
			t.Fatalf("expected participants in report")
		}
		if payload.PeakConcurrentHuman == 0 {
			t.Fatalf("peak humans = 0, want positive")
		}
	case <-time.After(time.Second):
		t.Fatalf("expected match metrics report")
	}
}

func TestMatchAnalyticsAssignsPlacementToEvictedParticipants(t *testing.T) {
	server := newClassicTestServer()
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	collector := newMatchMetricsCollector(&Lobby{
		ID:        "lobby-1",
		MatchID:   "match-1",
		MatchKind: matchKindNormal,
	}, server.cfg, start)

	active := &Player{
		ID:        "active",
		Name:      "Active",
		Connected: true,
		Alive:     true,
		Mass:      10,
	}
	evicted := &Player{
		ID:        "evicted",
		Name:      "Evicted",
		Connected: true,
		Alive:     true,
		Mass:      60,
	}
	collector.RegisterParticipant(active, "active-hash", start)
	collector.RegisterParticipant(evicted, "evicted-hash", start)
	collector.Sample(map[string]*Player{
		active.ID:  active,
		evicted.ID: evicted,
	}, start.Add(time.Second))
	collector.OnDisconnect(evicted, start.Add(2*time.Second))

	report := collector.Finalize(
		map[string]*Player{active.ID: active},
		[]scoreboardResult{{
			PlayerID:   active.ID,
			PlayerName: active.Name,
			FinalMass:  active.Mass,
			TotalScore: 0,
		}},
		matchEndReasonTimeLimit,
		false,
		start.Add(time.Minute),
	)
	if report == nil {
		t.Fatalf("expected report")
	}

	placements := make(map[string]int, len(report.Participants))
	for _, participant := range report.Participants {
		placements[participant.SessionPlayerIDHash] = participant.Placement
	}
	if placements["evicted-hash"] == 0 {
		t.Fatalf("evicted participant placement = 0, want ordinal placement")
	}
	if placements["active-hash"] == 0 {
		t.Fatalf("active participant placement = 0, want ordinal placement")
	}
}

func TestMatchAnalyticsSenderRetriesServerErrors(t *testing.T) {
	attempts := 0
	server := newClassicTestServer()
	server.cfg.MatchAnalyticsReportRetries = 2
	server.cfg.MatchAnalyticsRetryDelay = time.Millisecond
	server.cfg.APIServerURL = "http://api.test"
	server.httpClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			attempts++
			status := http.StatusInternalServerError
			if attempts == 2 {
				status = http.StatusOK
			}
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader(`{"status":"accepted"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	server.reportMatchMetrics(matchMetricsReport{
		SchemaVersion: matchMetricsSchemaVersion,
		MatchID:       "match-1",
		LobbyID:       "lobby-1",
		MatchKind:     string(matchKindNormal),
		EndReason:     matchEndReasonTimeLimit,
		StartedAt:     time.Now(),
		EndedAt:       time.Now(),
	})

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestMatchAnalyticsSenderDoesNotRetryValidationErrors(t *testing.T) {
	attempts := 0
	server := newClassicTestServer()
	server.cfg.MatchAnalyticsReportRetries = 2
	server.cfg.MatchAnalyticsRetryDelay = time.Millisecond
	server.cfg.APIServerURL = "http://api.test"
	server.httpClient = &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"status":"validation_error"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	server.reportMatchMetrics(matchMetricsReport{
		SchemaVersion: matchMetricsSchemaVersion,
		MatchID:       "match-1",
		LobbyID:       "lobby-1",
		MatchKind:     string(matchKindNormal),
		EndReason:     matchEndReasonTimeLimit,
		StartedAt:     time.Now(),
		EndedAt:       time.Now(),
	})

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
