import { describe, expect, it, vi } from "vitest";
import { renderWorld } from "./renderer";
import type { SnapshotMessage } from "./types";

function createContextStub() {
  const clearRect = vi.fn();
  const translate = vi.fn();
  const fillText = vi.fn();
  const addColorStop = vi.fn();
  const createRadialGradient = vi.fn(() => ({ addColorStop })) as unknown as CanvasRenderingContext2D["createRadialGradient"];

  const ctx = {
    canvas: {
      clientWidth: 320,
      clientHeight: 180,
      width: 640,
      height: 360,
    },
    beginPath: vi.fn(),
    closePath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    fillRect: vi.fn(),
    clearRect,
    save: vi.fn(),
    translate,
    rotate: vi.fn(),
    drawImage: vi.fn(),
    strokeRect: vi.fn(),
    restore: vi.fn(),
    fillText,
    createRadialGradient,
    globalAlpha: 1,
    globalCompositeOperation: "source-over" as GlobalCompositeOperation,
    lineWidth: 0,
    strokeStyle: "",
    fillStyle: "",
    font: "",
    textAlign: "left" as CanvasTextAlign,
    textBaseline: "alphabetic" as CanvasTextBaseline,
  } as unknown as CanvasRenderingContext2D;

  return { ctx, clearRect, translate, fillText, createRadialGradient, addColorStop };
}

const snapshot: SnapshotMessage = {
  type: "snapshot",
  serverTime: Date.now(),
  world: {
    width: 4000,
    height: 4000,
  },
  matchId: "match-1",
  phase: "active",
  matchKind: "normal",
  matchOver: false,
  timeRemainingMs: 1000,
  intermissionRemainingMs: 0,
  players: [
    {
      id: "self",
      name: "Pilot",
      x: 1000,
      y: 900,
      vx: 0,
      vy: 0,
      mass: 40,
      radius: 18,
      angle: 0,
      health: 100,
      maxHealth: 100,
      isAlive: true,
      respawnInMs: 0,
      isBot: false,
      color: "#68e1fd",
    },
  ],
  objects: [],
  projectiles: [],
  killFeed: [],
  scoreboard: [],
};

describe("renderWorld", () => {
  it("uses logical canvas size for camera and clearRect calculations", () => {
    const { ctx, clearRect, translate } = createContextStub();

    renderWorld(ctx, snapshot, "self");

    expect(clearRect).toHaveBeenCalledWith(0, 0, 320, 180);
    expect(translate).toHaveBeenCalledWith(-840, -810);
  });

  it("renders pickup text for self collectible gains", () => {
    const { ctx, fillText } = createContextStub();
    const snapshotWithPickup: SnapshotMessage = {
      ...snapshot,
      serverTime: snapshot.serverTime + 16,
      you: {
        playerId: "self",
        playerName: "Pilot",
        score: 0,
        mass: 41.5,
        health: 100,
        maxHealth: 100,
        kills: 0,
        isAlive: true,
        respawnInMs: 0,
        pickupFeedback: {
          sequence: 1,
          massGain: 1.5,
          healthGain: 2.2,
        },
      },
    };

    renderWorld(ctx, snapshotWithPickup, "self", snapshotWithPickup);

    expect(fillText.mock.calls.some(([text]) => text === "+1.5 mass  +2.2 health")).toBe(true);
  });

  it("does not render pickup text from non-self player state alone", () => {
    const { ctx, fillText } = createContextStub();
    const snapshotWithOtherPlayerPickup: SnapshotMessage = {
      ...snapshot,
      matchId: "match-2",
      serverTime: snapshot.serverTime + 32,
      players: [
        ...snapshot.players,
        {
          id: "other",
          name: "Other",
          x: 1120,
          y: 920,
          vx: 0,
          vy: 0,
          mass: 42,
          radius: 18,
          angle: 0,
          health: 100,
          maxHealth: 100,
          isAlive: true,
          respawnInMs: 0,
          isBot: false,
          color: "#ff7d61",
        },
      ],
    };

    renderWorld(ctx, snapshotWithOtherPlayerPickup, "self", snapshotWithOtherPlayerPickup);

    expect(fillText.mock.calls.some(([text]) => `${text}`.includes("mass"))).toBe(false);
  });
});
