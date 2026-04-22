# Player Transfer, Observer Reattach, and Graceful Drain Plan

## Goals

- Let gameplay participants finish their current match during deploy rollouts.
- Move `player` sessions during intermission with minimal/no perceived downtime.
- Keep deploy-driven player transfers in the same gamemode by default.
- Support voluntary player transfers to other gamemodes during intermission.
- Preserve the correct non-player behavior for `spectator` and `debug_simulation` sessions during drain.
- Ensure explicit player choice overrides drain defaults when requested.

## Why This Plan Changed

This plan originally assumed that every websocket effectively represented a player session. That is no longer true.

The codebase now has three real session types:

- `player`
- `spectator`
- `debug_simulation`

That means graceful drain can no longer be modeled as "all websocket clients are transferred the same way." The drain plan now needs to distinguish:

- **player transfer** between lobbies/pods
- **spectator reattach** to another observable lobby
- **debug simulation termination or explicit restart**, because a live debug match is currently pod-local and not transferable across pods

## Current Implementation Status

This plan is no longer fully greenfield. Some enabling pieces already exist in the codebase because of spectator/debug-session work.

### Already implemented foundations

- `sessionMode` is real and end-to-end: API, JWT, websocket welcome, client store, and refresh flows already distinguish `player`, `spectator`, and `debug_simulation`.
- `spectator` and `debug_simulation` already use separate matchmaking endpoints.
- observers already have distinct identity/state concepts such as `viewerId` and `debugSessionId`.
- the registry/lobby model already carries observer/debug metadata such as `phase`, `match_kind`, `spectator_count`, `max_spectators`, and `debug_session_id`.
- the client reconnect/refresh path is already session-mode aware rather than hardcoded to player-only flows.
- the server already distinguishes gameplay humans from spectators for capacity/liveness purposes.

### Still missing for this plan

- `modeId` / gamemode-aware lobby assignment for ordinary player transfer.
- an explicit handoff protocol such as `handoff_notice` plus a planned handoff close code.
- signed handoff tickets for cross-route drain behavior.
- a dedicated `POST /api/matchmaking/transfer` path for player transfer.
- explicit drain-time spectator reattach semantics.
- explicit drain-time debug-session termination semantics.
- graceful ws-router termination behavior that preserves planned close semantics.

### Planning implication

This document should be read as:

- **reuse the existing session-aware observer architecture**
- **add missing player-transfer and session-aware drain behavior on top of it**

It should not be read as "replace the current spectator/debug implementation with a new model."

## Baseline

Current codebase behavior:

- `sessionMode` is carried through API responses, JWT claims, websocket welcome, client state, and refresh flows.
- `spectator` and `debug_simulation` use separate API endpoints from normal player joins.
- spectators are kept outside `lobby.Players`.
- registry/lobby metadata now includes observer/debug fields such as `phase`, `match_kind`, `spectator_count`, `max_spectators`, and `debug_session_id`.
- the registry is still **not** gamemode-aware for ordinary player transfer; there is no `modeId` yet.
- client reconnect/refresh behavior is now session-mode aware, but it still needs explicit drain/handoff semantics rather than treating every close the same.
- debug simulations are resumable by `debugSessionId` on the current lobby, but there is no cross-pod migration of a live debug match.

## Session-Mode Scope

This plan now splits behavior by session type.

### `player`

This is the original intermission transfer problem:

- no mid-match migration
- same-mode transfer by default during deploy drain
- optional cross-mode transfer when the player explicitly requests it

### `spectator`

Spectators are observers, not participants:

- they should never become players during drain
- they do not need a gamemode choice UI
- they may reattach to another observable lobby after drain
- same-mode observation is a preference, not a hard requirement in v1

### `debug_simulation`

Debug simulations are a separate product track:

- a live debug match is currently tied to one pod/lobby
- there is no state transfer for an in-progress debug simulation
- deploy drain should **not** silently convert a debug session into a player join or ordinary spectator join
- in v1, deploy drain should end the current debug session explicitly and optionally allow a manual or explicit future restart on another pod

## Non-Goals

- No mid-match migration of gameplay state such as position, mass, health, kills, projectiles, or bot internal state.
- No requirement to keep a draining gameplay lobby together as a cohort; players may be split across multiple destination lobbies in the same mode.
- No requirement to preserve the exact same destination pod for all reconnect cases.
- No cross-pod migration of a live `debug_simulation` match in v1.

