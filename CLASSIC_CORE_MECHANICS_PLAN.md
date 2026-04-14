# Classic Core Mechanics Plan

## Scope

- Deliver the base `classic` ruleset that all alternate game modes inherit from.
- Keep formulas tunable via config to support balancing without code refactors.

## Mechanics to Implement

- Replace toughness-based object interaction in `server/internal/game/server.go`: map objects should carry only `mass`, and collision always grants that mass (no toughness gate kill condition).
- Wire migration for toughness removal:
  - Server: replace `Toughness` field with `Mass` on `Collectible` struct in `server/internal/game/server.go`. Remove energy-vs-toughness gate and shard-death logic from `resolveObjectCollisionsLocked`.
  - Protocol: replace `toughness` with `mass` in JSON object payloads. This is a breaking wire change; no backward compatibility needed (single deployment, all clients update together).
  - Client types: replace `toughness: number` with `mass: number` in `WorldObject` in `client/src/engine/types.ts`.
  - Client renderer: replace `toughnessColor()` in `client/src/engine/renderer.ts` with a mass-based visual encoding (e.g., color or size derived from object mass).
- Object mass gain is direct addition (no logarithmic or diminishing transform on object pickup).
- Projectile damage no longer shrinks victim mass; projectiles affect health only.
- Balance object mass distribution so eating `5-10` average objects is approximately equivalent to one fresh-player kill reward, to keep PvP incentives strong.
- Add kill mass transfer (default `45%` of victim mass) for all kill paths in `server/internal/game/server.go`, with kill method irrelevant (projectile and collision both award via same helper).
- Add kill-heal reward in `server/internal/game/server.go`: on credited kill, heal killer by `20%` of current max health (clamped).
- Add passive heal-over-time in `server/internal/game/server.go`: flat health-per-second regeneration independent of max health (same absolute HPS for all players), clamped at each player's max health.
- Change respawn mass in `server/internal/game/server.go` to retain `45%` of pre-death mass with floor at `startingMass`.
- Introduce sublinear max-health scaling vs mass in `server/internal/game/server.go`.
- Lock current-health rescaling rule when max health changes: preserve health percentage, i.e. `newHealth = clamp((oldHealth/oldMaxHealth) * newMaxHealth, 0, newMaxHealth)` for mass-driven max-health changes. Respawn still sets health to full max health.
- Introduce sublinear player radius scaling vs mass in `server/internal/game/server.go` so players do not become visually large too quickly at higher mass.
- Ensure radius scaling is authoritative for gameplay too: the same radius function must drive collision overlap, projectile hit checks, and rendered size so visuals and mechanics remain consistent.
- Replace mass-winner crash logic with simultaneous crash damage (`90%` of each attacker max health by default) in `server/internal/game/server.go`.
- Add crash re-hit cooldown in `server/internal/game/server.go`: once pair `A-B` collides, skip further crash checks for that pair for `0.5s` (configurable).
- Add post-crash knockback impulse (only if both players survive): push both players apart along the separation vector with configurable impulse magnitude. If either player dies from the crash, no knockback (they are dead). This prevents persistent overlap from devolving into sticky forced double-KOs on cooldown expiry.
- Add spawn clearance for respawns: check candidate spawn position against all alive players and reject positions where the respawning player's radius would overlap any existing player. Retry with a new random position up to N attempts; if all attempts fail, allow overlap at the best available position (furthest from nearest player). Additionally add brief spawn invulnerability window (configurable, default `1.0s`) with the following behavior:
  - Harmful collisions involving an invulnerable player are ignored entirely: no player-player crash damage/knockback/cooldown entries, and no projectile-player interactions (projectiles pass through without despawn).
  - Non-harmful collectible overlap remains enabled: invulnerable players can still pick up map objects.
  - The invulnerable player cannot fire projectiles during the invulnerability window.
  - The invulnerable player can move normally.
  - If fallback spawn overlaps one or more players, run a deterministic one-time separation routine at invulnerability end: resolve against nearest-overlap-first with capped push steps, clamp to world bounds, iterate up to K steps, then proceed even if minor overlap remains.
