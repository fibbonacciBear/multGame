import { useGameStore } from "../store/gameStore";
import type {
  MatchJoinResponse,
  Projectile,
  ServerMessage,
  SnapshotMessage,
  WorldPlayer,
} from "./types";

type SnapshotListener = (snapshot: SnapshotMessage) => void;
const MAX_RECONNECT_ATTEMPTS = 10;
const MAX_SNAPSHOT_BUFFER_SIZE = 20;
const SNAPSHOT_STALE_TIMEOUT_MS = 1500;
const DEFAULT_PROJECTILE_TYPE = "railgun";
const DEFAULT_PROJECTILE_COLOR = "#68e1fd";
const MIN_HEADING_SPEED = 0.001;

export type ProjectileHeading = {
  vx: number;
  vy: number;
};

type PartialProjectile = Partial<Projectile> & {
  id?: string;
  x?: number;
  y?: number;
};

function interpolationDelayMs() {
  const raw = import.meta.env.VITE_INTERPOLATION_DELAY_MS;
  if (!raw) {
    return 100;
  }
  const parsed = Number(raw);
  if (!Number.isFinite(parsed)) {
    return 100;
  }
  return Math.min(Math.max(Math.round(parsed), 0), 250);
}

const INTERPOLATION_DELAY_MS = interpolationDelayMs();

function getTokenExpiryMs(token: string): number | undefined {
  const [, payload] = token.split(".");
  if (!payload) {
    return undefined;
  }

  try {
    const normalized = payload.replace(/-/g, "+").replace(/_/g, "/");
    const json = atob(normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "="));
    const decoded = JSON.parse(json) as { exp?: number };
    return decoded.exp ? decoded.exp * 1000 : undefined;
  } catch {
    return undefined;
  }
}

function parseServerMessage(data: string): ServerMessage {
  return JSON.parse(data) as ServerMessage;
}

function finiteOr(value: unknown, fallback: number) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

export function normalizeSnapshotProjectiles(
  message: SnapshotMessage,
  previous?: SnapshotMessage,
  headingCache = new Map<string, ProjectileHeading>(),
): SnapshotMessage {
  const previousProjectiles = new Map(
    previous?.projectiles.map((projectile) => [projectile.id, projectile]),
  );
  const projectiles = message.projectiles.map((projectile) =>
    normalizeProjectile(projectile as PartialProjectile, previousProjectiles, headingCache),
  );
  pruneProjectileHeadingCache(headingCache, projectiles);

  return {
    ...message,
    projectiles,
  };
}

function pruneProjectileHeadingCache(
  headingCache: Map<string, ProjectileHeading>,
  projectiles: Projectile[],
) {
  if (headingCache.size <= projectiles.length + MAX_SNAPSHOT_BUFFER_SIZE) {
    return;
  }

  const activeIds = new Set(projectiles.map((projectile) => projectile.id));
  for (const id of headingCache.keys()) {
    if (!activeIds.has(id)) {
      headingCache.delete(id);
    }
  }
}

function normalizeProjectile(
  projectile: PartialProjectile,
  previousProjectiles: Map<string, Projectile>,
  headingCache: Map<string, ProjectileHeading>,
): Projectile {
  const id = projectile.id ?? "shot-unknown";
  const x = finiteOr(projectile.x, 0);
  const y = finiteOr(projectile.y, 0);
  const prior = previousProjectiles.get(id);

  let vx = finiteOr(projectile.vx, Number.NaN);
  let vy = finiteOr(projectile.vy, Number.NaN);
  const speed = Math.hypot(vx, vy);
  if (!Number.isFinite(speed) || speed <= MIN_HEADING_SPEED) {
    const dx = prior ? x - prior.x : 0;
    const dy = prior ? y - prior.y : 0;
    if (Math.hypot(dx, dy) > MIN_HEADING_SPEED) {
      vx = dx;
      vy = dy;
    } else {
      const cached = headingCache.get(id);
      vx = cached?.vx ?? 1;
      vy = cached?.vy ?? 0;
    }
  }

  const normalizedSpeed = Math.hypot(vx, vy);
  if (Number.isFinite(normalizedSpeed) && normalizedSpeed > MIN_HEADING_SPEED) {
    headingCache.set(id, { vx, vy });
  }

  return {
    id,
    x,
    y,
    vx,
    vy,
    radius: finiteOr(projectile.radius, 0),
    ownerId: typeof projectile.ownerId === "string" ? projectile.ownerId : "",
    type: typeof projectile.type === "string" && projectile.type !== "" ? projectile.type : DEFAULT_PROJECTILE_TYPE,
    color: typeof projectile.color === "string" && projectile.color !== "" ? projectile.color : DEFAULT_PROJECTILE_COLOR,
  };
}

