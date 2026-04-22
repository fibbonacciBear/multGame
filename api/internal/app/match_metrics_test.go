package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeMatchMetricsStore struct {
	status string
	err    error
	hash   string
	report matchMetricsReport
}

func (f *fakeMatchMetricsStore) StoreMatchMetricsReport(ctx context.Context, report matchMetricsReport, payloadHash string) (string, error) {
	f.hash = payloadHash
	f.report = report
	if f.status == "" {
		f.status = matchMetricsStatusAccepted
	}
	return f.status, f.err
}

func TestHandleMatchMetricsReportStoresSignedPayload(t *testing.T) {
	store := &fakeMatchMetricsStore{}
	server := &Server{
		cfg: Config{
			ReportSecret:           "test-secret",
			MatchAnalyticsEnabled:  true,
			MatchMetricsMaxBytes:   1 << 20,
			MatchMetricsMaxPlayers: 10,
			MatchMetricsMaxEvents:  10,
			DBQueryTimeout:         time.Second,
		},
		matchMetricsStore: store,
	}

	body := mustMatchMetricsBody(t)
	request := httptest.NewRequest(http.MethodPost, "/api/match-metrics/report", strings.NewReader(string(body)))
	request.Header.Set("X-Report-Signature", signPayload(body, "test-secret"))
	recorder := httptest.NewRecorder()

	server.HandleMatchMetricsReport(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if store.report.MatchID != "match-1" {
		t.Fatalf("stored match id = %q, want match-1", store.report.MatchID)
	}
	if store.hash == "" {
		t.Fatalf("payload hash was not passed to store")
	}
}

func TestHandleMatchMetricsReportTreatsMatchingDuplicateAsSuccess(t *testing.T) {
	store := &fakeMatchMetricsStore{status: matchMetricsStatusAlreadyStored}
	server := &Server{
		cfg: Config{
			ReportSecret:           "test-secret",
			MatchAnalyticsEnabled:  true,
			MatchMetricsMaxBytes:   1 << 20,
			MatchMetricsMaxPlayers: 10,
			MatchMetricsMaxEvents:  10,
			DBQueryTimeout:         time.Second,
		},
		matchMetricsStore: store,
	}

	body := mustMatchMetricsBody(t)
	request := httptest.NewRequest(http.MethodPost, "/api/match-metrics/report", strings.NewReader(string(body)))
	request.Header.Set("X-Report-Signature", signPayload(body, "test-secret"))
	recorder := httptest.NewRecorder()

	server.HandleMatchMetricsReport(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), matchMetricsStatusAlreadyStored) {
		t.Fatalf("response = %q, want already stored status", recorder.Body.String())
	}
}

