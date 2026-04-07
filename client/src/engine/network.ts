import { useGameStore } from "../store/gameStore";
import type { MatchJoinResponse, ServerMessage, SnapshotMessage } from "./types";

type SnapshotListener = (snapshot: SnapshotMessage) => void;
const MAX_RECONNECT_ATTEMPTS = 10;

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

export class NetworkClient {
  private readonly match: MatchJoinResponse;
  private readonly tokenExpiryMs?: number;
  private readonly snapshotListeners = new Set<SnapshotListener>();
  private socket?: WebSocket;
  private snapshotBuffer: SnapshotMessage[] = [];
  private disposed = false;
  private reconnectTimer?: number;
  private reconnectAttempts = 0;

  constructor(match: MatchJoinResponse) {
    this.match = match;
    this.tokenExpiryMs = getTokenExpiryMs(match.token);
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

    const url = new URL(this.match.wsUrl);
    url.searchParams.set("token", this.match.token);
    url.searchParams.set("lobby", this.match.lobbyId);

    const socket = new WebSocket(url);
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

      const message = JSON.parse(event.data) as ServerMessage;

      if (message.type === "welcome") {
        useGameStore.getState().setLocalPlayerId(message.playerId);
        return;
      }

      if (message.type === "server_notice") {
        useGameStore.getState().setServerNotice(message.message);
        return;
      }

      if (message.type === "snapshot") {
        this.snapshotBuffer = [...this.snapshotBuffer.slice(-1), message];
        useGameStore.getState().setSnapshotState({
          matchTimerMs: message.timeRemainingMs,
          killFeed: message.killFeed,
          self: message.you,
          matchOver: message.matchOver,
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

      this.socket = undefined;
      useGameStore
        .getState()
        .setConnectionStatus("connecting", "Connection lost, retrying...");
      this.scheduleReconnect();
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

    const [previous, current] = this.snapshotBuffer;
    const duration = Math.max(current.serverTime - previous.serverTime, 1);
    const t = Math.min((Date.now() - current.serverTime) / duration + 1, 1);

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
