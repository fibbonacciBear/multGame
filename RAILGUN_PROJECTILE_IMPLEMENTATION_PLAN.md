# Railgun Projectile Implementation Plan (Revised)

## Objective
Implement a high-performance railgun projectile effect using cached sprite rendering, while preserving gameplay correctness and fixing integration gaps in the current codebase.

This revision addresses four key issues:
- Missing projectile heading/type in client snapshot data.
- Visual tip-origin vs gameplay center-origin mismatch.
- Oversized fixed wake dimensions for current game scale.
- Blend mode strategy for readability (avoid full additive washout).

## Current Constraints
- Client projectile type currently includes: `id`, `x`, `y`, `radius`, `ownerId`, `color` (`client/src/engine/types.ts`).
- Server snapshot projectile payload currently includes the same fields and omits `vx`, `vy`, `type` (`server/internal/game/server.go`).
- Projectiles are currently simulated and collision-checked as center-positioned world objects.
- Rendering pipeline now supports batched additive + source-over passes and can be adapted to sprite blitting.

## Design Decisions

### 1) Projectile Direction Source
Preferred approach:
- Add `vx`, `vy`, and `type` to server `snapshotShot`.
- Mirror those fields in client `Projectile`.

Fallback approach (if protocol change delayed):
- Derive heading from interpolated position delta (`currentPos - priorPos`) with per-id fallback cache.
- Normalize missing/invalid projectile fields in `client/src/engine/network.ts` before buffering/interpolation:
  - default `type` to `"railgun"`
  - if `vx`/`vy` are missing or non-finite, derive/cache heading from recent motion and expose a safe fallback direction
  - never allow render code to receive `NaN` heading inputs

### 2) Coordinate Convention (Tip vs Center)
Keep gameplay semantics unchanged (center-based simulation/collision).
- Server continues sending center `(x, y)`.
- Renderer treats the server projectile position as the visual head/tip anchor for railgun rendering.
- The sprite wake extends backward from that anchor in `-dir`.
- Do not move the visible head forward ahead of the authoritative collision position.
- If any forward offset is introduced later for polish, it must stay `<= projectile.Radius` and be validated against hit perception in playtesting.

This preserves collision honesty while still allowing a tip-anchored wake sprite.

### 3) Blend Strategy
Use two render passes:
- Pass A (`lighter`): wake glow + halo/flash only.
- Pass B (`source-over`): white core thread + diamond head.

Reason: keeps HDR punch while preserving sharp shape readability at high projectile density.

### 4) Sprite Scale and Quality
Do not start with `200px` wake in this game.
Initial tuned baseline:
- Wake length: `72px`
- Outer wake width: `5px`
- Core width: `1px`
- Tip halo radius: `10px`
- Diamond points: `( +5, 0 ), ( 0, -3 ), ( -5, 0 ), ( 0, +3 )`

Support DPR:
- Build sprite canvas at `logicalSize * devicePixelRatio`.
- Scale context once during sprite build.
- The returned sprite contract uses logical units:
  - `originX`, `originY`, `width`, and `height` are logical canvas/world units
  - `canvas.width` / `canvas.height` are backing-pixel dimensions
- Because the main render canvas already applies `ctx.setTransform(dpr, 0, 0, dpr, 0, 0)`, sprite draw calls must use logical destination dimensions (for example `drawImage(..., width, height)`) rather than raw backing-pixel size.
- Cache sprites by:
  - `coreCache` keyed by `{ dpr, type }`
  - `glowCache` keyed by `{ dpr, color, type }`
- Rebuild sprite entries when DPR changes (for example on resize / display migration / browser zoom changes).

### 5) Projectile Type Contract
Do not leave projectile type implicit.
- Add `Type string` to the server `Projectile` model at spawn time.
- Include `type` in `snapshotShot`.
- Mirror `type` in the client `Projectile`.
- Use `"railgun"` as the initial concrete value for the current projectile weapon.

This prevents implementation drift and makes future multi-weapon rendering straightforward.

### 6) Color Strategy
Projectile color is part of gameplay readability and must remain part of the design.
- Use a shared white `coreSprite`.
- Use a bounded color-keyed `glowSprite` cache keyed by projectile/team color.
- Do not rely on per-frame tinting operations in the hot path.
- Concretely:
  - `coreCache`: `{ dpr, type } -> coreSprite`
  - `glowCache`: `{ dpr, type, color } -> glowSprite`

This keeps owner/team identity while preserving the sprite-performance win.

## Implementation Phases

## Phase 1: Data Contract Upgrade
Files:
- `server/internal/game/server.go`
- `client/src/engine/types.ts`
- `client/src/engine/network.ts` (interpolation path)

Tasks:
1. Extend `snapshotShot` with:
   - `VX float64 'json:"vx"'`
   - `VY float64 'json:"vy"'`
   - `Type string 'json:"type"'`
2. Populate new fields in `snapshotProjectilesLocked`.
3. Update client `Projectile` type with `vx`, `vy`, `type`.
4. Normalize projectile data in `client/src/engine/network.ts` before buffering/interpolation:
   - default missing `type` to `"railgun"`
   - preserve `vx`, `vy`, `type` from current snapshot when present
   - if `vx`/`vy` are missing or invalid, derive/cache a safe heading fallback from recent motion
