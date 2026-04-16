import { useGameStore } from "../store/gameStore";
import type { MatchJoinResponse, ServerMessage, SnapshotMessage } from "./types";

type SnapshotListener = (snapshot: SnapshotMessage) => void;
const MAX_RECONNECT_ATTEMPTS = 10;
const MAX_SNAPSHOT_BUFFER_SIZE = 20;

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

export class NetworkClient {
  private tokenExpiryMs?: number;
  private match: MatchJoinResponse;
  private readonly snapshotListeners = new Set<SnapshotListener>();
  private readonly refreshMatch?: () => Promise<MatchJoinResponse>;
  private socket?: WebSocket;
  private snapshotBuffer: SnapshotMessage[] = [];
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
        useGameStore.getState().setLocalPlayerId(message.playerId);
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

        this.snapshotBuffer = [...this.snapshotBuffer, message].slice(-MAX_SNAPSHOT_BUFFER_SIZE);
        useGameStore.getState().setSnapshotState({
          matchTimerMs: message.timeRemainingMs,
          killFeed: message.killFeed,
          self: message.you,
          matchOver: message.matchOver,
          intermissionRemainingMs: message.intermissionRemainingMs ?? 0,
          scoreboard: message.scoreboard,
          serverNotice: message.serverNotice,
        });
        for (const listener of this.snapshotListeners) {
          listener(message);
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
    if (this.shouldRematch()) {
      const refreshed = await this.tryRefreshMatch();
      if (refreshed) {
        return;
      }
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
    return this.refreshMatch !== undefined && this.match.wsUrl.includes("/ws/");
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
    useGameStore.getState().setConnectionStatus("error", message);
  }

  onSnapshot(listener: SnapshotListener) {
    this.snapshotListeners.add(listener);
    return () => this.snapshotListeners.delete(listener);
  }

  getInterpolatedSnapshot() {
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

    return {
      ...current,
      players: current.players.map((player) => {
        const prior = previous.players.find((candidate) => candidate.id === player.id);
        if (!prior) {
          return player;
        }

        return {
          ...player,
          x: prior.x + (player.x - prior.x) * t,
          y: prior.y + (player.y - prior.y) * t,
        };
      }),
      objects: current.objects,
      projectiles: current.projectiles.map((projectile) => {
        const prior = previous.projectiles.find((candidate) => candidate.id === projectile.id);
        if (!prior) {
          return projectile;
        }

        return {
          ...projectile,
          x: prior.x + (projectile.x - prior.x) * t,
          y: prior.y + (projectile.y - prior.y) * t,
        };
      }),
    };
  }

  sendInput(payload: string) {
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
