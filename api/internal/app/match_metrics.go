package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	matchMetricsSchemaVersion = 1

	matchMetricsStatusAccepted        = "accepted"
	matchMetricsStatusAlreadyStored   = "already_stored"
	matchMetricsStatusValidationError = "validation_error"
	matchMetricsStatusAuthError       = "auth_error"
	matchMetricsStatusConflict        = "conflict"
	matchMetricsStatusInternalError   = "internal_error"
	matchMetricsStatusDisabled        = "disabled"
)

var (
	errMatchMetricsConflict = errors.New("match metrics payload conflicts with stored report")
	errMatchMetricsDisabled = errors.New("match metrics disabled")
)

type matchMetricsStore interface {
	StoreMatchMetricsReport(ctx context.Context, report matchMetricsReport, payloadHash string) (string, error)
}

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

func (s *Server) handleMatchMetricsReport(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.MatchAnalyticsEnabled {
		MatchMetricsReports.WithLabelValues(matchMetricsStatusDisabled).Inc()
		http.Error(w, "match analytics disabled", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := readLimitedBody(w, r, s.cfg.MatchMetricsMaxBytes)
	if err != nil {
		MatchMetricsReports.WithLabelValues(matchMetricsStatusValidationError).Inc()
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	if !verifySignature(body, r.Header.Get("X-Report-Signature"), s.cfg.ReportSecret) {
		MatchMetricsReports.WithLabelValues(matchMetricsStatusAuthError).Inc()
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var payload matchMetricsReport
	if err := json.Unmarshal(body, &payload); err != nil {
		MatchMetricsReports.WithLabelValues(matchMetricsStatusValidationError).Inc()
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.validateMatchMetricsReport(payload); err != nil {
		MatchMetricsReports.WithLabelValues(matchMetricsStatusValidationError).Inc()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.DBQueryTimeout)
	defer cancel()

	store, err := s.ensureMatchMetricsStore(ctx)
	if err != nil {
		MatchMetricsReports.WithLabelValues(matchMetricsStatusInternalError).Inc()
		MatchMetricsWriteFailures.Inc()
		if s.logger != nil {
			s.logger.Printf("match metrics store unavailable: %v", err)
		}
		http.Error(w, "match metrics storage unavailable", http.StatusServiceUnavailable)
		return
	}

	start := time.Now()
	status, err := store.StoreMatchMetricsReport(ctx, payload, hashPayload(body))
	MatchMetricsWriteDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		if errors.Is(err, errMatchMetricsConflict) {
			MatchMetricsReports.WithLabelValues(matchMetricsStatusConflict).Inc()
			MatchMetricsDuplicates.WithLabelValues(matchMetricsStatusConflict).Inc()
			http.Error(w, "conflicting match metrics report", http.StatusConflict)
			return
		}
		MatchMetricsReports.WithLabelValues(matchMetricsStatusInternalError).Inc()
		MatchMetricsWriteFailures.Inc()
		s.logger.Printf("match metrics write failed: %v", err)
		http.Error(w, "failed to persist match metrics", http.StatusInternalServerError)
		return
	}

	switch status {
	case matchMetricsStatusAlreadyStored:
		MatchMetricsReports.WithLabelValues(matchMetricsStatusAlreadyStored).Inc()
		MatchMetricsDuplicates.WithLabelValues("same_hash").Inc()
	default:
		MatchMetricsReports.WithLabelValues(matchMetricsStatusAccepted).Inc()
		status = matchMetricsStatusAccepted
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func readLimitedBody(w http.ResponseWriter, r *http.Request, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	defer r.Body.Close()
	return io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
}

func (s *Server) validateMatchMetricsReport(report matchMetricsReport) error {
	if report.SchemaVersion != matchMetricsSchemaVersion {
		return errors.New("unsupported schema version")
	}
	if strings.TrimSpace(report.MatchID) == "" {
		return errors.New("missing match id")
	}
	if strings.TrimSpace(report.LobbyID) == "" {
		return errors.New("missing lobby id")
	}
	if strings.TrimSpace(report.MatchKind) == "" {
		return errors.New("missing match kind")
	}
	if strings.TrimSpace(report.EndReason) == "" {
		return errors.New("missing end reason")
	}
	if report.StartedAt.IsZero() || report.EndedAt.IsZero() || report.EndedAt.Before(report.StartedAt) {
		return errors.New("invalid match timestamps")
	}
	if len(report.Participants) > s.cfg.MatchMetricsMaxPlayers {
		return errors.New("too many participants")
	}
	if len(report.Events) > s.cfg.MatchMetricsMaxEvents {
		return errors.New("too many events")
	}
	participants := make(map[string]struct{}, len(report.Participants))
	for _, participant := range report.Participants {
		id := strings.TrimSpace(participant.ParticipantID)
		if id == "" {
			return errors.New("missing participant id")
		}
		if _, exists := participants[id]; exists {
			return errors.New("duplicate participant id")
		}
		participants[id] = struct{}{}
	}
	seenEvents := make(map[int64]struct{}, len(report.Events))
	for _, event := range report.Events {
		if err := validateMatchMetricEvent(event, participants); err != nil {
			return err
		}
		if _, exists := seenEvents[event.EventSeq]; exists {
			return errors.New("duplicate event sequence")
		}
		seenEvents[event.EventSeq] = struct{}{}
	}
	return nil
}

func validateMatchMetricEvent(event matchMetricsEvent, participants map[string]struct{}) error {
	if event.EventSeq <= 0 {
		return errors.New("invalid event sequence")
	}
	switch event.EventType {
	case "kill", "death", "pickup", "respawn", "disconnect", "bot_decision_summary", "bot_state_transition":
	default:
		return errors.New("unsupported event type")
	}
	if event.ActorParticipantID != "" {
		if _, ok := participants[event.ActorParticipantID]; !ok {
			return errors.New("event actor participant missing")
		}
	}
	if event.TargetParticipantID != "" {
		if _, ok := participants[event.TargetParticipantID]; !ok {
			return errors.New("event target participant missing")
		}
	}
	return nil
}

func hashPayload(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
