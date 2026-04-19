# Spectate Mode Plan

## Goal

Add a spectate mode that lets us observe human players and bots without spawning a real ship into the match.

The primary motivation is bot debugging and tuning. If we join the game with a normal player entity, bots may react to us, which changes the behavior we are trying to observe. A proper spectator should receive world state and be able to move the camera around or follow targets, but should not:

- exist as a simulated `Player`
- affect bot targeting or combat decisions
- change bot fill behavior
- count as a connected human for adaptive difficulty or lobby capacity
- send gameplay input that influences the match

## Current Constraints

### Server

Today the server treats every connected human as a real participant:

- websocket join goes through `HandleWS`
- auth resolves to a player identity
- `upsertHumanPlayerLocked()` creates or refreshes a `Player`
- snapshots are delivered per connected player with a per-player `you` payload
- human counts are used by bot fill and adaptive difficulty

This means a normal connection is not suitable for passive observation.

### Client

Today the client ties together:

- `localPlayerId`
- camera target
- self highlighting
- HUD / death / minimap / scoreboard "you" behavior

To support spectating, we need to split "who I control" from "what I am watching".

## Options Considered

### Option A: Spectators as flagged `Player` rows

Add something like `IsSpectator` to `Player`, then branch everywhere to exclude spectators from simulation and gameplay.

Pros:

- reuses some existing connection/player structures
- smaller initial structural change

Cons:

- easy to miss simulation or lifecycle branches
- adds spectator-specific checks across many gameplay paths
- higher risk of subtle bugs, such as spectators affecting counts or resets

Recommendation: not preferred.

### Option B: Separate spectator connection store

Keep spectators out of `lobby.Players` entirely. Maintain a separate collection of spectator connections and send them world snapshots without creating a simulated entity.

Pros:

- clean separation between simulated participants and observers
- spectators cannot accidentally affect physics, combat, counts, or bots
- easiest model to reason about over time

Cons:

- requires separate connection lifecycle handling
- snapshot delivery logic needs an additional path

Recommendation: preferred.

### Option C: Client-only spectate while still spawning a ship

Keep the current server model, but let the camera follow another player or bot while the local ship still exists.

Pros:

- fastest to build
- mostly client-only

Cons:

- does not solve the core problem
- local ship still affects bot behavior and match dynamics
- still counts as a connected human participant

Recommendation: useful only as a temporary debug shortcut, not as the real solution.

## Recommended Approach

Implement a first-class spectator role using a separate server-side spectator connection path.

High-level design:

- keep `lobby.Players` for simulated participants only
- add a separate spectator connection store
- allow spectator clients to receive snapshots without creating a `Player`
- ensure spectators do not count toward human participation or player slots
- split client camera target from local controlled player identity
- start with follow-target spectating before adding free camera

## Product Semantics To Lock

Before implementation, we should explicitly lock the intended product behavior.

Recommended version 1 semantics:

- version 1 is split into two distinct debug-oriented tracks:
  - `spectator mode`: observe an existing active or intermission match without affecting it
  - `bot debug simulation`: explicitly start and observe a bot-only debug match from an idle lobby
- in spectator mode:
  - a spectator can observe an existing active or intermission match
  - a spectator can keep the current active match observable after all active human players leave
  - a spectator does not count as a gameplay participant for bot fill, adaptive difficulty, or capacity
  - if the last gameplay participant leaves during an active match, bots may continue simulating until the current match naturally ends so spectators can keep observing
  - after that match ends, do not start the next match unless at least one connected human player exists
  - if intermission ends with spectators connected but no connected human players remaining, transition to an idle-observer state instead of starting a new empty active match
- in bot debug simulation mode:
  - a dev/admin-only observer may explicitly start and observe a bot-only match from an idle lobby
  - this is separate from ordinary spectator joins and is not a public spectating feature
  - starting a bot-only debug match must be an explicit privileged action, not an automatic side effect of spectator presence

This distinction matters because "observe an existing match", "keep an existing match alive", and "start a bot-only debug simulation" are different product behaviors and should not be conflated.

## Access Model To Lock

The initial motivation for this feature is bot debugging and tuning, which has different security implications than public spectating.

Recommended version 1 access model:

