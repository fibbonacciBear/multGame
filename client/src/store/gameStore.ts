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
  pickupFeedback?: {
    sequence: number;
    massGain: number;
    healthGain: number;
  };
};

export type GameSessionMode = "player" | "spectator" | "debug_simulation";
export type GameLobbyPhase = "idle" | "active" | "intermission";
export type GameMatchKind = "normal" | "debug_bot_sim";

type GameStore = {
  playerName: string;
  localPlayerId?: string;
  viewerId?: string;
  sessionMode: GameSessionMode;
  cameraTargetId?: string;
  phase: GameLobbyPhase;
  matchKind: GameMatchKind;
  debugSessionId?: string;
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
  setSessionState: (payload: {
    sessionMode: GameSessionMode;
    viewerId: string;
    localPlayerId?: string;
    cameraTargetId?: string;
    phase: GameLobbyPhase;
    matchKind: GameMatchKind;
    debugSessionId?: string;
  }) => void;
  setCameraTargetId: (cameraTargetId?: string) => void;
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
    phase: GameLobbyPhase;
    matchKind: GameMatchKind;
    debugSessionId?: string;
  }) => void;
  setLeaderboardPreview: (entries: LeaderboardEntry[]) => void;
  resetMatchUi: () => void;
};

const initialUiState = {
  localPlayerId: undefined,
  viewerId: undefined,
  sessionMode: "player" as const,
  cameraTargetId: undefined,
  phase: "idle" as const,
  matchKind: "normal" as const,
  debugSessionId: undefined,
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
  setSessionState: ({ sessionMode, viewerId, localPlayerId, cameraTargetId, phase, matchKind, debugSessionId }) =>
    set({
      sessionMode,
      viewerId,
      localPlayerId,
      cameraTargetId,
      phase,
      matchKind,
      debugSessionId,
    }),
  setCameraTargetId: (cameraTargetId) => set({ cameraTargetId }),
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
    phase,
    matchKind,
    debugSessionId,
  }) =>
    set({
      matchTimerMs,
      killFeed,
      self,
      matchOver,
      intermissionRemainingMs,
      scoreboard,
      serverNotice,
      phase,
      matchKind,
      debugSessionId,
    }),
  setLeaderboardPreview: (leaderboardPreview) => set({ leaderboardPreview }),
  resetMatchUi: () => set(initialUiState),
}));
