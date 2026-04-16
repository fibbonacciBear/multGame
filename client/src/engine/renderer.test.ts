import { describe, expect, it, vi } from "vitest";
import { renderWorld } from "./renderer";
import type { SnapshotMessage } from "./types";

function createContextStub() {
  const clearRect = vi.fn();
  const translate = vi.fn();
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
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    fillRect: vi.fn(),
    clearRect,
    save: vi.fn(),
    translate,
    strokeRect: vi.fn(),
    restore: vi.fn(),
    fillText: vi.fn(),
    createRadialGradient,
    globalAlpha: 1,
    lineWidth: 0,
    strokeStyle: "",
    fillStyle: "",
    font: "",
    textAlign: "left" as CanvasTextAlign,
  } as unknown as CanvasRenderingContext2D;

  return { ctx, clearRect, translate, createRadialGradient, addColorStop };
}

const snapshot: SnapshotMessage = {
  type: "snapshot",
  serverTime: Date.now(),
  world: {
    width: 4000,
    height: 4000,
  },
  matchId: "match-1",
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
});