- both spectator mode and bot debug simulation are gated behind a concrete dev/admin mechanism:
  - `SPECTATOR_MODE_ENABLED=true`
  - plus an admin/debug secret required on the spectator API request
- the spectator secret must not be baked into the browser bundle as a `VITE_*` client environment variable
- for a dev-only UI, the user should enter the secret manually; for tooling, use a local script or another protected request path
- public spectating can be added later, but it should be treated as a deliberate product decision because it changes privacy and cheating assumptions

Debug-data recommendation:

- basic spectator snapshots should remain gameplay-safe
- bot-debug metadata and overlays should be a separate privileged/debug mode rather than bundled into ordinary spectator payloads by default

## Proposed Rollout

### Phase 1: API, routing, and protocol contract

Define spectator mode at the API and transport boundary first.

Today, real clients enter through the matchmaking/API path and the websocket router. Because of that, spectator mode should not rely on an ad hoc websocket query parameter alone.

Recommended contract:

- keep normal player matchmaking on its existing API surface
- add a separate privileged spectator endpoint such as `POST /api/matchmaking/spectate`
- add a separate privileged debug-simulation endpoint such as `POST /api/matchmaking/debug-simulate`
- include an explicit signed JWT claim such as `session_mode`
- use `sessionMode: "player" | "spectator" | "debug_simulation"` as the authoritative session discriminator in API responses, JWT claims, welcome payloads, and reconnect state
- `viewerMode` may still exist as a client-facing rendering concept, but it is not enough on its own once debug simulation exists
- include an explicit snapshot/welcome state field such as `phase: "idle" | "active" | "intermission"` or `observerState: "idle" | "watching"`
- make `playerId` optional in welcome messages when `sessionMode != "player"`
- include a stable `viewerId` for spectator sessions, or keep JWT `sub` as the internal spectator identity even when `playerId` is absent from client player semantics
- optionally include an initial `cameraTargetId` for spectator sessions
- preserve `sessionMode` through all refresh/rematch/reconnect flows, not just the initial join

Reconnect/refresh rule:

- route refreshes and reconnect-based rematch flows must carry `sessionMode`
- spectator refreshes should go back through the spectator endpoint, not the normal player join endpoint
- debug-simulation refreshes should go back through the debug-simulation endpoint, not the normal player join endpoint
- initial debug start and debug reconnect/refresh must use distinct semantics:
  - initial debug start uses a token/claim such as `debug_simulation_start=true`
  - reconnect/refresh uses an attach/resume token for the existing debug match/session and must not require the lobby to still be idle
  - attach/resume must target an explicit debug-session identity such as `debugSessionId`, not just "whatever debug match is currently in this lobby"
- spectator sessions must never silently fall back to a normal player join on refresh
- store `sessionMode` in route state and/or session state wherever the current match route is refreshed

Routing note:

- if spectate intent is passed only as a websocket query parameter, it may not survive the current routing path unless the router is explicitly updated to preserve it

Files likely affected:

- `api/internal/app/server.go`
- `api/internal/app/matchmaking.go`
- `client/src/engine/types.ts`
- `client/src/engine/network.ts`
- `client/src/pages/Game.tsx`
- `ws-router/internal/router/router.go`

### Phase 2: Server-side spectator role

Add a distinct way to connect as a spectator.

Possible ways to express spectator intent:

- JWT claim such as `session_mode=spectator`
- explicit API join mode
- server-side admin/debug toggle for local observation

Initial recommendation:

- support a simple explicit spectate mode in the authenticated join contract
- keep it easy to use during development

Server changes:

- update websocket join flow to branch between player join and spectator join
- avoid calling `upsertHumanPlayerLocked()` for spectators
- maintain a separate spectator connection collection
- add `MaxSpectators` as a real limit, not just a nice-to-have
- make sure spectator connections participate in drain/shutdown connection handling and notice delivery
- add explicit helper separation for gameplay participants, spectators, observer-liveness, and all connections

Files likely affected:

- `server/internal/game/server.go`
- `server/internal/game/registry.go`

Recommended helper split:

- `connectedGameplayHumans`
- `connectedSpectators`
- `connectedObserversForIdle`
- `allConnectionsForNoticeClose`

Important drain rule:

- spectators should receive drain/shutdown notices and be closed cleanly
- spectators should not block drain completion just because they are connected observers

