import { describe, expect, it } from "vitest";
import { normalizeSnapshotProjectiles } from "./network";
import type { SnapshotMessage } from "./types";

function snapshotWithProjectiles(projectiles: unknown[]): SnapshotMessage {
  return {
    type: "snapshot",
    serverTime: Date.now(),
    world: { width: 4000, height: 4000 },
    matchId: "match-1",
    matchOver: false,
    timeRemainingMs: 1000,
    intermissionRemainingMs: 0,
    players: [],
    objects: [],
    projectiles: projectiles as SnapshotMessage["projectiles"],
    killFeed: [],
    scoreboard: [],
  };
}

describe("normalizeSnapshotProjectiles", () => {
  it("defaults missing projectile protocol fields and derives a safe heading", () => {
    const previous = snapshotWithProjectiles([
      {
        id: "shot-1",
        x: 10,
        y: 10,
        vx: 1,
        vy: 0,
        radius: 5,
        ownerId: "player-1",
        type: "railgun",
        color: "#68e1fd",
      },
    ]);
    const current = snapshotWithProjectiles([
      {
        id: "shot-1",
        x: 16,
        y: 14,
        radius: 5,
        ownerId: "player-1",
        color: "#68e1fd",
      },
    ]);

    const normalized = normalizeSnapshotProjectiles(current, previous);

    expect(normalized.projectiles[0]).toMatchObject({
      id: "shot-1",
      type: "railgun",
      vx: 6,
      vy: 4,
    });
    expect(Number.isFinite(normalized.projectiles[0].vx)).toBe(true);
    expect(Number.isFinite(normalized.projectiles[0].vy)).toBe(true);
  });
});