## Target Behavior By Session Mode

### Player sessions

- **In-match:** no migration; players complete the match on the current pod.
- **Intermission:** a transfer window opens.
- **Deploy drain default:** move players to another ready lobby running the same mode.
- **Voluntary transfer:** if a player explicitly chooses another mode during intermission, honor that choice when capacity exists.

### Spectator sessions

- **In-match / intermission:** spectators continue observing without becoming participants.
- **Deploy drain:** spectators receive an explicit observer handoff/reattach flow.
- **Destination policy:** prefer another observable lobby, ideally in the same mode if available, but do not block observer recovery on same-mode matching in v1.

### Debug simulation sessions

- **In-match:** let the current debug simulation continue while the draining pod is still alive.
- **Deploy drain:** send an explicit "debug session ending" notice; do not promise cross-pod continuation.
- **Post-drain:** the client may offer explicit manual restart or future explicit restart semantics, but not silent automatic migration of the live debug match.

### Generic disconnect

For all session types:

- generic socket errors should first retry the same websocket route while the token is still valid
- only an explicit handoff/termination signal should trigger cross-route behavior

## Required Foundation: Extend the Existing Session-Aware Registry

The registry already became session-aware because of spectator/debug work, but player transfer still needs gamemode metadata.

Keep the existing session-aware fields:

- `phase`
- `match_kind`
- `connected_humans`
- `spectator_count`
- `max_spectators`
- `debug_session_id`

Add player-transfer metadata:

- `modeId` on each lobby assignment
- enough placement metadata to estimate transfer capacity, or a reservation counter
- optional spectator-oriented `spectatable`/ranking signals if current fields prove insufficient

Recommended direction:

- keep pod-level heartbeat/state
- keep lobby assignment records as the source of truth for `lobbyId -> podIP + modeId + matchKind + capacity`
- filter player transfer candidates by `modeId` and non-draining state
- keep debug lobbies excluded from ordinary player placement

## Routing Policy and Precedence

This precedence applies to `player` sessions only:

1. If a player explicitly requests a mode during intermission, route to that requested mode.
2. Else, if transfer reason is deploy drain, enforce same-mode transfer.
3. Else, use normal intermission defaults.

For `spectator` sessions:

- prefer another observable match
- prefer same-mode observation when available, but do not make it a hard failure in v1

For `debug_simulation` sessions:

- do not attempt same-mode or cross-mode migration of the live debug match in v1

## Identity Model

Treat websocket route tokens as transport credentials, not durable identity.

### Player identities

Preserve across transfer:

- stable `playerId`
- display name
- optional durable `accountId` if auth/account support is added later
- optional analytics/session identifier

Do not preserve into the next match:

- position
- mass
- health
- kills
- match-local scoreboard state

### Spectator identities

Preserve across reattach:

- stable `viewerId`
- `sessionMode=spectator`

### Debug simulation identities

Preserve for same-session resume while the source pod still exists:

- stable `viewerId`
- `sessionMode=debug_simulation`
- `debugSessionId`

Important limit:

- `debugSessionId` is currently a pod-local/live-session identifier, not a cross-pod migration token

## Handoff Protocol

Do not rely on websocket close reason text alone for metadata. Keep the close code small and send explicit session-aware handoff details in a dedicated message first.

### New server-to-client message

Add `handoff_notice` with common fields:

- `sessionMode`
- `action`
- `deadlineMs`
- optional `handoffTicket`

Where `action` is one of:

- `player_transfer`
- `spectator_reattach`
- `debug_session_end`

### Player-only fields

- `intent`: `deploy_drain` | `player_requested`
- `modeConstraint`: `same_mode` | `any_mode`
- `currentMode`
- `allowedModes`

### Spectator-only fields

- optional `currentMode`
- optional `preferredMode`
- optional `sourceMatchKind`

### Debug-only fields

- `debugSessionId`
- `reason`: for example `deploy_drain`
- optional explicit `restartSupported=false` in v1 if we want the contract to be unambiguous

### Close semantics

- Use an explicit planned close code, for example `4002`, with reason `handoff_required`.
- Client only enters cross-route handoff behavior if it has seen a valid `handoff_notice` and then receives the explicit close.
- Generic socket errors and generic closes should not trigger transfer/reattach/restart behavior.