export class NetworkClient {
  private tokenExpiryMs?: number;
  private match: MatchJoinResponse;
  private readonly snapshotListeners = new Set<SnapshotListener>();
  private readonly refreshMatch?: () => Promise<MatchJoinResponse>;
  private socket?: WebSocket;
  private snapshotBuffer: SnapshotMessage[] = [];
  private projectileHeadingCache = new Map<string, ProjectileHeading>();
  private readonly previousPlayerLookup = new Map<string, WorldPlayer>();
  private readonly previousProjectileLookup = new Map<string, Projectile>();
  private readonly interpolatedPlayers = new Map<string, WorldPlayer>();
  private readonly interpolatedProjectiles = new Map<string, Projectile>();
  private readonly interpolatedPlayerOutput: WorldPlayer[] = [];
  private readonly interpolatedProjectileOutput: Projectile[] = [];
  private interpolatedSnapshotScratch?: SnapshotMessage;
  private lastSnapshotReceivedAtMs = 0;
  private staleSnapshotRecoveryTriggered = false;
  private disposed = false;
  private reconnectTimer?: number;
  private reconnectAttempts = 0;

  constructor(match: MatchJoinResponse, refreshMatch?: () => Promise<MatchJoinResponse>) {
    this.match = match;
    this.tokenExpiryMs = getTokenExpiryMs(match.token);
    this.refreshMatch = refreshMatch;
  }

  connect() {
    this.disposed = false;
    this.openSocket();
  }

  private openSocket() {
    if (this.disposed) {
      return;
    }

    if (this.hasReconnectExpired()) {
      this.failReconnect("Session expired, return to menu.");
      return;
    }

    useGameStore.getState().setConnectionStatus("connecting");

    const socket = new WebSocket(this.match.wsUrl);
    this.socket = socket;
    this.staleSnapshotRecoveryTriggered = false;
    this.lastSnapshotReceivedAtMs = 0;

    socket.addEventListener("open", () => {
      if (this.socket !== socket || this.disposed) {
        return;
      }

      this.reconnectAttempts = 0;
      useGameStore.getState().setConnectionStatus("connected");
    });

    socket.addEventListener("message", (event) => {
      if (this.socket !== socket || this.disposed) {
        return;
      }

      if (typeof event.data !== "string") {
        return;
      }

      const message = parseServerMessage(event.data);

      if (message.type === "welcome") {
        this.match = {
          ...this.match,
          sessionMode: message.sessionMode,
          debugSessionId: message.debugSessionId ?? this.match.debugSessionId,
        };
        useGameStore.getState().setSessionState({
          sessionMode: message.sessionMode,
          viewerId: message.viewerId,
          localPlayerId: message.playerId,
          cameraTargetId: message.cameraTargetId ?? message.playerId,
          phase: message.phase,
          matchKind: message.matchKind,
          debugSessionId: message.debugSessionId,
        });
        return;
      }

      if (message.type === "server_notice") {
        useGameStore.getState().setServerNotice(message.message);
        return;
      }

      if (message.type === "snapshot") {
        if (!message.killFeed) message.killFeed = [];
        if (!message.scoreboard) message.scoreboard = [];
        if (!message.players) message.players = [];
        if (!message.projectiles) message.projectiles = [];
        if (!message.objects) message.objects = [];
        this.lastSnapshotReceivedAtMs = Date.now();
        this.staleSnapshotRecoveryTriggered = false;

        const normalizedMessage = normalizeSnapshotProjectiles(
          message,
          this.snapshotBuffer[this.snapshotBuffer.length - 1],
          this.projectileHeadingCache,
        );
        this.snapshotBuffer = [...this.snapshotBuffer, normalizedMessage].slice(
          -MAX_SNAPSHOT_BUFFER_SIZE,
        );
        useGameStore.getState().setSnapshotState({
          matchTimerMs: normalizedMessage.timeRemainingMs,
          killFeed: normalizedMessage.killFeed,
          self: normalizedMessage.you,
          matchOver: normalizedMessage.matchOver,
          intermissionRemainingMs: normalizedMessage.intermissionRemainingMs ?? 0,
          scoreboard: normalizedMessage.scoreboard,
          serverNotice: normalizedMessage.serverNotice,
          phase: normalizedMessage.phase,
          matchKind: normalizedMessage.matchKind,
          debugSessionId: normalizedMessage.debugSessionId,
        });
        for (const listener of this.snapshotListeners) {
          listener(normalizedMessage);
        }
      }
    });

    socket.addEventListener("close", () => {
      if (this.socket !== socket || this.disposed) {
        return;
      }

      void this.handleClose(socket);
    });

    socket.addEventListener("error", () => {
      if (this.socket !== socket || this.disposed) {
        return;
      }

      useGameStore
        .getState()
        .setConnectionStatus("connecting", "Socket error, retrying...");
    });
  }

