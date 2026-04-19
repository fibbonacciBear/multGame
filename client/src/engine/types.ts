import type { KillFeedEntry, ScoreboardEntry, SelfState } from "../store/gameStore";

export type SessionMode = "player" | "spectator" | "debug_simulation";
export type MatchKind = "normal" | "debug_bot_sim";
export type LobbyPhase = "idle" | "active" | "intermission";

export type MatchJoinResponse = {
  wsUrl: string;
  lobbyId: string;
  token: string;
  sessionMode: SessionMode;
  debugSessionId?: string;
};

export type WorldPlayer = {
  id: string;
  name: string;
  spriteId?: string;
  spriteVariant?: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
  mass: number;
  radius: number;
  angle: number;
  health: number;
  maxHealth: number;
  isAlive: boolean;
  respawnInMs: number;
  isBot: boolean;
  color: string;
};

export type WorldObject = {
  id: string;
  x: number;
  y: number;
  radius: number;
  mass: number;
};

export type Projectile = {
  id: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
  ownerId: string;
  type: "railgun" | (string & {});
  color: string;
};

export type SnapshotMessage = {
  type: "snapshot";
  serverTime: number;
  world: {
    width: number;
    height: number;
  };
  matchId: string;
  phase: LobbyPhase;
  matchKind: MatchKind;
  debugSessionId?: string;
  matchOver: boolean;
  timeRemainingMs: number;
  intermissionRemainingMs: number;
  players: WorldPlayer[];
  objects: WorldObject[];
  projectiles: Projectile[];
  killFeed: KillFeedEntry[];
  scoreboard: ScoreboardEntry[];
  you?: SelfState;
  serverNotice?: string;
};

export type WelcomeMessage = {
  type: "welcome";
  sessionMode: SessionMode;
  viewerId: string;
  playerId?: string;
  lobbyId: string;
  matchId: string;
  phase: LobbyPhase;
  matchKind: MatchKind;
  cameraTargetId?: string;
  debugSessionId?: string;
};

export type ServerMessage =
  | WelcomeMessage
  | SnapshotMessage
  | {
      type: "server_notice";
      message: string;
    };

export type ClientInputMessage = {
  type: "input";
  angle: number;
  strength: number;
  shoot: boolean;
};
