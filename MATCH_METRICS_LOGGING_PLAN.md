---
name: match metrics logging
overview: Brainstorm and define a flexible Postgres-backed match analytics pipeline that records bot and player behavior after each match without forcing frequent schema churn. The design should fit the existing game-server to API reporting flow and keep gameplay changes localized.
todos:
  - id: define-metric-catalog
    content: Choose the initial summary metrics and curated event types to support in V1 for bots, humans, and matches.
    status: pending
  - id: design-payload-and-schema
    content: Define the versioned `matchMetricsReport` payload and the Postgres tables using relational metadata plus JSONB.
    status: pending
  - id: map-hook-points
    content: Map collector hook calls onto current match-end, kill, respawn, pickup, disconnect, and bot-decision code paths.
    status: pending
  - id: api-ingestion-path
    content: Plan a signed API ingestion endpoint and centralized Postgres write path, reusing the existing leaderboard-report pattern.
    status: pending
  - id: query-and-rollout
    content: Define initial SQL views/dashboards and a phased rollout so you can add metrics without frequent schema migrations.
    status: pending
---

# Match Metrics Logging Plan

## Recommended Scope

Use **end-of-match summaries plus a curated event stream**.

That fits your goal well because it gives you:

- cheap, queryable match and participant summaries for dashboards and balancing
- a small set of high-value behavioral events for deeper analysis
- a stable schema that rarely changes, while still letting you add/remove metrics with minimal code churn

Keep participant identity **practical but structured**:

- `participant_id` should stay match-scoped
- the current system only has per-session/player-join IDs, so any hashed identifier available in V1 is really a session-scoped hash unless a future stable account/client identifier is introduced
- if needed in V1, store a separate hashed session identifier such as `session_player_id_hash`
- omit `display_name` by default in V1 until retention/privacy policy is implemented

## Best Metrics To Record

Start with a small, high-signal set rather than trying to log everything.

### Bot Metrics

Per bot, per match:

- `bot_level` / profile id
- spawn count / respawn count
- survival time alive
- final placement
- kills, deaths, kill/death split vs humans vs bots
- final mass, peak mass, average mass sampled over match
- damage dealt / damage taken if available
- pickups collected
- shots fired, hits landed, hit rate if projectile hooks are easy to add
- combat engagements entered
- time spent fleeing vs pursuing if you expose those decisions
- target switches / retarget count
- border or corner recoveries
- stall time / low-progress time

These are especially useful for tuning profiles in [server/internal/game/bot.go](server/internal/game/bot.go).

### Player Behavior Metrics

Per human participant, per match:

- join time, disconnect time, total connected duration
- total alive time / survival time
- kills, deaths, death reasons, killed-by-bot vs killed-by-human
- final placement
- final mass, peak mass, average mass
- pickups collected
- distance traveled
- shots fired, hits landed, hit rate
- combat engagements entered
- first combat time, first kill time, first death time
- time near borders / time in danger zone if easy to compute
- idle or low-input time
- respawn count
- aggression ratio (initiated combat vs disengaged/fled), if derived later

### Match-Level Metrics

Per match:

- `match_id`, `lobby_id`, `match_kind`, start/end timestamps, duration
- `end_reason` such as `time_limit`, `no_humans`, `drain`, `debug_abandoned`
- player count, bot count, human count, peak concurrent humans
- map/config snapshot needed for analysis (match duration, max players, bot config version)
- total kills, kill split human-vs-human / human-vs-bot / bot-vs-bot
- total pickups, total shots, total respawns
- winner type (human or bot)
- whether this was a debug or bot-sim match

Persist **debug matches too**, but tag them explicitly so normal dashboards can exclude them by default.

## Fit With Current Code

The cleanest insertion points already exist:

- [server/internal/game/server.go](server/internal/game/server.go) owns `Player`, `Lobby`, match lifecycle, `scoreboardLocked()`, and match-end handling.
- [server/internal/game/collision.go](server/internal/game/collision.go) resolves combat and funnels lethal outcomes into `killPlayerLocked`.
- [server/internal/game/bot.go](server/internal/game/bot.go) contains bot behavior and is the right place for optional bot-decision counters.
- [api/internal/app/server.go](api/internal/app/server.go) already accepts a signed end-of-match leaderboard payload and persists centralized results.

A very important existing pattern is the match-end flow in [server/internal/game/server.go](server/internal/game/server.go): the server computes the final scoreboard, calls `finishMatchLocked`, then asynchronously posts a signed report to the API. Reusing that exact pattern for metrics keeps DB access out of the game server and avoids scattering persistence logic through simulation code.

