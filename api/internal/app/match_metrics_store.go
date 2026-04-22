package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var openMatchMetricsStore = func(ctx context.Context, cfg Config) (matchMetricsStore, error) {
	return newPostgresMatchMetricsStore(ctx, cfg)
}

type postgresMatchMetricsStore struct {
	db *sql.DB
}

func (s *Server) ensureMatchMetricsStore(ctx context.Context) (matchMetricsStore, error) {
	if !s.cfg.MatchAnalyticsEnabled {
		return nil, errMatchMetricsDisabled
	}

	if store := s.currentMatchMetricsStore(); store != nil {
		MatchMetricsStoreAvailable.Set(1)
		return store, nil
	}

	s.matchMetricsInitMu.Lock()
	defer s.matchMetricsInitMu.Unlock()

	if store := s.currentMatchMetricsStore(); store != nil {
		MatchMetricsStoreAvailable.Set(1)
		return store, nil
	}

	initCtx, cancel := context.WithTimeout(ctx, matchMetricsStoreConnectTimeout(s.cfg))
	defer cancel()

	store, err := openMatchMetricsStore(initCtx, s.cfg)
	if err != nil {
		MatchMetricsStoreAvailable.Set(0)
		return nil, err
	}

	s.matchMetricsStoreMu.Lock()
	s.matchMetricsStore = store
	s.matchMetricsStoreMu.Unlock()

	MatchMetricsStoreAvailable.Set(1)
	return store, nil
}

func (s *Server) currentMatchMetricsStore() matchMetricsStore {
	s.matchMetricsStoreMu.RLock()
	defer s.matchMetricsStoreMu.RUnlock()
	return s.matchMetricsStore
}

func matchMetricsStoreConnectTimeout(cfg Config) time.Duration {
	if cfg.DBQueryTimeout > 0 {
		return cfg.DBQueryTimeout
	}
	return 3 * time.Second
}

func newPostgresMatchMetricsStore(ctx context.Context, cfg Config) (*postgresMatchMetricsStore, error) {
	db, err := openPostgresDB(cfg)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &postgresMatchMetricsStore{db: db}, nil
}

func openPostgresDB(cfg Config) (*sql.DB, error) {
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required when match analytics is enabled")
	}
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxInt(cfg.DBMaxOpenConns, 1))
	db.SetMaxIdleConns(maxInt(cfg.DBMaxIdleConns, 0))
	if cfg.DBConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	}
	return db, nil
}

func (s *postgresMatchMetricsStore) StoreMatchMetricsReport(
	ctx context.Context,
	report matchMetricsReport,
	payloadHash string,
) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO match_reports (
			match_id,
			lobby_id,
			match_kind,
			end_reason,
			drain_flag,
			is_debug,
			started_at,
			ended_at,
			duration_ms,
			schema_version,
			collector_version,
			payload_hash,
			human_count,
			bot_count,
			peak_concurrent_humans,
			config_snapshot,
			match_metrics
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (match_id) DO NOTHING`,
		report.MatchID,
		report.LobbyID,
		report.MatchKind,
		report.EndReason,
		report.DrainFlag,
		report.IsDebug,
		report.StartedAt,
		report.EndedAt,
		report.DurationMs,
		report.SchemaVersion,
		report.CollectorVersion,
		payloadHash,
		report.HumanCount,
		report.BotCount,
		report.PeakConcurrentHuman,
		jsonString(report.ConfigSnapshot),
		jsonString(report.MatchMetrics),
	)
	if err != nil {
		return "", err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		status, err := s.duplicateStatus(ctx, tx, report.MatchID, payloadHash)
		if err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return status, nil
	}

	for _, participant := range report.Participants {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO participant_reports (
				match_id,
				participant_id,
				session_player_id_hash,
				is_bot,
				bot_level,
				placement,
				summary_metrics
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			report.MatchID,
			participant.ParticipantID,
			nullString(participant.SessionPlayerIDHash),
			participant.IsBot,
			nullString(participant.BotLevel),
			participant.Placement,
			jsonString(participant.SummaryMetrics),
		); err != nil {
			return "", err
		}
	}

	for _, event := range report.Events {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO match_events (
				match_id,
				ts_ms,
				event_seq,
				tick,
				event_type,
				actor_participant_id,
				target_participant_id,
				payload
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			report.MatchID,
			event.TimestampMs,
			event.EventSeq,
			nullInt64(event.Tick),
			event.EventType,
			nullString(event.ActorParticipantID),
			nullString(event.TargetParticipantID),
			jsonString(event.Payload),
		); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return matchMetricsStatusAccepted, nil
}

func (s *postgresMatchMetricsStore) duplicateStatus(
	ctx context.Context,
	tx *sql.Tx,
	matchID string,
	payloadHash string,
) (string, error) {
	var storedHash string
	if err := tx.QueryRowContext(ctx, `SELECT payload_hash FROM match_reports WHERE match_id = $1`, matchID).Scan(&storedHash); err != nil {
		return "", err
	}
	if storedHash != payloadHash {
		return "", errMatchMetricsConflict
	}
	return matchMetricsStatusAlreadyStored, nil
}

func jsonString(value map[string]any) string {
	if value == nil {
		return "{}"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: strings.TrimSpace(value) != ""}
}

func nullInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func maxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func (s *postgresMatchMetricsStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close match metrics db: %w", err)
	}
	return nil
}