- Extend snapshot player payload in `server/internal/game/server.go` with authoritative `maxHealth` so client health bars use server-calculated scaling.
- Also add `maxHealth` to the `selfState` payload so the local player HUD can display accurate health percentage and bar.

## Implementation Structure

- Extract reusable helpers (see File Organization below for placement):
  - `maxHealthForMass(mass)` -- sublinear health scaling
  - `killMassTransfer(victimMass, killerMass)` -- 45% of victim mass
  - `killHealAmount(killerMass)` -- `0.2 * maxHealthForMass(killerMass)`
  - `passiveHealDelta(dt)` -- flat absolute heal for elapsed tick duration
  - `respawnMass(preDeathMass)` -- 45% retention with floor
  - `radiusForMass(mass)` -- logarithmic sublinear curve
  - `resolveCrashDamage(left, right)` -- simultaneous max-health-based damage
  - `applyCrashKnockback(left, right)` -- separation impulse along overlap axis (survivors only). Zero-separation fallback: if players are at effectively identical coordinates (distance < epsilon), use relative velocity vector as the knockback axis; if velocity is also zero, fall back to a deterministic axis derived from stable player-id ordering (lower ID pushed in +X direction).
  - `findClearSpawnPosition(players, worldBounds, candidateRadius)` -- spawn clearance search
- Add pair collision tracking (for example `map[pairKey]time.Time`) keyed by stable player-id pair to enforce crash cooldown.
- Prune expired pair-cooldown entries periodically (lazily on check or via periodic sweep) to prevent unbounded map growth.
- Track per-player spawn invulnerability expiry timestamp; collision resolvers ignore only harmful interactions for invulnerable players (player-player and projectile-player), allow collectible pickup, gate firing while invulnerable, and run a deterministic one-time separation routine when invulnerability ends if the spawn used overlap fallback.
- Keep this as the authoritative base rules layer that later modes wrap/override minimally.

## File Organization

- Split new code across files within the same `game` package for navigability:
  - `mechanics.go` -- pure formula helpers (`maxHealthForMass`, `radiusForMass`, `killMassTransfer`, `killHealAmount`, `respawnMass`, `passiveHealDelta`). Stateless, easy to unit test.
  - `collision.go` -- crash resolution, projectile collision, pair cooldown tracking, same-tick lethality resolution loop.
  - `bot.go` -- bot AI framework, difficulty ladder, behavior gates, target selection.
  - `server.go` -- retains main game loop (`step`), lobby/match lifecycle, websocket handling, config loading, snapshot building. Orchestrates calls into the above.

## Config and Tuning

- Add env-backed coefficients in `config/game-server.env` and wire parsing in `server/internal/game/server.go`.
- Mirror env entries in `k8s/base/game-server/configmap.yaml`.
- Remove dual sources of truth for legacy knobs:
  - deprecate `STARTING_HEALTH` in favor of `HEALTH_BASE` (keep one-release alias with warning log, then remove),
  - deprecate `PLAYER_RADIUS_SCALE` in favor of `RADIUS_BASE`/`RADIUS_SCALE`/`RADIUS_MASS_SCALE` (keep one-release alias adapter, then remove).
