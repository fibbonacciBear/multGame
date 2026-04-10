import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";
import { useGameStore } from "../store/gameStore";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
  useGameStore.setState({
    playerName: "",
    leaderboardPreview: [],
  });
  useGameStore.getState().resetMatchUi();
});