func TestHandleMatchMetricsReportRejectsConflictWithoutRetryStatus(t *testing.T) {
	store := &fakeMatchMetricsStore{err: errMatchMetricsConflict}
	server := &Server{
		cfg: Config{
			ReportSecret:           "test-secret",
			MatchAnalyticsEnabled:  true,
			MatchMetricsMaxBytes:   1 << 20,
			MatchMetricsMaxPlayers: 10,
			MatchMetricsMaxEvents:  10,
			DBQueryTimeout:         time.Second,
		},
		matchMetricsStore: store,
	}

	body := mustMatchMetricsBody(t)
	request := httptest.NewRequest(http.MethodPost, "/api/match-metrics/report", strings.NewReader(string(body)))
	request.Header.Set("X-Report-Signature", signPayload(body, "test-secret"))
	recorder := httptest.NewRecorder()

	server.HandleMatchMetricsReport(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
}

func TestHandleMatchMetricsReportRejectsUnsignedPayload(t *testing.T) {
	server := &Server{
		cfg: Config{
			ReportSecret:           "test-secret",
			MatchAnalyticsEnabled:  true,
			MatchMetricsMaxBytes:   1 << 20,
			MatchMetricsMaxPlayers: 10,
			MatchMetricsMaxEvents:  10,
			DBQueryTimeout:         time.Second,
		},
		matchMetricsStore: &fakeMatchMetricsStore{},
	}

	request := httptest.NewRequest(http.MethodPost, "/api/match-metrics/report", strings.NewReader(string(mustMatchMetricsBody(t))))
	recorder := httptest.NewRecorder()

	server.HandleMatchMetricsReport(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestHandleMatchMetricsReportRejectsOversizedPayload(t *testing.T) {
	server := &Server{
		cfg: Config{
			ReportSecret:           "test-secret",
			MatchAnalyticsEnabled:  true,
			MatchMetricsMaxBytes:   8,
			MatchMetricsMaxPlayers: 10,
			MatchMetricsMaxEvents:  10,
			DBQueryTimeout:         time.Second,
		},
		matchMetricsStore: &fakeMatchMetricsStore{},
	}

	body := mustMatchMetricsBody(t)
	request := httptest.NewRequest(http.MethodPost, "/api/match-metrics/report", strings.NewReader(string(body)))
	request.Header.Set("X-Report-Signature", signPayload(body, "test-secret"))
	recorder := httptest.NewRecorder()

	server.HandleMatchMetricsReport(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestMatchMetricsStoreUnavailableDoesNotTakeAPIHealthDown(t *testing.T) {
	previousOpen := openMatchMetricsStore
	openMatchMetricsStore = func(ctx context.Context, cfg Config) (matchMetricsStore, error) {
		return nil, errors.New("postgres unavailable")
	}
	defer func() {
		openMatchMetricsStore = previousOpen
	}()

	var logOutput bytes.Buffer
	server := &Server{
		cfg: Config{
			ReportSecret:           "test-secret",
			MatchAnalyticsEnabled:  true,
			MatchMetricsMaxBytes:   1 << 20,
			MatchMetricsMaxPlayers: 10,
			MatchMetricsMaxEvents:  10,
			DBQueryTimeout:         time.Second,
		},
		logger: log.New(&logOutput, "", 0),
	}

	server.initializeMatchMetricsStore(context.Background(), "test")
	if server.matchMetricsStore != nil {
		t.Fatalf("match metrics store initialized unexpectedly")
	}

	healthRecorder := httptest.NewRecorder()
	server.HandleHealthz(healthRecorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthRecorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", healthRecorder.Code, http.StatusOK)
	}
	if !strings.Contains(healthRecorder.Body.String(), `"matchAnalyticsStore":"unavailable"`) {
		t.Fatalf("health body = %q, want unavailable analytics store", healthRecorder.Body.String())
	}
	if !strings.Contains(logOutput.String(), "API will stay up") {
		t.Fatalf("startup log = %q, want fail-open message", logOutput.String())
	}

	body := mustMatchMetricsBody(t)
	request := httptest.NewRequest(http.MethodPost, "/api/match-metrics/report", strings.NewReader(string(body)))
	request.Header.Set("X-Report-Signature", signPayload(body, "test-secret"))
	recorder := httptest.NewRecorder()

	server.HandleMatchMetricsReport(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func mustMatchMetricsBody(t *testing.T) []byte {
	t.Helper()
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	payload := matchMetricsReport{
		SchemaVersion:       matchMetricsSchemaVersion,
		CollectorVersion:    "test",
		MatchID:             "match-1",
		LobbyID:             "lobby-1",
		MatchKind:           matchKindNormal,
		EndReason:           "time_limit",
		StartedAt:           start,
		EndedAt:             start.Add(time.Minute),
		DurationMs:          time.Minute.Milliseconds(),
		HumanCount:          1,
		PeakConcurrentHuman: 1,
		ConfigSnapshot:      map[string]any{"maxPlayers": 10},
		MatchMetrics:        map[string]any{"totalKills": 0},
		Participants: []matchMetricsParticipant{{
			ParticipantID:  "participant-001",
			IsBot:          false,
			Placement:      1,
			SummaryMetrics: map[string]any{"kills": 0},
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return body
}