- Explicit config knob inventory (env name -> default):
  - `CRASH_DAMAGE_PCT` -> `0.9` (fraction of attacker max health dealt as crash damage)
  - `CRASH_PAIR_COOLDOWN` -> `0.5s` (minimum time between crash checks for the same pair)
  - `CRASH_KNOCKBACK_IMPULSE` -> `250` (starter magnitude for post-crash separation push; tune from telemetry)
  - `KILL_MASS_TRANSFER_PCT` -> `0.45` (fraction of victim mass awarded to killer)
  - `KILL_HEAL_PCT` -> `0.2` (fraction of killer max health healed on credited kill)
  - `RESPAWN_RETENTION_PCT` -> `0.45` (fraction of pre-death mass kept on respawn)
  - `SPAWN_INVULNERABILITY_DURATION` -> `1.0s`
  - `SPAWN_CLEARANCE_ATTEMPTS` -> `20` (max random position retries before fallback)
  - `PASSIVE_HEAL_PER_SECOND` -> `2.5`
  - `PASSIVE_HEAL_COMBAT_DELAY` -> `1.5s`
  - `HEALTH_BASE` -> `100` (exact max health at `startingMass` after normalization)
  - `HEALTH_SCALE` -> `25` (coefficient for normalized health log curve)
  - `HEALTH_MASS_SCALE` -> `10` (denominator inside normalized health log curve)
  - `RADIUS_BASE` -> `10` (exact radius at `startingMass` after normalization)
  - `RADIUS_SCALE` -> `6` (coefficient for normalized radius log curve)
  - `RADIUS_MASS_SCALE` -> `10` (denominator inside normalized radius log curve)
  - `BOT_DIFFICULTY_MODE` -> `weighted` (one of `fixed`, `weighted`, `adaptive`)
  - `BOT_DIFFICULTY_DISTRIBUTION` -> `L0:10,L1:30,L2:40,L3:20` (weights for weighted mode)
- Add optional metrics in `server/internal/game/server.go` for mass distribution and crash lethality to tune comeback behavior.

### Normalized Curve Definitions (Locked)

- Health curve is normalized around `startingMass` so baseline players are exactly `HEALTH_BASE`:
  - `maxHealth(m) = HEALTH_BASE + HEALTH_SCALE * (ln(1 + m/HEALTH_MASS_SCALE) - ln(1 + startingMass/HEALTH_MASS_SCALE))`
- Radius curve is normalized around `startingMass` so baseline players are exactly `RADIUS_BASE`:
  - `radius(m) = RADIUS_BASE + RADIUS_SCALE * (ln(1 + m/RADIUS_MASS_SCALE) - ln(1 + startingMass/RADIUS_MASS_SCALE))`
- Clamp both outputs to sane minimums to avoid negative values at very low mass due to normalization offset.

## Validation

### Deterministic Unit Tests

- Add/extend targeted tests for progression and collisions under `server/internal/game`.
- Add tests for pair cooldown behavior (no repeated `A-B` crash application during cooldown window).
- Add tests for double-KO crash outcomes and respawn retained mass (each player uses own pre-death mass snapshot).
- Add tests for kill-heal (`+20%` max health, clamped), including suppression during same-tick mutual-lethal outcomes for both crash and projectile exchanges.
- Add tests for passive heal-over-time:
  - equal absolute healing for low-mass and high-mass players over identical durations,
  - no healing above max health cap,
  - no healing while dead/respawning.
- Add tests for max-health rescaling semantics (health percentage preserved across mass-driven max-health changes, respawn restores to full).
- Add tests for radius scaling monotonicity and pacing (radius always increases with mass, but with diminishing growth rate).
- Add collision/hitbox consistency tests so server collision radius and client-rendered radius use the same scaling inputs.
- Add tests for knockback impulse application (only when both survive, correct direction, zero knockback on death, deterministic fallback at identical coordinates).
- Add tests for spawn clearance (no overlap with existing alive players, fallback to best-available on exhausted attempts, and deterministic nearest-overlap-first separation routine after invulnerability).
- Add tests for invulnerability constraints (no firing allowed, object pickup allowed, harmful collisions only are ignored).
- Add tests for overlap-fallback one-time separation (routine runs exactly once at invulnerability end, handles multiple overlaps deterministically, clamps to bounds, then normal collisions resume).
- Add tests for toughness-to-mass wire migration (object payloads contain `mass`, no `toughness`).
- Add tests for `maxHealth` presence in both `snapshotPlayer` and `selfState` payloads.
- Verify updated client and server pair produce valid join/snapshot payloads end-to-end (post-migration compatibility, not backward compatibility with pre-migration clients).

