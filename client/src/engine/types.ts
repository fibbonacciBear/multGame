import type { KillFeedEntry, ScoreboardEntry, SelfState } from "../store/gameStore";

export type MatchJoinResponse = {
  wsUrl: string;
  lobbyId: string;
  token: string;
};

export type WorldPlayer = {
  id: string;
  name: string;
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
  radius: number;
  ownerId: string;
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
  playerId: string;
  lobbyId: string;
  matchId: string;
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