### Phase 3: Matchmaking, routing, and spectator-capacity selection

Spectator matchmaking needs different selection rules from player matchmaking.

Current concern:

- player-facing matchmaking only targets ready pods
- game pods currently advertise "full" based on gameplay-human capacity
- a full active match may still be spectatable if it has spectator capacity remaining

Version 1 recommendation:

- spectator joins may target full-but-not-draining pods if spectator capacity is still available
- matchmaking should distinguish gameplay capacity from spectator capacity
- registry state or registry payloads should expose enough information for spectator-aware assignment and observability ranking

Possible registry/API approaches:

- keep `state=full` for gameplay joins, but expose additional spectator-capacity metadata
- add a separate "spectatable" signal for non-draining lobbies that can still admit spectators
- enforce `MaxSpectators` on the game server and reflect that capacity to the API/registry

Recommended registry observability fields:

- `phase`
- `match_over`
- `connected_humans`
- `spectator_count`
- `max_spectators`
- optional `spectatable`

With those fields, the API can prefer active/intermission lobbies for spectator joins and only fall back to idle-observer when no currently observable match exists.

Files likely affected:

- `api/internal/app/matchmaking.go`
- `api/internal/app/server.go`
- `server/internal/game/registry.go`
- `server/internal/game/server.go`

### Phase 3b: Dev/admin bot debug simulation start path

Because the feature is primarily for bot debugging, version 1 should include an explicit privileged way to start a bot-only debug match from an idle lobby.

Recommended version 1 shape:

- use a separate endpoint such as `POST /api/matchmaking/debug-simulate`
- require the same admin/debug gate as spectator mode
- only allow this path when the target lobby is idle
- API-side target selection rule:
  - prefer idle, non-draining lobbies with spectator capacity and no active debug match
  - if no such lobby exists, return `503` rather than guessing or reusing a non-idle lobby
  - if websocket startup later fails its authoritative server-side checks, surface a clear "debug lobby unavailable" close/error to the client
- start a bot-only match intentionally; do not piggyback on ordinary spectator join semantics
- the API should not attempt to authoritatively start the match by itself
- recommended authoritative control path:
  - the API issues a signed token with a claim such as `session_mode=debug_simulation`, `debug_simulation_start=true`, and a generated `debugSessionId`
  - the game server validates that claim during `HandleWS`
  - the game server atomically checks `idle + authorized + capacity` before starting the debug match
  - once accepted, the game server records the accepted `debugSessionId` in match state and returns that same server-confirmed value in welcome/snapshot payloads
  - the client should treat the server-confirmed `debugSessionId` as authoritative for later resume attempts
- reconnect/refresh path:
  - once the debug match is active, refreshed tokens should attach/resume the existing debug session rather than requesting `debug_simulation_start=true` again
  - resume tokens should carry the same `debugSessionId`
  - attach/resume tokens must not require the server to pass an idle check
- do not rely on a registry-only idle check in the API as the authoritative start decision; it is stale and race-prone
- starting a debug simulation should set explicit per-match state such as `debugMatch=true` or `matchKind=debug_bot_sim`
- that debug-match state must bypass the usual human-required start/fill/idle rules for the current debug match only
- debug-match state must also reject normal `sessionMode=player` joins while the debug match is active
- normal player matchmaking must exclude lobbies running `matchKind=debug_bot_sim`

Recommended debug controls:

- bot count
- optional deterministic random seed

Recommended v1 seed semantics:

- if `seed` is provided, it should control the match-scoped server RNG for that debug match
- that means it should cover all debug-match randomness driven by that RNG, including bot spawn positions, bot retarget/decision randomness, and collectible placement
- the debug match should use RNG state isolated from the normal lobby/server RNG stream; do not reseed or perturb the shared RNG used by normal matches
- if some subsystem is intentionally left out of seeded determinism, the plan and API should say so explicitly rather than implying full determinism

Debug bot-count semantics:

- `botCount` is a fixed debug-match target, not just an initial spawn hint
- the debug match should spawn exactly the requested clamped bot count
- the server should maintain that debug bot target for the lifetime of the debug match
- this should not rely on normal `fillBotsLocked()` semantics unless that function is explicitly extended to support a debug-target mode

Follow-up knobs, not required for v1:

- bot profile mix / difficulty distribution override
- match duration override

Recommended safeguards:

- keep debug simulation separate from public matchmaking behavior
- disable leaderboard/reporting side effects for debug matches as a hard requirement
- ensure debug matches do not affect normal player-facing state except registry observability
- for unlabeled normal counters/metrics, version 1 should default to not incrementing them for debug matches (for example, do not increment normal `MatchesCompleted` for debug matches)
- make debug-match state explicit in welcome/snapshot payloads for UI labeling
- define a cleanup rule for debug simulation:
  - recommended default: when no spectators remain attached to the debug simulation, start a short `DebugSpectatorGracePeriod`
  - if no spectator reattaches before that grace period expires, end the debug match and return the lobby to idle
  - reconnect/refresh during the grace period should resume the existing debug match rather than starting a new one
  - do not let a debug simulation persist indefinitely with no observers

Files likely affected:

- `api/internal/app/server.go`
- `api/internal/app/matchmaking.go`
- `server/internal/game/server.go`
- `server/internal/game/bot.go`

### Phase 4: Snapshot delivery and explicit spectator welcome

Spectators should receive the world snapshot without being treated as active participants.

Recommended behavior:

- send normal world snapshot data:
  - players
  - objects
  - projectiles
  - kill feed
  - scoreboard
  - timers
- omit `you` for spectators, or make it explicitly optional in spectator mode
- send an explicit spectator welcome shape so the client knows it is intentionally in spectate mode rather than inferring it from missing `you`

This keeps protocol changes minimal while making the semantics clear.

Recommended welcome shape:

- `sessionMode`
- `viewerId`
- optional `debugSessionId`
- optional `playerId`
- optional `cameraTargetId`
- `phase` or `observerState`
- optional `matchKind`
- existing lobby/match identifiers

Server changes:

- extend `broadcastSnapshots()` to include spectator deliveries
- ensure spectator connections do not depend on a backing `Player`
- define how welcome/init payload should look for a spectator
- ensure spectator welcome/init payloads remain compatible with existing player flow

Client changes:

- treat spectator welcome separately from player welcome
- do not call `setLocalPlayerId()` as if a spectator were a spawned player

### Phase 5: Exclude spectators from gameplay counts and define liveness

Spectators must not affect match behavior.

Specifically they should not affect:

- `connectedHumansLocked()`
- `canAdmitHumanLocked()`
- bot fill logic
- adaptive bot difficulty
- lobby "full" reporting
- registry heartbeat/fullness metadata

Lifecycle note:

- connected human spectators should keep the match/session from dropping to idle, even if there are `0` active human players
- in other words, "no active human players" should not by itself force idle when spectators are still connected and observing
- this likely affects idle-transition logic separately from gameplay-participant counts, because spectators should not count as players for bot logic, but should count as live observers for match/session liveness

Version 1 recommendation:

- spectator presence may keep the current active match observable until it ends
- spectator presence does not by itself start a new bot-only simulation from an idle lobby
- if no active players remain and the world is idle, the spectator client should see an explicit idle-observer state rather than silently starting gameplay
- if intermission ends and there are spectators but no connected human players, transition to idle-observer or hold a non-playing observer state instead of calling `resetMatchLocked()` into a new empty match

Precise reset gate:

- bots may continue the currently observed match
- bots do not authorize the next round
- version 1 should only start/reset into a new active match when at least one connected human player exists

Files likely affected:

- `server/internal/game/server.go`
- `server/internal/game/bot.go`
- `server/internal/game/registry.go`

### Phase 6: Split camera target from player identity on the client

The client currently assumes that the local player is also the camera target.

We should introduce separate client state concepts:

- `localPlayerId`: the actual player being controlled, if any
- `cameraTargetId`: the entity currently being followed
- `sessionMode`: `player` | `spectator` | `debug_simulation`
- optional client-facing `viewerMode` if the UI still wants a coarser rendering concept

This lets us:

- spectate without controlling a ship
- follow bots or humans independently of local identity
- avoid incorrect self-highlighting and HUD behavior

Files likely affected:

- `client/src/store/gameStore.ts`
- `client/src/engine/network.ts`
- `client/src/engine/index.ts`
- `client/src/engine/renderer.ts`

### Phase 7: Suppress gameplay input in spectator mode