## Handoff Tickets

The API should not trust raw client-supplied context during drain handoff.

### Player handoff ticket

The current game server should mint a short-lived signed ticket containing:

- stable `playerId`
- display name
- source `lobbyId`
- source `modeId`
- `sessionMode=player`
- `intent`
- `modeConstraint`
- expiration
- nonce / transfer ID for replay protection

### Spectator handoff ticket

The current game server should mint a short-lived signed ticket containing:

- stable `viewerId`
- source `lobbyId`
- optional source `modeId`
- `sessionMode=spectator`
- expiration
- nonce / transfer ID for replay protection

### Debug simulation

Do not use a drain handoff ticket to imply cross-pod debug-session migration in v1.

If we later support explicit debug-session restart-on-drain semantics, that should be a separate contract from ordinary debug resume.

## API Changes

The original generic transfer endpoint now needs to be scoped more carefully.

### Player transfer endpoint

Add or keep:

- `POST /api/matchmaking/transfer`

This endpoint is for `sessionMode=player`.

Inputs:

- `handoffTicket`
- optional `requestedMode`
- region if still relevant to placement

Outputs:

- fresh websocket route token
- destination `lobbyId`
- destination `modeId`
- `sessionMode=player`
- optional transfer status details for UI

### Spectator reattach endpoint

Prefer reusing the existing spectator path:

- `POST /api/matchmaking/spectate`

Recommended extension:

- allow the spectator endpoint to accept an optional handoff ticket / reattach token during drain-driven observer recovery

Outputs should preserve:

- `sessionMode=spectator`
- `viewerId`

### Debug simulation endpoint

Keep:

- `POST /api/matchmaking/debug-simulate`

Important rule:

- this endpoint should remain start/resume oriented
- deploy drain must not silently turn a terminating debug session into a new cross-pod debug session

## Placement Rules

### Player transfer

- exclude draining pods/lobbies
- filter by destination `modeId`
- destination lobby does not need to preserve cohort membership
- prefer ready lobbies with available transfer capacity

### Spectator reattach

- exclude draining pods/lobbies
- exclude `matchKind=debug_bot_sim` unless explicitly resuming a matching debug session
- prefer observable active/intermission lobbies
- allow idle observer fallback if no active/intermission match is available
- optionally prefer same-mode observation when available

### Debug simulation

- no cross-pod transfer of the live debug match in v1

## Reservation Rules

The reservation problem matters most for `player` transfer.

Recommended direction:

- reserve a gameplay slot in Redis when issuing a player transfer result
- use a short TTL on the reservation
- include reservation identity in the route token or related transfer state
- release/expire the reservation if the websocket is not established in time
- if reservation fails or expires, allow player retry without losing the handoff flow

Spectator reattach does not need the same strict gameplay-slot reservation semantics unless spectator capacity becomes highly contended.

## Game Server Drain Behavior

On `SIGTERM`:

1. Mark draining immediately.
2. Fail readiness and publish draining state to registry.
3. Reject new gameplay joins and new observer attaches.
4. Let the current active match complete naturally.
5. At the appropriate boundary, send session-aware notices:
   - `player_transfer` for players at intermission
   - `spectator_reattach` for spectators
   - `debug_session_end` for debug observers
6. Freeze/extend intermission only for the player-transfer window, not for indefinite observer attachment.
7. Close remaining sockets with the explicit handoff close code after notices are delivered.
8. Wait for successful player transfers or timeout.
9. Force-close stragglers after timeout and finish shutdown.

Important drain rules relative to the current observer architecture:

- spectators should receive notices and be closed cleanly
- spectators should **not** block drain completion just because they are connected observers
- `debug_simulation` observers should receive an explicit terminal message rather than an implicit failed resume
- drain completion should not be modeled as "all websocket sessions transferred"; only player transfer is a first-class success path in v1

## Client Behavior

### Generic reconnect

For all session modes:

- first retry the same websocket route while the current token is still valid

### Explicit handoff actions

- `player_transfer` -> call `POST /api/matchmaking/transfer`
- `spectator_reattach` -> call `POST /api/matchmaking/spectate` with preserved viewer/session identity
- `debug_session_end` -> show a clear terminal message and return to an explicit restart/manual action flow

### Safety rules

- `spectator` refresh must never silently become `player`
- `debug_simulation` refresh must never silently become `player` or ordinary `spectator`
- drain of a debug session must not auto-start a brand-new debug match on another pod unless we explicitly design and approve that behavior later

