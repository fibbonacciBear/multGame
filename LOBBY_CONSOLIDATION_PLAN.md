# Lobby Consolidation Plan

## Goal

Consolidate under-populated lobbies between matches so players end up in fuller, more competitive games and fewer pods spend time running redundant half-bot lobbies.

Example target outcome:

- two lobbies of the same `modeId` and same `matchKind`
- each currently has `5` real players and `5` bots
- after intermission, players are consolidated into one fuller lobby instead of both lobbies continuing separately

## Why This Is Separate From Drain

Lobby consolidation and graceful drain share some underlying mechanics, but they are different features.

- **Graceful drain** is deploy/shutdown driven.
- **Lobby consolidation** is population optimization driven.

They will likely reuse the same kinds of primitives:

- intermission-only movement
- planned handoff notices
- transfer/reattach tickets
- reservation-aware placement
- session-aware handling for `player`, `spectator`, and `debug_simulation`

But consolidation needs different policy rules, thresholds, and anti-thrashing behavior, so it should be planned separately.

## Core Product Semantics

### What consolidation is

- a controlled intermission-time move of players from one under-populated lobby to another compatible lobby
- a matchmaking optimization, not a deploy fallback

### What consolidation is not

- not a mid-match migration
- not a spectator feature
- not a debug-simulation migration mechanism
- not an excuse to rebalance arbitrary incompatible lobbies

## Hard Compatibility Rules

Two lobbies are eligible for consolidation only if they are compatible.

Minimum hard requirements:

- same `modeId`
- same `matchKind`
- same region / placement bucket if region remains meaningful
- both are non-draining
- both are not in the middle of an active match

Recommended additional policy gate:

- the `matchKind` must be explicitly marked `consolidatable=true`

That gives us a clean distinction between:

- **compatibility**: same `modeId` + same `matchKind`
- **policy**: even if compatible, some match kinds may still opt out of consolidation

## Session-Mode Rules

### `player`

This is the primary target of consolidation.

- only `player` sessions are actively consolidated
- only during intermission
- players should receive an explicit planned handoff rather than an arbitrary reconnect

### `spectator`

Spectators should not drive consolidation, but they do need clear behavior once a source lobby is being consolidated.

Recommended version 1 behavior:

- spectators observing a consolidating lobby receive an observer reattach flow
- they reattach to the destination lobby if appropriate, or to another compatible observable lobby
- spectators never become participants

### `debug_simulation`

Do not consolidate debug sessions as part of ordinary lobby consolidation.

- `matchKind=debug_bot_sim` should normally be `consolidatable=false`
- a live debug simulation is not part of player population optimization
- if a future non-normal match kind is meant to consolidate, that should be an explicit decision, not an accidental consequence of broad matching rules

## Non-Goals

- No mid-match migration of gameplay state.
- No merging of incompatible `modeId` or `matchKind` values.
- No cross-pod migration of live debug simulations.
- No lobby consolidation triggered by spectators alone.
- No repeated merge/split churn every round.

## When Consolidation Should Happen

Recommended default:

- only evaluate consolidation during intermission
- only after end-of-match results are finalized
- only before the next round would normally begin

This keeps the feature predictable and avoids moving players while they are actively playing.

## Candidate Selection Rules

Consolidation needs two decisions:

1. which lobbies should be considered under-populated
2. which compatible lobby should be the destination

### Source-lobby criteria

Recommended version 1 heuristics:

- low real-player population relative to `MaxPlayers`
- same `modeId`
- same `matchKind`
- not draining
- not a debug match
- currently in intermission

### Destination-lobby criteria

Recommended version 1 heuristics:

- compatible with the source lobby
- enough capacity to accept incoming players
- not draining
- currently in intermission
- prefer fuller or healthier destination lobbies over emptier ones

### Simple first-pass policy

Prefer moving one source lobby into one destination lobby rather than partially redistributing both.

That is easier to reason about operationally and in the UI.

## Population Rules

Recommended consolidation should optimize for real-player density, not total lobby occupancy including bots.

Suggested reasoning:

- two lobbies with `5 real + 5 bots` each are effectively two medium-population lobbies, not two full real-player lobbies
- one lobby with `10 real` players is often a better player experience than two separate `5 real` lobbies padded with bots

Recommended version 1 rule:

- use real-player counts as the main consolidation signal
- treat bots as fill that can disappear naturally when players are merged

## Destination Semantics

When consolidation happens:

- the destination lobby remains the authoritative next-round lobby
- players from the source lobby transfer there during intermission
- the source lobby returns to idle once its transferable sessions have left

Players do not need to remain grouped by original lobby after consolidation.