Input suppression should be an explicit implementation task, not just a validation note.

Recommended spectator input rule:

- spectators do not instantiate normal gameplay input controls
- `NetworkClient.sendInput()` becomes a no-op in spectator mode
- the server ignores or rejects unexpected spectator input on the spectator connection path

This prevents spectators from accidentally behaving like hidden players and keeps the mode semantically clean.

Files likely affected:

- `client/src/engine/index.ts`
- `client/src/engine/input.ts`
- `client/src/engine/network.ts`
- `server/internal/game/server.go`

### Phase 8: Minimal spectator UX

Start with a narrow but useful feature set.

Recommended first version:

- follow one target at a time
- cycle targets with keyboard controls
- show current followed target name
- optionally filter to bots only

Why this first:

- matches the bot-debugging use case
- avoids the complexity of free camera initially
- easy to validate

Potential later additions:

- free camera mode
- click scoreboard to spectate a target
- spectate killer / top player after death
- lock spectate to bots only
- spectate target health/mass overlay

Files likely affected:

- `client/src/engine/renderer.ts`
- `client/src/components/HUD.tsx`
- `client/src/components/Scoreboard.tsx`
- `client/src/components/MiniMap.tsx`

Camera-target rules:

- if the followed target dies but remains in the snapshot, keep following it until it is removed or replaced by explicit user action
- if the followed target disappears from the snapshot, auto-advance to a deterministic fallback target
- if there are no valid targets, center the world view or show an explicit idle spectator state instead of relying on an incidental `players[0]` fallback

### Phase 9: Spectator-specific UI behavior

The current HUD assumes an active participant with a `you` payload.

In spectator mode:

- hide self-only HUD elements
- suppress death/respawn UI
- show a spectator banner or target label
- optionally show followed target info instead of local player info

This should be handled cleanly so the client does not treat missing `you` as an error.

### Phase 10: Validation and safety checks

We should add focused validation for both server and client behavior.

Server validation:

- spectator API/join flow issues tokens and welcome payloads with the correct `sessionMode`
- debug-simulation API path issues tokens with explicit debug-start claims
- debug-simulation API path prefers idle, non-draining, spectator-capable lobbies with no active debug match and returns `503` when none exist
- game server atomically starts a bot-only debug match only when idle + authorized + capacity checks pass
- debug-simulation reconnect/refresh uses attach/resume semantics and does not require idle once the match already exists
- debug attach/resume targets the correct `debugSessionId` and does not accidentally bind to a newer debug match in the same lobby
- game server returns the accepted server-confirmed `debugSessionId` and the client uses that value for future resume attempts
- debug-match state bypasses normal human-required start/fill/idle rules for the current debug match
- debug-match state rejects normal player joins, and normal matchmaking excludes debug lobbies
- spectator join does not create a `Player`
- spectator does not count as connected human
- spectator does not affect bot fill
- spectator does not affect adaptive difficulty
- spectator presence prevents the match/session from idling when there are no active human players
- spectator reconnect/refresh preserves mode and never becomes a normal player join
- spectator matchmaking can target spectatable full lobbies when spectator capacity remains
- spectator-only intermission does not start a new empty active match
- spectator is not included in collision/combat/simulation
- spectator input is ignored or rejected
- spectator connections receive drain/shutdown notices and are closed correctly
- spectator capacity limits are enforced
- spectator/helper counts are kept distinct for gameplay, idle-liveness, and drain/notice handling
- debug matches do not report leaderboard results or pollute normal match metrics/state
- unlabeled normal counters/metrics are not incremented for debug matches in version 1 unless they are first split or labeled explicitly
- debug simulations honor `DebugSpectatorGracePeriod` before returning to idle when no attached spectators remain
- debug matches maintain the requested fixed bot-count target for the duration of the match

Client validation:

- spectator can connect and render the world without `you`
- spectator welcome is handled explicitly rather than inferred from missing player data
- spectator idle-observer vs active-watching state is explicit in protocol data
- reconnect/refresh preserves `sessionMode` and routes through the correct endpoint for spectator vs debug simulation
- reconnect/refresh preserves `debugSessionId` for debug simulations
- camera can follow a chosen target
- target switching works
- followed target disappearance falls back deterministically
- no self-HUD is shown in spectator mode
- gameplay input is suppressed in spectator mode
- renderer still handles match timers, scoreboard, and kill feed correctly