### Offline Balance Simulations

- Build lightweight offline simulation harnesses (not part of CI) that replay N-tick scenarios to measure:
  - ram contact rate vs mass tier,
  - projectile hit rate vs target mass tier,
  - time-to-kill distribution by mass tier,
  - mass economy convergence or drift over match-length runs.

### Runtime Metrics / Telemetry

- Add Prometheus counters/histograms (exported via existing `/metrics` endpoint) for:
  - mass distribution percentiles over time,
  - crash lethality rate,
  - comeback rate (win from below-median mass at mid-match).
- Add client rendering checks for world health bars using `health / maxHealth` for all visible alive players.

## Mechanics Clarifications Locked

- **Current collision behavior:** today collisions are checked each simulation tick by pair iteration in `resolvePlayerCollisionsLocked`; there is no cooldown memory.
- **Planned cooldown behavior:** maintain a per-pair last-collision timestamp and ignore repeat checks until `crashPairCooldown` elapses (default `0.5s`, config-driven).
- **Double-KO respawn rule:** if both die in the same crash, each player respawns with `max(startingMass, respawnRetention * theirOwnPreDeathMass)` independently.
- **Kill-heal rule:** any credited kill grants heal of `0.2 * killerMaxHealthAtKillTime`, capped at max health. For non-mutual kills, heal is computed after kill mass transfer (uses post-transfer max health).
- **Health rescaling on mass change:** preserve health percentage when max health changes from mass gain/loss; only respawn restores to full health.
- **Post-crash knockback:** on non-lethal crash (both survive), apply symmetric separation impulse along the overlap axis. No knockback if either player dies. Zero-separation fallback: relative velocity, then stable player-id ordering.
- **Spawn clearance:** respawn positions must not overlap any alive player; if all N attempts fail, overlap is allowed at the best-available position. Brief invulnerability window (default `1.0s`) prevents immediate re-engagement.
- **Spawn invulnerability behavior:** harmful interactions with invulnerable players are ignored entirely (no crash damage/knockback/cooldown entries; projectiles pass through without despawn). Collectible pickup is still allowed. Invulnerable players cannot fire but can move. If spawned via overlap fallback, run a deterministic one-time nearest-overlap-first separation routine at invulnerability end before normal collisions resume.

## Scoring Ownership (Locked)

- **In-match scoreboard** is computed by the game server (needed for live display during match). This is a lightweight real-time ranking, not the official persistent score.
- **Official persistent scoring formula** is owned by the leaderboard plan/API layer. The server reports raw stats (kills, final mass, damage dealt, etc.) at match end; the API computes official scores.
- Current `scoreboardLocked()` in `server.go` remains for live display but its formula may diverge from the leaderboard's official formula. This is intentional.

## Intentional Design Decisions (Locked)

- **Player speed is mass-independent.** All players move at the same speed regardless of mass. Health scaling benefits from mass are intentionally minor enough that this does not create an insurmountable advantage.
- **Projectile tankiness at high mass is intended.** Sublinear health scaling means high-mass players take slightly more shots to kill, but the gains are small enough not to dominate.
- **Objects are risk-free after toughness removal.** No more "crashed into a dense shard" deaths. Objects are purely positive pickups. This is intentional to simplify the object economy and shift risk to PvP.
- **Object mass gain is direct addition.** No diminishing or logarithmic transform on object pickup; mass is added as-is.
- **Projectile hits affect health only.** Projectile damage no longer reduces victim mass. Mass changes come only from object pickups, kill rewards, and respawn retention.
- **Dead players are not rendered.** Health bars should not be drawn for players that are not drawn. Current dead-player filter (skip rendering) is preserved.

## Double-KO Mass Sequencing (Locked Order)