## Transfer Mechanics

Consolidation should reuse the same planned-handoff primitives that the drain plan introduces.

Recommended direction:

- send a consolidation-specific handoff notice during intermission
- issue signed handoff tickets for moving `player` sessions
- use reservation-aware placement so destination capacity is not over-allocated

Suggested handoff action:

- `lobby_consolidation`

Suggested common handoff fields:

- `sessionMode`
- `action=lobby_consolidation`
- `reason=population_optimization`
- `sourceLobbyId`
- `destinationLobbyId`
- `deadlineMs`
- optional handoff ticket

## Reservation Rules

Consolidation should reserve destination gameplay slots before telling clients to move.

Recommended version 1 behavior:

- reserve slots for players in the destination lobby
- use short TTL reservations
- only announce consolidation when the destination can actually accept the expected incoming player count
- if reservations fail, abort consolidation for that cycle rather than partially moving users unpredictably

## Anti-Thrashing Rules

This is one of the most important parts of the feature.

Without hysteresis, lobbies may repeatedly merge, refill, split conceptually, and merge again.

Recommended version 1 safeguards:

- only evaluate consolidation at intermission
- require under-population to persist for at least one round boundary or a short stability window
- apply a cooldown after consolidation before the resulting lobby can be reconsidered
- prefer leaving stable full-enough lobbies alone

## Match-Kind Policy

Do not special-case `normal` forever. Instead, make consolidation capability an explicit per-match-kind policy.

Recommended model:

- each `matchKind` has:
  - compatibility rule: same `matchKind`
  - policy flag: `consolidatable`

Version 1 defaults:

- `normal` -> `consolidatable=true`
- `debug_bot_sim` -> `consolidatable=false`
- future match kinds decide explicitly

This supports the rule you wanted:

- consolidate lobbies with the same `matchKind`, even when that kind is not `normal`

but still lets us opt out specific kinds when needed.

## Registry / Matchmaking Data Needs

This plan depends on registry/lobby data being rich enough to make safe consolidation decisions.

Recommended required fields:

- `modeId`
- `matchKind`
- `phase`
- `connectedHumans`
- optional total players / bot count if useful
- capacity information
- draining state
- optional `consolidatable` policy signal if not hardcoded centrally

## Spectator Handling

If a spectator is attached to a source lobby that is consolidating:

- do not convert them into a player
- do not treat them as a reason to block consolidation
- reattach them as an observer to the destination or another compatible observable lobby

This is a secondary concern compared to player consolidation, but it should be defined explicitly.

## UX Semantics

For players:

- during intermission, communicate that the next round is moving to a fuller lobby
- keep messaging positive and low-friction
- avoid exposing raw infrastructure concepts like pods

For spectators:

- communicate that observation is reattaching to the consolidated lobby

## Validation Scenarios

Core scenarios to cover:

- two compatible under-populated lobbies in intermission -> one consolidates into the other
- incompatible `matchKind` values -> no consolidation
- incompatible `modeId` values -> no consolidation
- `consolidatable=false` match kind -> no consolidation
- destination lacks capacity -> no consolidation
- consolidation reservations fail -> no partial move
- spectators on source lobby reattach as observers
- debug simulations are excluded from ordinary consolidation
- no consolidation occurs mid-match
- repeated rounds do not thrash between consolidation decisions

## Suggested Implementation Order

1. Add `modeId` as a first-class registry/lobby field if not already present.
2. Define per-`matchKind` consolidation policy, including `consolidatable`.
3. Add lobby compatibility evaluation: same `modeId` + same `matchKind`.
4. Define consolidation heuristics and anti-thrashing rules.
5. Reuse or extend planned handoff primitives from the drain/transfer work.
6. Add reservation-aware destination locking for players.
7. Add spectator reattach behavior for consolidating lobbies.
8. Add focused server/client tests and a small end-to-end smoke path.

## Relationship To Other Plans

This plan depends conceptually on:

- `GRACEFUL_DRAIN_AND_GAMEMODE_TRANSFER_PLAN.md` for intermission handoff/transfer primitives
- `SPECTATE_MODE_PLAN.md` for correct spectator/debug-session behavior

It should remain a separate plan because the policy and product decisions are distinct from deploy drain behavior.

## Summary

Lobby consolidation is a good follow-on feature, but it should be conservative.

The recommended version 1 model is:

- intermission-only
- player-focused
- compatible lobbies only: same `modeId` and same `matchKind`
- additionally gated by per-`matchKind` `consolidatable` policy
- no ordinary consolidation of debug simulations
- reuse planned handoff and reservation primitives rather than inventing a second movement mechanism
