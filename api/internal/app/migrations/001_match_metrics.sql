CREATE TABLE IF NOT EXISTS match_reports (
    match_id text PRIMARY KEY,
    lobby_id text NOT NULL,
    match_kind text NOT NULL,
    end_reason text NOT NULL,
    drain_flag boolean NOT NULL DEFAULT false,
    is_debug boolean NOT NULL DEFAULT false,
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL,
    duration_ms bigint NOT NULL,
    schema_version integer NOT NULL,
    collector_version text NOT NULL,
    payload_hash text NOT NULL,
    human_count integer NOT NULL,
    bot_count integer NOT NULL,
    peak_concurrent_humans integer NOT NULL,
    config_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    match_metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS participant_reports (
    match_id text NOT NULL REFERENCES match_reports(match_id) ON DELETE CASCADE,
    participant_id text NOT NULL,
    session_player_id_hash text,
    is_bot boolean NOT NULL DEFAULT false,
    bot_level text,
    placement integer NOT NULL DEFAULT 0,
    summary_metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (match_id, participant_id)
);

CREATE TABLE IF NOT EXISTS match_events (
    id bigserial PRIMARY KEY,
    match_id text NOT NULL REFERENCES match_reports(match_id) ON DELETE CASCADE,
    ts_ms bigint NOT NULL,
    event_seq bigint NOT NULL,
    tick bigint,
    event_type text NOT NULL,
    actor_participant_id text,
    target_participant_id text,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (match_id, event_seq)
);

CREATE INDEX IF NOT EXISTS idx_match_reports_started_at ON match_reports (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_match_reports_kind_debug ON match_reports (match_kind, is_debug);
CREATE INDEX IF NOT EXISTS idx_participant_reports_bot_level ON participant_reports (bot_level) WHERE bot_level IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_match_events_type ON match_events (event_type);