- Use deterministic `snapshot -> apply kills -> apply deaths -> respawn` ordering in `server/internal/game/server.go`.
- For non-mutual kills, apply kill mass transfer first, recompute max health, then apply kill-heal from post-transfer max health.
- Step order for a crash event where both die:
  1. Snapshot both players' pre-collision mass (`massA0`, `massB0`) and max health.
  2. Apply simultaneous crash damage.
  3. Detect deaths and assign kill credits (both credited in double-KO for default classic policy).
  4. Apply kill mass transfer first using snapshotted masses (`A += killPct * massB0`, `B += killPct * massA0`).
  5. Suppress kill-heal for any player who is lethal in this same crash resolution tick (double-KO should not be survivable via kill-heal).
  6. Mark dead players and persist each dead player's `preDeathMassAfterKillReward` for respawn retention calculation.
  7. On respawn, apply `respawnMass = max(startingMass, respawnRetentionPct * preDeathMassAfterKillReward)`.
- This preserves kill mass reward meaningfully even in mutual kills while still enforcing death penalty through retention and non-rescuable simultaneous lethality.
- Add config guardrails to avoid runaway inflation in mutual kills (for example lower `killPct` or lower `respawnRetentionPct` if telemetry shows drift).
- Apply the same sequencing principles for projectile kills where both players can die in the same tick window: snapshot masses, apply kill rewards by credited kills, then apply death/respawn retention.

## Same-Tick Mutual Lethality Rule (Global)

- Define one explicit global rule in `server/internal/game/server.go`:
  - if two players are both marked lethal within the same resolution tick window (from crash, projectile exchange, or mixed sources), both deaths stand and kill-heal is suppressed for both.
- Kill mass reward remains governed by the locked mass sequencing policy, but health restoration cannot prevent same-tick mutual death outcomes.
- Ensure this rule is implemented in shared death-resolution logic rather than per-damage-type branches to avoid drift between crash and projectile handling.
- Extend suppression to a per-player lethal flag for the tick window:
  - if a player is marked lethal in the current resolution tick, suppress all kill-heal they would otherwise receive from any kills they credited during that same tick (including third-party chains such as `A kills B` while `B kills C`).

## Object Economy Targets (Locked)

- Remove object `toughness` from gameplay logic and config surface; objects should expose `mass` as the only progression-relevant stat.
- Set spawn mass distribution targets such that:
  - `avgFreshKillMassReward ~= killPct * startingMass`
  - `avgObjectsNeededForFreshKillEquivalent` is in `[5, 10]`
- Add balancing checks in tests/metrics to track realized object-to-kill reward ratio over live matches.

## Bot Rework (Classic)

All bot logic lives in `bot.go` (see File Organization). References to `server.go` below mean the orchestration calls in `step()` that invoke bot helpers.

- Update bot utility scoring in `bot.go` so bots value:
  - object pickups by `objectMass / travelCost`,
  - PvP opportunities by expected kill mass reward plus kill-heal survival value,
  - disengage when projected incoming crash/projectile damage is lethal.
- Re-tune bot aggression thresholds against new crash and health model:
  - avoid repeated crash entries during pair cooldown windows,
  - prefer finishing low-health targets when kill reward is likely.
- Add bot-side awareness of respawn retention economy:
  - prioritize denying high-mass players when safe,
  - reduce suicidal trades that only feed opponent retention loops.
- Add instrumentation for bot outcomes (bot K/D, bot median mass, bot-vs-human kill split) to ensure bots remain competitive but not oppressive.
- Add deterministic bot behavior tests where feasible under `server/internal/game`, focused on target-selection decisions under new rules.

## Bot Difficulty Ladder (Locked)

- Implement explicit bot skill levels in `bot.go`:
  - `L0` (dummy): moves in a simple straight-line pattern, no aiming, no shooting, no evasive behavior.
  - `L1` (evasive): moves and avoids threats, does not shoot.
  - `L2` (novice combat): moves and shoots with intentional inaccuracy/noise in aim and movement decisions.
  - `L3` (full): current full bot functionality (best available behavior stack).