5. Ensure interpolation preserves normalized `vx`, `vy`, `type` from current snapshot while blending position.
6. Set projectile type explicitly at spawn time on the server `Projectile` model (`"railgun"` for now).

Acceptance:
- Client receives projectile heading/type without runtime type errors.

## Phase 2: Sprite Builder Module
Files:
- `client/src/engine/projectileSprites.ts` (new)

Tasks:
1. Add `buildRailgunSprite(options)` returning:
   - `{ canvas, originX, originY, width, height }`
   - `originX`, `originY`, `width`, `height` are logical units, not backing-pixel dimensions
2. Build sprite variants with split caches:
   - shared `coreSprite` from `coreCache` (for source-over pass)
   - color-keyed `glowSprite` from `glowCache` (for additive pass)
3. Use DPR-aware backing canvas creation.
4. Ensure sprite draw sites use logical destination dimensions when calling `drawImage`.
5. Key sprite caches by:
   - `coreCache`: `{ dpr, type }`
   - `glowCache`: `{ dpr, color, type }`
6. Expose tunable constants for wake/halo/diamond dimensions.

Acceptance:
- Sprite generation does not allocate in the frame loop.
- Sprite cache rebuilds correctly when DPR changes.

## Phase 3: Renderer Integration
Files:
- `client/src/engine/renderer.ts`

Tasks:
1. Initialize sprite cache once (module-level lazy init is acceptable).
2. Replace the current procedural railgun geometry in `client/src/engine/renderer.ts` with sprite blits:
   - Compute angle from `atan2(vy, vx)` (fallback to cached heading if near-zero speed).
   - Use server `(x, y)` directly as the railgun head/tip anchor.
   - Draw the wake backward from that anchor via sprite origin.
3. Add conservative viewport culling using precomputed sprite bounds:
   - culling radius / AABB must account for `wakeLength + haloRadius + forwardMargin`
   - do not cull only around the tip position
4. Keep two-pass batch:
   - `lighter`: draw `glowSprite` for all visible projectiles using logical destination dimensions
   - `source-over`: draw `coreSprite` for all visible projectiles using logical destination dimensions
5. Reset canvas state (`globalCompositeOperation`, `globalAlpha`) after pass.
6. Remove or isolate the current procedural wake/head code path so it is not executed for railgun projectiles.

Acceptance:
- No per-projectile gradient/path construction in frame loop.
- No blend-state flicker leaks into player rendering.
- Railgun projectiles are rendered only through the sprite path.

## Phase 4: Visual/Performance Tuning
Files:
- `client/src/engine/projectileSprites.ts`
- `client/src/engine/renderer.ts`

Tasks:
1. Tune wake length/width to world readability.
2. Validate overlap behavior with 50+ projectiles visible.
3. Tune bounded color-keyed sprite cache behavior:
   - `coreCache`: shared by `{dpr,type}`
   - `glowCache`: keyed by `{dpr,type,color}`
   - build-on-demand
   - bounded size / reuse policy if needed

Acceptance:
- Stable framerate under stress scenarios.
- Projectile shape remains legible in crowded combat.

## Test Plan
1. Functional:
   - Fire single projectile: angle follows travel direction.
   - Older / partially upgraded snapshots still render safely without `NaN` rotation or crashes.
   - Visible head aligns with perceived collision point.
   - Rapid fire burst: no duplicate/spawn ghost bullet artifact.
   - Despawn: no lingering wake artifacts.
2. Visual:
   - Overlapping projectiles preserve distinct white cores.
   - Tip flash is visible but not overpowering.
   - Trail does not pop at screen edges due to aggressive culling.
3. Performance:
   - Compare before/after with many projectiles on screen.
   - Confirm no per-frame `createLinearGradient/createRadialGradient` in hot path.
4. Regression:
   - Player/body alpha remains stable (no transient transparency flicker).
   - Sprite cache rebuilds correctly after DPR change.
   - DPR-backed sprites render at the intended logical size, without double-scaling.

## Risks and Mitigations
- Risk: Heading jitter at low speed.
  - Mitigation: cache last valid heading per projectile id and reuse when `hypot(vx, vy)` is below threshold.
- Risk: Visual head appears to hit before server collision.
  - Mitigation: anchor the visible head at authoritative `(x, y)` and avoid forward offset beyond collision radius.
- Risk: Protocol mismatch during rollout.
  - Mitigation: feature-gate render path; normalize projectile snapshots in `network.ts`; fallback to position-delta heading until all fields are available.
- Risk: Sprite appears blurry on high-DPI.
  - Mitigation: DPR-scaled offscreen canvas generation.
- Risk: DPR-backed sprite is drawn at backing-pixel size and appears double-scaled.
  - Mitigation: define sprite metadata in logical units and require `drawImage` destination sizing in logical dimensions.
- Risk: Trail pops near viewport edges.
  - Mitigation: use conservative culling bounds based on full sprite extents, not just tip position.
- Risk: Mixed rendering paths reintroduce hot-path allocations.
  - Mitigation: explicitly remove or bypass the current procedural railgun renderer once sprite mode is enabled.

## Deliverables
- New plan file in repo root (`RAILGUN_PROJECTILE_IMPLEMENTATION_PLAN.md`).
- Data contract updates for projectile heading/type.
- New sprite builder module.
- Renderer switched to batched additive glow + source-over core sprite passes.
- Procedural railgun path removed or bypassed for railgun projectiles.