### Intermission UX rules for players

- normal intermission: player may stay in mode or choose another mode
- drain intermission default: communicate "moving you to another lobby in this mode"
- if the player chooses a different mode during drain, that explicit choice overrides the default
- if requested mode is unavailable, show retry/wait/fallback UI explicitly

### Observer UX rules

- spectator drain should communicate "reattaching you to another observable lobby" or equivalent
- debug-simulation drain should communicate that the current debug session is ending due to deploy/shutdown

## WS Router Shutdown Behavior

The websocket router still needs graceful termination behavior:

- stop accepting new upgrades
- keep existing proxied sockets alive during the grace window
- allow backend-initiated close codes to propagate to the client where possible
- only force-close remaining proxied sockets when the router grace window expires

## Kubernetes and Rollout Hardening

- Keep `maxUnavailable: 0` and `maxSurge: 1` for game-server rollouts.
- Keep drain-aware readiness so terminating pods are removed from new assignment quickly.
- Ensure `terminationGracePeriodSeconds` exceeds:
  - worst-case match completion tail
  - player handoff notice window
  - player transfer/reconnect window
  - observer close window
  - safety margin
- Maintain enough ready capacity per mode to support same-mode player drain transfers.

## Observability and Success Metrics

Track player, spectator, and debug outcomes separately.

Suggested metrics:

- `player_handoff_initiated_total`
- `player_handoff_completed_total`
- `player_handoff_failed_total`
- `spectator_reattach_initiated_total`
- `spectator_reattach_completed_total`
- `spectator_reattach_failed_total`
- `debug_session_end_notified_total`
- `handoff_ticket_invalid_total`
- `handoff_ticket_replay_total`
- `transfer_reservation_created_total`
- `transfer_reservation_failed_total`
- `deploy_drain_same_mode_success_total`
- `player_requested_cross_mode_transfer_total`
- transfer latency percentiles
- mid-match disconnects during rollout
- generic reconnect success rate by `sessionMode`

Success criteria:

- No mid-match player migrations during deploy.
- High player handoff success at intermission.
- Same-mode player transfer succeeds by default on deploy drain.
- Explicit player cross-mode requests are honored when capacity exists.
- Spectators do not get converted into gameplay participants during drain.
- Debug sessions end explicitly and cleanly rather than failing ambiguously.
- Generic non-handoff disconnects mostly recover via same-route reconnect.

## Suggested Implementation Order

1. Add `modeId` to the existing session-aware registry/lobby assignment records.
2. Define a session-aware `handoff_notice` schema with `sessionMode` and `action`.
3. Introduce signed player and spectator handoff tickets.
4. Keep `POST /api/matchmaking/transfer` player-specific and extend spectator reattach through `POST /api/matchmaking/spectate`.
5. Update the client reconnect state machine so only explicit handoff actions trigger cross-route behavior.
6. Update game-server drain sequencing around player transfer, spectator reattach, and explicit debug-session end.
7. Update ws-router graceful termination behavior.
8. Tune Kubernetes grace timings and per-mode capacity.
9. Add player intermission transfer UX and observer drain messaging.
10. Add automated tests for rolling deploy + session-aware drain behavior.

## Validation Strategy

Validate this plan with three complementary layers:

- protocol/backend validation via `WEBSOCKET_LOAD_TEST_BOTS.plan.md`
- focused client integration tests for reconnect and handoff gating in `client/src/engine/network.ts`
- a very small number of browser smoke tests for end-to-end UI wiring

Core scenarios to cover:

- Player active match + pod drain -> match completes -> intermission player handoff -> reconnect success.
- Player drain with no explicit choice -> same-mode destination only.
- Player drain with explicit cross-mode selection -> requested mode honored.
- Player requested mode unavailable -> correct UI/error/retry behavior.
- Spectator drain -> observer reattach path preserves `sessionMode=spectator`.
- Spectators do not block drain completion.
- Debug simulation drain -> explicit debug-session-end behavior, no silent player/spectator fallback.
- Generic socket error (non-handoff) -> same-route reconnect first for every session mode.
- Transfer ticket replay -> rejected.
- Player transfer reservation collision -> retry path succeeds.
- Real client only enters cross-route behavior after `handoff_notice` + explicit handoff close.

