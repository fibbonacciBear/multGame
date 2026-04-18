# Intermission Transfer and Graceful Drain Plan

## Goals

- Let players finish their current match during deploy rollouts.
- Move players during intermission with minimal/no perceived downtime.
- Keep deploy-driven transfers in the same gamemode by default.
- Support voluntary intermission transfers to other gamemodes.
- Ensure explicit player choice overrides drain defaults when requested.
- Preserve the parts of player identity that matter across transfer while still treating the next match as a fresh gameplay round.

## Baseline

- Players currently stay on the same lobby/pod while the websocket remains connected.
- Reassignment happens on reconnect/rematch flows.
- The API currently assigns a random ready lobby and does not understand gamemodes.
- The client currently refreshes matchmaking on some reconnect paths, so generic disconnects can lead to reassignment.
- The game server has a basic draining flag, but it does not yet run an explicit handoff protocol.

## Non-Goals

- No mid-match migration of gameplay state such as position, mass, health, kills, or projectiles.
- No requirement to keep a draining lobby together as a cohort; players may be split across multiple destination lobbies in the same mode.
- No requirement to preserve the exact same destination pod for all reconnect cases.

## Target Behavior

- **In-match:** no migration; players complete the match on the current pod.
- **Intermission:** a transfer window can open.
- **Deploy drain default:** move players to another ready lobby running the same mode.
- **Voluntary transfer:** if a player explicitly chooses another mode during intermission, honor that choice when capacity exists.
- **Generic disconnect:** first retry the same websocket route; do not treat every close as a rematch or transfer.

## Required Foundation: Mode-Aware Registry

The current registry is not mode-aware, so same-mode drain is not implementable until routing metadata is extended.

Add mode metadata to the published registry/lobby records:

- `modeId` on each lobby assignment.
- `state` on each pod/lobby assignment: `ready` | `full` | `draining`.
- Enough capacity metadata to estimate available transfer slots, or a separate reservation counter.

Recommended direction:

- Keep pod-level heartbeat/state.
- Make lobby assignment records the source of truth for `lobbyId -> podIP + modeId + capacity`.
- Filter transfer candidates by `modeId` and non-draining state.

## Routing Policy and Precedence

1. If a player explicitly requests a mode during intermission, route to that requested mode.
2. Else, if transfer reason is deploy drain, enforce same-mode transfer.
3. Else, use normal intermission defaults.

In short: explicit player choice wins over drain defaults.

## Identity Model

Treat websocket route tokens as transport credentials, not the durable identity.

Preserve across transfer:

- stable `playerId`
- display name
- optional durable `accountId` if account/auth support is added later
- stable analytics/session identifier if present

Do not preserve across transfer into the next match:

- position
- mass
- health
- kills
- match-local scoreboard state

Recommended direction:

- Introduce a durable player/session identity that survives route refresh and intermission transfer.
- Keep issuing a fresh websocket route token per assignment.
- Ensure the transfer path reuses the same durable `playerId` instead of minting a brand-new player identity.
- Keep the identity contract future-friendly so an optional `accountId` can also survive cross-pod transfer later without redesigning the handoff flow.

## Handoff Protocol

Do not rely on websocket close reason text alone for metadata; keep the close code small and send transfer details in a dedicated websocket message first.

### New server-to-client message

Add `handoff_notice` with:

- `intent`: `deploy_drain` | `player_requested`
- `modeConstraint`: `same_mode` | `any_mode`
- `currentMode`
- `allowedModes`
- `deadlineMs`
- `handoffTicket`

### Close semantics

- Use an explicit planned close code, for example `4002`, with reason `handoff_required`.
- Client only enters transfer flow if it has seen a valid `handoff_notice` and then receives the explicit handoff close.
- Generic socket errors and generic closes should not trigger transfer.

### Handoff ticket

The transfer API should not trust raw client-supplied mode/intent/player context. Instead, the current game server should mint a short-lived signed handoff ticket containing:

- stable `playerId`
- display name
- source `lobbyId`
- source `modeId`
- `intent`
- `modeConstraint`
- expiration
- nonce / transfer ID for replay protection

Recommended direction:

- Use a signed ticket plus single-use nonce stored in Redis.
- Expire tickets quickly, for example within 15 to 30 seconds.

## Client Behavior

### Reconnect rules

- On generic disconnect/error: retry the same websocket route first while the current route token is still valid.
- On explicit handoff close (`4002` + valid cached `handoff_notice`): call the transfer API.
- Only fetch a fresh route from transfer/rematch APIs when the server explicitly instructs the client to do so.

### Intermission UX rules

- Normal intermission: player may stay in mode or choose another mode.
- Drain intermission default: communicate "moving you to another lobby in this mode."
- If player chooses a different mode during drain, that explicit choice overrides the default.
- If requested mode is unavailable, show retry/wait/fallback UI explicitly.

## Transfer API

Add:

- `POST /api/matchmaking/transfer`

Inputs:

- `handoffTicket`
- optional `requestedMode`
- region if still relevant to placement

Outputs:

- fresh websocket route token
- destination `lobbyId`
- destination `modeId`
- optional transfer status details for UI

### Validation rules

- Verify ticket signature, expiry, and single-use nonce.
- Verify `requestedMode` against ticket constraints.
- If `modeConstraint = same_mode` and no explicit mode is requested, force `currentMode`.
- If the player explicitly requests another mode, use that mode if capacity exists.

### Placement rules

- Exclude draining pods/lobbies.
- Filter by destination `modeId`.
- Destination lobby does not need to preserve cohort membership; players can be split across lobbies.
- Prefer ready lobbies with available transfer capacity.

### Reservation rules

To avoid many players receiving routes to the same nearly-full destination and then failing on websocket connect, add a short-lived reservation step.

Recommended direction:

- Reserve a slot in Redis when issuing a transfer result.
- Use a short TTL on the reservation.
- Include reservation identity in the route token or related transfer state.
- Release/expire the reservation if the websocket is not established in time.
- If reservation fails or expires, allow the client to retry transfer without losing the handoff flow.

## Game Server Drain Behavior

On `SIGTERM`:

1. Mark draining immediately.
2. Fail readiness and publish draining state to registry.
3. Reject new joins.
4. Let an active match complete naturally.
5. When intermission starts, send `handoff_notice` to connected players.
6. Freeze/extend the intermission countdown as needed while the handoff window is active.
7. Close remaining sockets with the explicit handoff close code after the notice has been delivered.
8. Wait for successful transfers or handoff timeout.
9. Force-close stragglers after timeout and finish shutdown.

Important behavior changes relative to the current implementation:

- Drain completion should not mean only "match over"; it should mean either all relevant players have left/transferred or the timeout has elapsed.
- Existing connected players should receive a planned handoff path, not just a generic shutdown notice.

## WS Router Shutdown Behavior

The websocket router currently hard-closes on shutdown. Replace that with graceful termination:

- stop accepting new upgrades
- keep existing proxied sockets alive during the grace window
- allow backend-initiated close codes to propagate to the client where possible
- only force-close remaining proxied sockets when the router grace window expires

## Kubernetes and Rollout Hardening

- Keep `maxUnavailable: 0` and `maxSurge: 1` for game-server rollouts.
- Keep drain-aware readiness so terminating pods are removed from new assignment quickly.
- Ensure `terminationGracePeriodSeconds` exceeds:
  - worst-case match completion tail
  - handoff notice window
  - transfer/reconnect window
  - safety margin
- Maintain enough ready capacity per mode to support same-mode drain transfers.

## Observability and Success Metrics

Track:

- `handoff_initiated_total`
- `handoff_completed_total`
- `handoff_failed_total`
- `handoff_ticket_invalid_total`
- `handoff_ticket_replay_total`
- `transfer_reservation_created_total`
- `transfer_reservation_failed_total`
- `deploy_drain_same_mode_success_total`
- `player_requested_cross_mode_transfer_total`
- transfer latency percentiles
- mid-match disconnects during rollout
- generic reconnect success rate

Success criteria:

- No mid-match migrations during deploy.
- High handoff success at intermission.
- Same-mode transfer succeeds by default on deploy drain.
- Explicit cross-mode requests are honored when capacity exists.
- Generic non-handoff disconnects mostly recover via same-route reconnect.

## Suggested Implementation Order

1. Add mode metadata to registry/lobby assignment records.
2. Introduce a durable player/session identity separate from websocket route tokens.
3. Define `handoff_notice` schema, handoff close code, and signed handoff ticket format.
4. Update the client reconnect state machine so only explicit handoff triggers transfer.
5. Implement `POST /api/matchmaking/transfer` with validation and mode-aware placement.
6. Add short-lived destination reservations to reduce transfer races.
7. Update game-server drain sequencing around intermission handoff.
8. Update ws-router graceful termination behavior.
9. Tune Kubernetes grace timings and per-mode capacity.
10. Add intermission mode-selection UX and unavailable-state handling.
11. Add automated tests for rolling deploy + transfer behavior.

## Validation Strategy

Validate this plan with three complementary layers:

- protocol/backend validation via `WEBSOCKET_LOAD_TEST_BOTS.plan.md`
- focused client integration tests for reconnect and handoff gating in `client/src/engine/network.ts`
- a very small number of browser smoke tests for end-to-end UI wiring

Core scenarios to cover:

- Active match + pod drain -> match completes -> intermission handoff -> reconnect success.
- Drain with no explicit player choice -> same-mode destination only.
- Drain with explicit cross-mode selection -> requested mode honored.
- Requested mode unavailable -> correct UI/error/retry behavior.
- Generic socket error (non-handoff) -> same-route reconnect first.
- Transfer ticket replay -> rejected.
- Transfer reservation collision -> retry path succeeds.
- Real client only enters transfer flow after `handoff_notice` + explicit handoff close.