  private async handleClose(socket: WebSocket) {
    if (this.socket !== socket || this.disposed) {
      return;
    }

    this.socket = undefined;
    this.lastSnapshotReceivedAtMs = 0;
    this.staleSnapshotRecoveryTriggered = false;
    if (this.shouldRematch()) {
      const refreshed = await this.tryRefreshMatch();
      if (refreshed) {
        return;
      }
      return;
    }
    useGameStore.getState().setConnectionStatus("connecting", "Connection lost, retrying...");
    this.scheduleReconnect();
  }

  private scheduleReconnect() {
    if (this.disposed || this.reconnectTimer !== undefined) {
      return;
    }

    if (this.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS || this.hasReconnectExpired()) {
      this.failReconnect("Session expired, return to menu.");
      return;
    }

    const delayMs = Math.min(500 * 2 ** this.reconnectAttempts, 5000);
    this.reconnectAttempts += 1;

    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = undefined;
      this.openSocket();
    }, delayMs);
  }

  private shouldRematch() {
    if (!this.refreshMatch) {
      return false;
    }
    if (this.match.sessionMode !== "player") {
      return true;
    }
    return this.match.wsUrl.includes("/ws/");
  }

  private async tryRefreshMatch() {
    if (!this.refreshMatch) {
      return false;
    }

    useGameStore.getState().setConnectionStatus("connecting", "Refreshing match route...");
    try {
      this.match = await this.refreshMatch();
      this.tokenExpiryMs = getTokenExpiryMs(this.match.token);
      this.reconnectAttempts = 0;
      this.snapshotBuffer = [];
      this.openSocket();
      return true;
    } catch {
      this.failReconnect("Unable to refresh route, return to menu.");
      return false;
    }
  }

  private hasReconnectExpired() {
    return this.tokenExpiryMs !== undefined && Date.now() >= this.tokenExpiryMs;
  }

  private failReconnect(message: string) {
    if (this.reconnectTimer !== undefined) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }

    this.socket = undefined;
    this.lastSnapshotReceivedAtMs = 0;
    this.staleSnapshotRecoveryTriggered = false;
    useGameStore.getState().setConnectionStatus("error", message);
  }

  onSnapshot(listener: SnapshotListener) {
    this.snapshotListeners.add(listener);
    return () => this.snapshotListeners.delete(listener);
  }

  getLatestSnapshot() {
    // Exposes the latest authoritative snapshot for readonly consumers (e.g. effect syncing).
    // Callers must not mutate this object.
    if (this.snapshotBuffer.length === 0) {
      return undefined;
    }
    return this.snapshotBuffer[this.snapshotBuffer.length - 1];
  }

  private pruneInterpolatedEntityCaches(playerCount: number, projectileCount: number) {
    if (this.interpolatedPlayers.size > playerCount + MAX_SNAPSHOT_BUFFER_SIZE) {
      const activePlayerIds = new Set(this.interpolatedPlayerOutput.map((player) => player.id));
      for (const id of this.interpolatedPlayers.keys()) {
        if (!activePlayerIds.has(id)) {
          this.interpolatedPlayers.delete(id);
        }
      }
    }

    if (this.interpolatedProjectiles.size > projectileCount + MAX_SNAPSHOT_BUFFER_SIZE) {
      const activeProjectileIds = new Set(
        this.interpolatedProjectileOutput.map((projectile) => projectile.id),
      );
      for (const id of this.interpolatedProjectiles.keys()) {
        if (!activeProjectileIds.has(id)) {
          this.interpolatedProjectiles.delete(id);
        }
      }
    }
  }

  private recoverFromStaleSnapshotStream() {
    if (this.disposed || !this.socket) {
      return;
    }
    if (this.socket.readyState !== WebSocket.OPEN) {
      return;
    }
    if (this.lastSnapshotReceivedAtMs === 0) {
      return;
    }
    if (Date.now() - this.lastSnapshotReceivedAtMs <= SNAPSHOT_STALE_TIMEOUT_MS) {
      return;
    }
    if (this.staleSnapshotRecoveryTriggered) {
      return;
    }

    this.staleSnapshotRecoveryTriggered = true;
    this.snapshotBuffer = [];
    this.interpolatedSnapshotScratch = undefined;
    useGameStore
      .getState()
      .setConnectionStatus("connecting", "Snapshot stream stalled, reconnecting...");
    this.socket.close();
  }

  getInterpolatedSnapshot() {
    this.recoverFromStaleSnapshotStream();
    if (this.snapshotBuffer.length === 0) {
      return undefined;
    }

    if (this.snapshotBuffer.length === 1) {
      return this.snapshotBuffer[0];
    }

    const renderTime = Date.now() - INTERPOLATION_DELAY_MS;
    const latest = this.snapshotBuffer[this.snapshotBuffer.length - 1];
    if (renderTime >= latest.serverTime) {
      return latest;
    }

    let previous = this.snapshotBuffer[0];
    let current = latest;

    for (let index = this.snapshotBuffer.length - 1; index > 0; index--) {
      const candidateCurrent = this.snapshotBuffer[index];
      const candidatePrevious = this.snapshotBuffer[index - 1];
      if (candidatePrevious.serverTime <= renderTime && renderTime <= candidateCurrent.serverTime) {
        previous = candidatePrevious;
        current = candidateCurrent;
        break;
      }
      if (renderTime < candidatePrevious.serverTime) {
        previous = candidatePrevious;
        current = candidateCurrent;
      }
    }

    const duration = Math.max(current.serverTime - previous.serverTime, 1);
    const t = Math.min(Math.max((renderTime - previous.serverTime) / duration, 0), 1);

    this.previousPlayerLookup.clear();
    for (const player of previous.players) {
      this.previousPlayerLookup.set(player.id, player);
    }

    this.previousProjectileLookup.clear();
    for (const projectile of previous.projectiles) {
      this.previousProjectileLookup.set(projectile.id, projectile);
    }

    this.interpolatedPlayerOutput.length = current.players.length;
    for (let index = 0; index < current.players.length; index += 1) {
      const player = current.players[index];
      const prior = this.previousPlayerLookup.get(player.id);
      if (!prior || prior.isAlive !== player.isAlive) {
        this.interpolatedPlayerOutput[index] = player;
        continue;
      }

      let interpolated = this.interpolatedPlayers.get(player.id);
      if (!interpolated) {
        interpolated = { ...player };
        this.interpolatedPlayers.set(player.id, interpolated);
      }
      Object.assign(interpolated, player);
      interpolated.x = prior.x + (player.x - prior.x) * t;
      interpolated.y = prior.y + (player.y - prior.y) * t;
      this.interpolatedPlayerOutput[index] = interpolated;
    }

    this.interpolatedProjectileOutput.length = current.projectiles.length;
    for (let index = 0; index < current.projectiles.length; index += 1) {
      const projectile = current.projectiles[index];
      const prior = this.previousProjectileLookup.get(projectile.id);
      if (!prior) {
        this.interpolatedProjectileOutput[index] = projectile;
        continue;
      }

      let interpolated = this.interpolatedProjectiles.get(projectile.id);
      if (!interpolated) {
        interpolated = { ...projectile };
        this.interpolatedProjectiles.set(projectile.id, interpolated);
      }
      Object.assign(interpolated, projectile);
      interpolated.x = prior.x + (projectile.x - prior.x) * t;
      interpolated.y = prior.y + (projectile.y - prior.y) * t;
      this.interpolatedProjectileOutput[index] = interpolated;
    }

    this.pruneInterpolatedEntityCaches(current.players.length, current.projectiles.length);

    if (!this.interpolatedSnapshotScratch) {
      this.interpolatedSnapshotScratch = {
        ...current,
        players: this.interpolatedPlayerOutput,
        projectiles: this.interpolatedProjectileOutput,
      };
    }

    const scratch = this.interpolatedSnapshotScratch;
    scratch.type = current.type;
    scratch.serverTime = current.serverTime;
    scratch.world = current.world;
    scratch.matchId = current.matchId;
    scratch.phase = current.phase;
    scratch.matchKind = current.matchKind;
    scratch.debugSessionId = current.debugSessionId;
    scratch.matchOver = current.matchOver;
    scratch.timeRemainingMs = current.timeRemainingMs;
    scratch.intermissionRemainingMs = current.intermissionRemainingMs;
    scratch.players = this.interpolatedPlayerOutput;
    scratch.objects = current.objects;
    scratch.projectiles = this.interpolatedProjectileOutput;
    scratch.killFeed = current.killFeed;
    scratch.scoreboard = current.scoreboard;
    scratch.you = current.you;
    scratch.serverNotice = current.serverNotice;
    return scratch;
  }

  sendInput(payload: string) {
    if (this.match.sessionMode !== "player") {
      return;
    }
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(payload);
    }
  }

  dispose() {
    this.disposed = true;
    if (this.reconnectTimer !== undefined) {
      window.clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
    this.socket?.close();
    this.socket = undefined;
  }
}
