import { useGameStore } from "../store/gameStore";
import { InputController } from "./input";
import { NetworkClient } from "./network";
import { renderWorld } from "./renderer";
import type { MatchJoinResponse, SnapshotMessage } from "./types";

const INPUT_SEND_INTERVAL_MS = 1000 / 60;

export type GameEngineHandle = {
  onSnapshot: (listener: (snapshot: SnapshotMessage) => void) => () => void;
  dispose: () => void;
};

export function startGameEngine(
  canvas: HTMLCanvasElement,
  match: MatchJoinResponse,
  options?: {
    refreshMatch?: () => Promise<MatchJoinResponse>;
  },
): GameEngineHandle {
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    throw new Error("2D canvas is not available");
  }

  const resize = () => {
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    canvas.width = Math.floor(rect.width * dpr);
    canvas.height = Math.floor(rect.height * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  };

  resize();
  window.addEventListener("resize", resize);

  const input = match.sessionMode === "player" ? new InputController(canvas) : undefined;
  const network = new NetworkClient(match, options?.refreshMatch);
  network.connect();

  let disposed = false;
  let animationFrame = 0;
  let lastInputSentAt = 0;

  const tick = (timestamp: number) => {
    if (disposed) {
      return;
    }

    if (input && timestamp - lastInputSentAt >= INPUT_SEND_INTERVAL_MS) {
      const inputPayload = JSON.stringify(input.getState());
      network.sendInput(inputPayload);
      lastInputSentAt = timestamp;
    }

    const snapshot = network.getInterpolatedSnapshot();
    if (snapshot) {
      renderWorld(
        ctx,
        snapshot,
        useGameStore.getState().localPlayerId,
        network.getLatestSnapshot(),
        useGameStore.getState().cameraTargetId,
      );
    }

    animationFrame = window.requestAnimationFrame(tick);
  };

  animationFrame = window.requestAnimationFrame(tick);

  return {
    onSnapshot(listener) {
      return network.onSnapshot(listener);
    },
    dispose() {
      disposed = true;
      window.cancelAnimationFrame(animationFrame);
      window.removeEventListener("resize", resize);
      network.dispose();
      input?.dispose();
      useGameStore.getState().resetMatchUi();
    },
  };
}
