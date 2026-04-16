import { create } from "zustand";

export type KillFeedEntry = {
  id: string;
  killerName: string;
  victimName: string;
  atMs: number;
};

export type ScoreboardEntry = {
  playerId: string;
  playerName: string;
  kills: number;
  finalMass: number;
  massBonus: number;
  totalScore: number;
  isBot: boolean;
};

export type LeaderboardEntry = {
  playerName: string;
  kills: number;
  massBonus: number;
  totalScore: number;
};

export type SelfState = {
  playerId: string;
  playerName: string;
  score: number;
  mass: number;
  health: number;
  maxHealth: number;
  kills: number;
  isAlive: boolean;
  respawnInMs: number;
  deathReason?: string;
  killedBy?: string;
};

type GameStore = {
  playerName: string;
  localPlayerId?: string;
  connectionStatus: "idle" | "connecting" | "connected" | "disconnected" | "error";
  connectionError?: string;
  matchTimerMs: number;
  killFeed: KillFeedEntry[];
  self?: SelfState;
  matchOver: boolean;
  intermissionRemainingMs: number;
  scoreboard: ScoreboardEntry[];
  leaderboardPreview: LeaderboardEntry[];
  serverNotice?: string;
  setPlayerName: (playerName: string) => void;
  setLocalPlayerId: (playerId: string) => void;
  setConnectionStatus: (status: GameStore["connectionStatus"], error?: string) => void;
  setServerNotice: (serverNotice?: string) => void;
  setSnapshotState: (payload: {
    matchTimerMs: number;
    killFeed: KillFeedEntry[];
    self?: SelfState;
    matchOver: boolean;
    intermissionRemainingMs: number;
    scoreboard: ScoreboardEntry[];
    serverNotice?: string;
  }) => void;
  setLeaderboardPreview: (entries: LeaderboardEntry[]) => void;
  resetMatchUi: () => void;
};

const initialUiState = {
  localPlayerId: undefined,
  connectionStatus: "idle" as const,
  connectionError: undefined,
  matchTimerMs: 0,
  killFeed: [],
  self: undefined,
  matchOver: false,
  intermissionRemainingMs: 0,
  scoreboard: [],
  serverNotice: undefined,
};

export const useGameStore = create<GameStore>((set) => ({
  playerName: "",
  leaderboardPreview: [],
  ...initialUiState,
  setPlayerName: (playerName) => set({ playerName }),
  setLocalPlayerId: (playerId) => set({ localPlayerId: playerId }),
  setConnectionStatus: (connectionStatus, connectionError) =>
    set({ connectionStatus, connectionError }),
  setServerNotice: (serverNotice) => set({ serverNotice }),
  setSnapshotState: ({
    matchTimerMs,
    killFeed,
    self,
    matchOver,
    intermissionRemainingMs,
    scoreboard,
    serverNotice,
  }) =>
    set({
      matchTimerMs,
      killFeed,
      self,
      matchOver,
      intermissionRemainingMs,
      scoreboard,
      serverNotice,
    }),
  setLeaderboardPreview: (leaderboardPreview) => set({ leaderboardPreview }),
  resetMatchUi: () => set(initialUiState),
}));