- Keep one shared behavior framework with capability gates per level, instead of separate bot codepaths, to avoid maintenance drift.
- Add configurable assignment strategies via `config/game-server.env` and `k8s/base/game-server/configmap.yaml`:
  - fixed level (all bots same level),
  - weighted random distribution (for example `L0:10%, L1:30%, L2:40%, L3:20%`),
  - adaptive mix by lobby population/skill target.
- Add observability slices by bot level (K/D, hit rate, survival time, objective mass gain) so each tier can be tuned independently.

## Health Bars (Client + Protocol)

- Protocol changes:
  - add `maxHealth` to per-player snapshot payload (`snapshotPlayer`) in `server/internal/game/server.go`,
  - add `maxHealth` to self-state payload (`selfState`) in `server/internal/game/server.go`,
  - update `WorldPlayer` in `client/src/engine/types.ts` with `maxHealth: number`,
  - update `SelfState` in `client/src/store/gameStore.ts` with `maxHealth: number`,
  - update HUD in `client/src/components/HUD.tsx` to display health as percentage or bar using `health / maxHealth`.
- Rendering changes:
  - draw lightweight world-space health bars above each visible player in `client/src/engine/renderer.ts`,
  - bar fill should be `clamp(health / maxHealth, 0, 1)` and support team color accents later,
  - bar length should scale linearly with `maxHealth` relative to a baseline (`baseBarWidth * (playerMaxHealth / baselineMaxHealth)`), so +20% max health renders as +20% bar length.
- UX details:
  - do not draw health bars for dead players (they are not rendered),
  - ensure bars remain visually balanced using screen-size/radius-aware clamping (no oversized overlays at small player radii or dense scenes).

## Passive Heal Rules (Locked)

- Passive regen is flat absolute HPS and intentionally not proportional to `maxHealth`.
- Default behavior in `server/internal/game/server.go`:
  - apply each tick to alive players only,
  - `health = min(maxHealth, health + passiveHealPerSecond * dtSeconds)`.
- Expose config controls in `config/game-server.env` and `k8s/base/game-server/configmap.yaml`:
  - `PASSIVE_HEAL_PER_SECOND` (default initial tuning value: `2.5`),
  - optional `PASSIVE_HEAL_COMBAT_DELAY` (time after taking/dealing damage before regen resumes).
- Tuning constraint: `PASSIVE_HEAL_COMBAT_DELAY` should default to at least `crashPairCooldown` (0.5s) or longer (recommended `1.0-2.0s`) to prevent micro-heal windows during sustained crash engagements.

## Radius-Mechanics Consistency (Locked)

- Radius is not cosmetic; it is a shared gameplay primitive.
- The radius output from server-side `radiusForMass` in `server/internal/game/server.go` must be the value used for:
  - player-player crash overlap checks,
  - projectile-vs-player collision checks,
  - client-rendered player circles (via snapshot `radius`).
- Rebalance combat constants after radius curve change (crash damage cadence, projectile speed/radius/damage if needed) based on observed mass-tier fairness metrics.

## Graphics Decoupling Foundation (In Scope, Visual Upgrade Out of Scope)

- Keep this plan focused on mechanics + protocol boundaries, not art polish or final asset pipeline.
- Add a stable render-facing data contract (shape/type/color/size/state) that is derived from gameplay state but does not embed renderer-specific logic into server mechanics.
- Keep gameplay authoritative fields (`mass`, `radius`, `health`, `maxHealth`, `isAlive`, collision-relevant geometry) independent from purely visual presentation fields.
- In client rendering code (for example `client/src/engine/renderer.ts`), route draw decisions through presentation adapters so circles can later be replaced by sprites/meshes without changing combat or simulation code.
- Reserve "nicer graphics" implementation (assets, shaders, animations, stylistic overhaul) for a dedicated follow-up graphics plan.

## Sequencing

- Complete this plan first; alternate modes should depend on these stable base rule primitives.
