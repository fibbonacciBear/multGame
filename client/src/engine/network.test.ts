import { describe, expect, it, vi } from "vitest";
import { NetworkClient, normalizeSnapshotProjectiles } from "./network";
import type { MatchJoinResponse, SnapshotMessage } from "./types";

function snapshotWithProjectiles(projectiles: unknown[]): SnapshotMessage {
  return {
    type: "snapshot",
    serverTime: Date.now(),
    world: { width: 4000, height: 4000 },
    matchId: "match-1",
    phase: "active",
    matchKind: "normal",
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

function snapshotWithPlayers(serverTime: number, players: SnapshotMessage["players"]): SnapshotMessage {
  return {
    type: "snapshot",
    serverTime,
    world: { width: 4000, height: 4000 },
    matchId: "match-1",
    phase: "active",
    matchKind: "normal",
    matchOver: false,
    timeRemainingMs: 1000,
    intermissionRemainingMs: 0,
    players,
    objects: [],
    projectiles: [],
    killFeed: [],
    scoreboard: [],
  };
}

function makeClientForInterpolationTests() {
  const match: MatchJoinResponse = {
    wsUrl: "ws://localhost/ws/test",
    lobbyId: "lobby-test",
    token: "invalid.jwt.token",
    sessionMode: "player",
  };
  return new NetworkClient(match);
}

describe("NetworkClient.getInterpolatedSnapshot", () => {
  it("does not interpolate player position across alive-state transitions", () => {
    const client = makeClientForInterpolationTests() as unknown as {
      snapshotBuffer: SnapshotMessage[];
      getInterpolatedSnapshot: () => SnapshotMessage | undefined;
    };
    client.snapshotBuffer = [
      snapshotWithPlayers(1000, [
        {
          id: "p1",
          name: "Pilot",
          x: 100,
          y: 120,
          vx: 0,
          vy: 0,
          mass: 10,
          radius: 10,
          angle: 0,
          health: 0,
          maxHealth: 100,
          isAlive: false,
          respawnInMs: 1000,
          isBot: false,
          color: "#68e1fd",
        },
      ]),
      snapshotWithPlayers(1100, [
        {
          id: "p1",
          name: "Pilot",
          x: 3300,
          y: 3400,
          vx: 0,
          vy: 0,
          mass: 10,
          radius: 10,
          angle: 0,
          health: 100,
          maxHealth: 100,
          isAlive: true,
          respawnInMs: 0,
          isBot: false,
          color: "#68e1fd",
        },
      ]),
    ];

    vi.spyOn(Date, "now").mockReturnValue(1150);
    const interpolated = client.getInterpolatedSnapshot();
    vi.restoreAllMocks();

    expect(interpolated).toBeDefined();
    expect(interpolated?.players[0]).toMatchObject({
      x: 3300,
      y: 3400,
      isAlive: true,
    });
  });

  it("continues to interpolate while player stays alive", () => {
    const client = makeClientForInterpolationTests() as unknown as {
      snapshotBuffer: SnapshotMessage[];
      getInterpolatedSnapshot: () => SnapshotMessage | undefined;
    };
    client.snapshotBuffer = [
      snapshotWithPlayers(1000, [
        {
          id: "p1",
          name: "Pilot",
          x: 100,
          y: 100,
          vx: 0,
          vy: 0,
          mass: 10,
          radius: 10,
          angle: 0,
          health: 100,
          maxHealth: 100,
          isAlive: true,
          respawnInMs: 0,
          isBot: false,
          color: "#68e1fd",
        },
      ]),
      snapshotWithPlayers(1100, [
        {
          id: "p1",
          name: "Pilot",
          x: 200,
          y: 300,
          vx: 0,
          vy: 0,
          mass: 10,
          radius: 10,
          angle: 0,
          health: 100,
          maxHealth: 100,
          isAlive: true,
          respawnInMs: 0,
          isBot: false,
          color: "#68e1fd",
        },
      ]),
    ];

    vi.spyOn(Date, "now").mockReturnValue(1150);
    const interpolated = client.getInterpolatedSnapshot();
    vi.restoreAllMocks();

    expect(interpolated).toBeDefined();
    expect(interpolated?.players[0]).toMatchObject({
      x: 150,
      y: 200,
      isAlive: true,
    });
  });
});

describe("NetworkClient reconnect behavior", () => {
  it("refreshes direct debug sessions on close", async () => {
    const refreshMatch = vi.fn().mockResolvedValue({
      wsUrl: "ws://localhost:8080/ws?lobby=lobby-test&token=token-resume",
      lobbyId: "lobby-test",
      token: "token-resume",
      sessionMode: "debug_simulation" as const,
      debugSessionId: "debug-1",
    });
    const client = new NetworkClient(
      {
        wsUrl: "ws://localhost:8080/ws?lobby=lobby-test&token=token-start",
        lobbyId: "lobby-test",
        token: "token-start",
        sessionMode: "debug_simulation",
        debugSessionId: "debug-1",
      },
      refreshMatch,
    ) as unknown as {
      socket?: WebSocket;
      openSocket: () => void;
      handleClose: (socket: WebSocket) => Promise<void>;
    };
    const socket = {} as WebSocket;
    const openSocket = vi.fn();
    client.socket = socket;
    client.openSocket = openSocket;

    await client.handleClose(socket);

    expect(refreshMatch).toHaveBeenCalledTimes(1);
    expect(openSocket).toHaveBeenCalledTimes(1);
  });

  it("does not reuse a stale debug start token when refresh fails", async () => {
    const refreshMatch = vi.fn().mockRejectedValue(new Error("unavailable"));
    const scheduleReconnect = vi.fn();
    const client = new NetworkClient(
      {
        wsUrl: "ws://localhost:8080/ws?lobby=lobby-test&token=token-start",
        lobbyId: "lobby-test",
        token: "token-start",
        sessionMode: "debug_simulation",
        debugSessionId: "debug-1",
      },
      refreshMatch,
    ) as unknown as {
      socket?: WebSocket;
      scheduleReconnect: () => void;
      handleClose: (socket: WebSocket) => Promise<void>;
    };
    const socket = {} as WebSocket;
    client.socket = socket;
    client.scheduleReconnect = scheduleReconnect;

    await client.handleClose(socket);

    expect(refreshMatch).toHaveBeenCalledTimes(1);
    expect(scheduleReconnect).not.toHaveBeenCalled();
  });
});