## Suggested Implementation Order

1. Define `sessionMode` in the API request/response, JWT claims, websocket welcome message, and client store/types.
2. Add the separate privileged API surfaces for `spectate` and `debug-simulate`, including explicit idle-lobby selection rules and refresh/reconnect behavior for both.
3. Add server spectator connections outside `lobby.Players`.
4. Add the authoritative game-server debug simulation start path, using signed debug-start claims plus atomic idle/authorization/capacity checks.
5. Add debug attach/resume semantics keyed by `debugSessionId`, plus `DebugSpectatorGracePeriod` cleanup behavior.
6. Deliver snapshots to spectators with `you` omitted and an explicit welcome shape carrying `sessionMode`, `phase`/`observerState`, and debug-match labeling where relevant.
7. Add separate gameplay-participant counts vs observer/liveness counts, including intermission/idle rules and debug-match bypasses.
8. Split client `localPlayerId` from `cameraTargetId` and suppress gameplay input.
9. Add follow-target UX and focused tests.

## Recommended Scope for Version 1

To keep the first pass small and useful:

- do implement `v1 spectator mode` for observing existing matches:
  - true spectator joins
  - follow-target spectating
  - bot-following support
  - spectators kept out of the simulation entirely
  - explicit API/protocol-level spectator contract
  - explicit spectator input suppression
  - spectator mode preserved across reconnect and refresh using `sessionMode`
  - spectator access to observable full lobbies when spectator capacity exists
- do implement `v1 bot debug simulation` for controlled bot-only observation:
  - a separate privileged endpoint to start a bot-only debug match from an idle lobby
  - debug simulation gated behind the same dev/admin access model
  - authoritative start on the game server via signed debug-start claims
  - explicit/manual start semantics, not automatic start from spectator presence
  - mandatory suppression of leaderboard/reporting side effects
  - required v1 knobs: bot count and optional seed only
- do gate version 1 spectator/debug access behind a dev/admin mechanism

- do not implement free camera yet
- do not implement replay/recording yet
- do not auto-start bot-only debug simulation from plain spectator joins
- do not make bot debug simulation a public spectating feature

## Open Questions And Recommended Defaults

### When the last human leaves an active match, should bots keep simulating?

Recommended default:

- yes, let the current active match continue to completion if spectators are observing
- after that match completes, move to idle-observer unless at least one connected human player exists to authorize the next match

This preserves the bot-observation use case without turning spectator mode into an automatic simulation launcher.

### Should bot-only debug simulation be in version 1 or later?

Recommended default:

- include it in version 1 as a separate dev/admin-only track
- keep it distinct from ordinary spectator mode so the semantics stay clear
- require explicit start from an idle lobby rather than automatic launch behavior

### Should spectators join a specific lobby/match, or "any observable match"?

Recommended default:

- version 1 should use "any observable match" via the existing matchmaking flow
- the API should prefer active/intermission observable matches and only fall back to idle-observer if none exist
- targeted lobby/match spectate can be a follow-up once the base role is stable

### Should spectator snapshots include bot-debug metadata?

Recommended default:

- no, not in ordinary spectator payloads
- keep debug overlays and bot-internal metadata behind a separate privileged/debug mode

## Future Follow-Ups

After the basic spectator mode works, the most useful follow-on features would be:

1. Bot-only spectate controls
2. Post-death spectate for normal players
3. Free camera mode
4. Match recording / replay support
5. Bot debug overlays showing profile/level and current decision mode

## Summary

The best way to observe bot behavior without contaminating it is to split the feature into two debug-oriented tracks: a true observer role for existing matches, and a separate explicit bot debug simulation path for bot-only observation.

The recommended design is:

- `spectator mode`:
  - separate spectators from simulated players on the server
  - send them normal world snapshots
  - keep them out of human counts and bot logic
  - split camera targeting from local player identity on the client
  - use a simple follow-target spectate mode for version 1
- `bot debug simulation`:
  - expose a separate privileged path to start a bot-only debug match from an idle lobby
  - keep this distinct from public/player matchmaking and from ordinary spectator joins
  - use it when you want to observe bots without waiting for an existing active match