However, the plan must also cover **early-ended and abandoned matches**, because not every lobby reaches the normal `finishMatchLocked` path. The collector and finalization flow should explicitly support:

- normal time-limit completion
- matches ending because all humans left
- drain or shutdown-related termination
- debug-session abandonment

The collector should persist a report for these match endings too, with an explicit `end_reason`.

In practice, that means finalization must happen before state-clearing transitions such as `transitionToIdleLocked()` and before drain-completion paths such as `markDrainCompleteLocked()` return control.

To avoid double submission across normal completion, intermission cleanup, and early-exit paths, finalization should go through a single helper such as `finalizeMatchAnalyticsLocked(endReason, flags)` guarded by collector state like `finalized bool`. The collector should be marked finalized once the immutable payload is built and handed off to the sender queue/path; later calls must become a no-op even if the async sender eventually retries or fails.

This also means delivery is **best-effort** unless a durable outbox/spool is added later. If the game-server process dies after finalization but before a successful API submission, that report may be lost and should be treated as acceptable analytics loss for V1.

## Recommended Architecture

```mermaid
flowchart LR
    GameHooks[GameplayHooks] --> MetricsCollector[MatchMetricsCollector]
    MetricsCollector --> Finalize[FinalizeAtMatchEnd]
    Finalize --> SignedReport[SignedMetricsReport]
    SignedReport --> ApiIngest[APIIngestionHandler]
    ApiIngest --> Postgres[(Postgres)]
    Postgres --> Views[SQLViewsOrMaterializedViews]
```

### 1. Collect metrics in-memory during the match

Add a per-lobby collector such as `MatchMetricsCollector` in a new file like [server/internal/game/match_metrics.go](server/internal/game/match_metrics.go).

It should:

- own per-match accumulators and participant summaries
- own participant state independently of `lobby.Players`, so disconnected or evicted humans are still represented at finalize time
- expose tiny hook methods like `OnKill`, `OnPickup`, `OnShot`, `OnRespawn`, `OnDisconnect`, `OnBotDecision`
- compute some derived metrics at finalize time instead of on every tick when possible
- reset cleanly in `resetMatchLocked`

### 2. Report one payload at match end

At match end, build a single `matchMetricsReport` payload beside the existing leaderboard report in [server/internal/game/server.go](server/internal/game/server.go).

Important detail: finalization should happen **while the game lock is held**, and should return a fully copied immutable payload such as `Finalize(now, scoreboard, endReason)`. The async sender should only receive that copied payload, never references into mutable lobby state.

Recommended payload shape:

- top-level match metadata
- `end_reason`
- optional flags such as `drain=true` when the match reached time limit while a drain was also in progress
- `schema_version`
- `collector_version` or `metric_set_version`
- array of participant summaries
- array of curated events
- optional `config_snapshot` JSON object

### 3. Persist through the API, not directly from the game server

Add a new API route such as `/api/match-metrics/report` in [api/internal/app/server.go](api/internal/app/server.go), using the same signature verification pattern as the leaderboard report.

Recommendation: only the API should talk to Postgres.

That keeps:

- database credentials centralized
- the game server simple and stateless
- retries / validation / dedupe in one place

Delivery semantics should be explicit:

- the game server should do bounded retry on metrics report submission
- the sender should retry only network errors, request timeouts, and retriable `5xx` responses
- matching-duplicate responses such as `already_stored` should be treated as success
- validation/auth `4xx` responses should be treated as permanent failures and should not be retried
- mismatched-hash `409 Conflict` responses should be treated as non-retryable integrity errors
- the API should store a payload hash for each accepted `match_id`
- a duplicate with the same `match_id` and same payload hash should return an `already_stored`-style success response
- a duplicate with the same `match_id` but different payload hash should be rejected as a conflict
- if retries are exhausted, log and count the failure as dropped analytics rather than blocking gameplay

API persistence should be transactional: if any Postgres write for a report fails, roll back the full report and return a retriable 5xx response. Only fully committed reports should participate in duplicate detection and `already_stored` responses.

If concurrent requests race to insert the same `match_id`, the database constraint should resolve the winner. The loser should re-read the stored `payload_hash` and return `already_stored` for the same hash or `409 Conflict` for a different hash.

That makes the system resilient without putting the simulation loop at risk.

### Deployment model

Assume this deployment shape unless requirements change:

- **production** uses a managed Postgres provider
- **local development** uses a local Postgres instance, typically via Docker
- **`game-dev` k8s** keeps match analytics disabled by default until explicit Postgres wiring is added for that environment
- if `game-dev` later needs analytics, give it either a separate managed/dev Postgres instance or clearly temporary dev-only DB wiring, but do **not** share production analytics credentials or data paths
- the application code and schema stay the same across environments; only connection config and secret wiring differ

Under this model, production rollout does **not** require in-cluster Postgres manifests, PVCs, or self-hosted database operations. The main production work is API connectivity, secret management, migrations, SSL/TLS settings, connection-pool sizing, and provider-level backup/retention verification.

Operational default by environment:

- **local Docker** can enable analytics once the compose stack includes a Postgres service plus migration/docs wiring
- **`game-dev`** should leave `MATCH_ANALYTICS_ENABLED=false` until it has its own secrets, DB target, and migration path
- **production** can enable analytics only after managed-Postgres secrets/config and migrations are in place

### Migration execution

Treat schema migrations as an explicit deployment step, not something every API pod does opportunistically on startup.

Recommended default:

- **production / k8s environments** run migrations through a single-owner path such as a CI/CD step or a dedicated Kubernetes Job before the feature flag is enabled
- the migration step should fail the rollout if required migrations do not apply cleanly
- **local development** should use a documented one-off command against Docker Postgres so developers can reset/reseed predictably
- avoid "every API pod runs migrations on startup" unless advisory locking, idempotency, and failure semantics are added intentionally

This keeps rollout behavior deterministic even if API replica count changes later.

### 4. Use a hybrid Postgres schema

Do **not** create a separate SQL column for every metric. That will make every metric change expensive.

Use stable relational tables for the fields you will always care about, and `JSONB` for the evolving metrics.

Recommended tables:

- `match_reports`
  - `match_id` PK
  - `lobby_id`
  - `match_kind`
  - `end_reason`
  - `drain_flag`
  - `is_debug`
  - `started_at`, `ended_at`, `duration_ms`
  - `schema_version`
  - `collector_version` or `metric_set_version`
  - `payload_hash`
  - `human_count`
  - `bot_count`
  - `peak_concurrent_humans`
  - `config_snapshot JSONB`
  - `match_metrics JSONB`
- `participant_reports`
  - `match_id` FK
  - `participant_id` (match-scoped id)
  - `PRIMARY KEY (match_id, participant_id)`
  - `session_player_id_hash NULL`
  - `is_bot`
  - `bot_level NULL`
  - `placement`
  - `summary_metrics JSONB`
- `match_events`
  - `id` PK
  - `match_id` FK
  - `ts_ms`
  - `event_seq`
  - `UNIQUE (match_id, event_seq)`
  - `tick NULL`
  - `event_type`
  - `actor_participant_id NULL`
  - `target_participant_id NULL`
  - `payload JSONB`

This gives you:

- clean joins and indexing on stable dimensions
- flexible metrics without migrations
- easy addition/removal of fields inside JSONB

## API Ingestion Guardrails

The metrics ingestion endpoint should be stricter than the current leaderboard handler because payloads can be much larger.

Add:

- `http.MaxBytesReader` or equivalent request-size caps
- maximum participant count and maximum event count per report
- schema-version validation
- allowed event-type validation
- explicit rejection behavior for oversized, malformed, or unsupported payloads
- duplicate handling keyed by `match_id` plus payload hash
- return success for matching duplicates and conflict for mismatched duplicates

This prevents the metrics endpoint from becoming an unbounded ingest surface.

## API Observability

Add API-side metrics and logs for the ingestion path so database failures are visible independently of sender-side retry exhaustion.

Recommended signals:

- `api_match_metrics_reports_total{status}` for accepted, already_stored, validation_error, auth_error, conflict, and internal_error outcomes
- `api_match_metrics_write_failures_total` for Postgres transaction failures
- `api_match_metrics_duplicate_total{result}` for same-hash vs mismatched-hash duplicate handling
- request/write duration histograms for ingestion and transactional persistence

These should complement, not replace, game-server-side dropped-analytics counters/logs.

### 5. Make metric definitions code-configurable

To avoid drastic code changes whenever you add or drop a metric, a metric registry may eventually be useful, but it should **not** be a Phase 1 blocker.

For V1, prefer:

- typed in-memory accumulators
- explicit report structs
- JSON summary objects written from those structs

Then add a registry later only if the metric surface becomes large enough that manual wiring becomes painful.

Even without a registry, most metric additions can still avoid schema migration because the persisted summary payloads live in `JSONB`.

### 6. Keep events curated, not exhaustive

Recommended first event types:

- `kill`
- `death`
- `pickup`
- `respawn`
- `disconnect`
- `bot_decision_summary` or `bot_state_transition`

Avoid logging every input packet or every tick. That volume grows fast and usually does not pay off early.

## Query Strategy

Use JSONB for flexibility, then add SQL views for the handful of metrics that become important.

Examples:

- view for bot profile win rate and average placement
- view for human death reasons by match size
- view for average shots-to-kill by participant type
- materialized view for daily balance aggregates if volume grows

This lets the storage format stay stable while your analysis surface evolves.

## Rollout Phases

### Phase 1

- add Postgres foundation first: driver, config, migrations, an explicit migration runner/command, repository layer, local Docker Postgres support, and basic repository tests
- add a feature flag for match analytics ingestion/reporting so the code can ship before every environment is Postgres-ready
- define the initial analytics env/config surface up front: `MATCH_ANALYTICS_ENABLED`, `DATABASE_URL` (or equivalent secret source), SSL mode / CA behavior, max open conns, max idle conns, connection max lifetime, and request/query timeout defaults
- make environment enablement explicit: local Docker may enable the feature once Postgres is wired, `game-dev` stays disabled by default, and production enables only after managed-Postgres secrets plus migrations are ready
- choose and document the managed-Postgres migration path in Phase 1, preferably CI/CD or a one-off k8s Job owned by the rollout process
- add collector skeleton and match-end report plumbing
- persist `match_reports` and `participant_reports`
- record only summary metrics for kills, deaths, survival, mass, placement, pickups
- support both normal completions and early/abandoned end reasons
- finalize immutable payloads under lock with an exactly-once guard
- include debug matches with explicit tagging
- add bounded retries and matching-duplicate success semantics for match reports
- add ingestion validation and request-size caps
- add minimal app config/secrets needed for environments where the feature flag is enabled, including managed-Postgres connection settings for production and local connection settings for dev
- make local Docker support concrete: add a `postgres` service, healthcheck, named volume, API env wiring, and migration command/docs for bring-up/reset

### Phase 2

- add curated `match_events`
- add bot-specific summaries from [server/internal/game/bot.go](server/internal/game/bot.go)
- add movement/combat derived metrics where hooks already exist
- add deterministic event ordering via mandatory `event_seq`, with optional `tick` kept only for diagnostics

### Phase 3

- add SQL views / dashboards
- promote a few high-value JSONB metrics into dedicated indexed columns only if query pressure justifies it
- consider adding a metric registry only if V1/V2 metric wiring becomes cumbersome

### Phase 4

- add managed-Postgres operational hardening across environments
- verify provider backups, retention, and restore path before or alongside full production launch; SSL requirements and connection limits should already be handled in Phase 1 enablement
- add production secret rotation and incident/restore runbooks
- add pruning/capacity planning for analytics growth

## Specific Files To Touch Later

- [server/internal/game/server.go](server/internal/game/server.go): initialize/reset collector, finalize before `transitionToIdleLocked()`, `markDrainCompleteLocked()`, or other state-clearing early-exit paths, and post signed metrics report
- [server/internal/game/collision.go](server/internal/game/collision.go): combat hooks (`OnKill`, damage/engagement counters if added)
- [server/internal/game/bot.go](server/internal/game/bot.go): bot behavior counters or end-of-match bot summaries
- [api/internal/app/server.go](api/internal/app/server.go): new metrics report endpoint and signature verification reuse
- likely new DB-focused files under `api/internal/app/` for Postgres ingestion/repository code
- local/dev environment wiring such as `docker-compose.yml` and local env files for a developer Postgres instance
- k8s overlay/config wiring such as `k8s/base/api-server/configmap.yaml` plus dev/prod secret injection for analytics env vars
- production app config/secret wiring for managed Postgres credentials instead of self-hosted database manifests

## Recommended Default Decisions

- Postgres lives behind the API only
- production uses managed Postgres; local development uses local Postgres
- `participant_id` is match-scoped, with optional `session_player_id_hash` in V1; true cross-match identity requires a future stable account/client identifier
- hybrid schema with relational metadata + JSONB metrics
- curated event stream, not full raw gameplay logging
- log debug matches too, clearly tagged as debug
- use `time_limit` plus a separate `drain_flag` when both conditions apply
- version every payload with `schema_version`
- keep collector isolated so gameplay code only calls small hook methods
- collector owns durable per-match participant state, not just live lobby references
- use a separate analytics inclusion rule for debug matches rather than reusing Prometheus gameplay-metrics guards
- omit `display_name` by default in V1
