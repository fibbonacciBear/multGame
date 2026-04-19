import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import DeathScreen from "../components/DeathScreen";
import HUD from "../components/HUD";
import KillFeed from "../components/KillFeed";
import LiveLeaderboard from "../components/LiveLeaderboard";
import MiniMap from "../components/MiniMap";
import Scoreboard from "../components/Scoreboard";
import { startGameEngine } from "../engine";
import type { MatchJoinResponse, SnapshotMessage } from "../engine/types";
import { useGameStore } from "../store/gameStore";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "";
const UI_SNAPSHOT_INTERVAL_MS = 150;

type GameLocationState = {
  match?: MatchJoinResponse;
  refresh?:
    | {
        sessionMode: "player";
        region: string;
        playerName: string;
      }
    | {
        sessionMode: "spectator";
        region: string;
        secret: string;
        lobbyId?: string;
        viewerId?: string;
      }
    | {
        sessionMode: "debug_simulation";
        region: string;
        secret: string;
        botCount: number;
        seed?: number;
        lobbyId?: string;
        viewerId?: string;
        debugSessionId?: string;
      };
};

async function requestMatchAssignment(
  path: string,
  payload: Record<string, unknown>,
): Promise<MatchJoinResponse> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  if (!response.ok) {
    const message = (await response.text()).trim();
    throw new Error(message || "failed to refresh route");
  }

  return (await response.json()) as MatchJoinResponse;
}

function getSpectatorTargets(snapshot: SnapshotMessage, sessionMode: MatchJoinResponse["sessionMode"]) {
  return [...snapshot.players].sort((left, right) => {
    if (sessionMode === "debug_simulation" && left.isBot !== right.isBot) {
      return left.isBot ? -1 : 1;
    }
    if (left.isAlive !== right.isAlive) {
      return left.isAlive ? -1 : 1;
    }
    if (left.name !== right.name) {
      return left.name.localeCompare(right.name);
    }
    return left.id.localeCompare(right.id);
  });
}

export default function GamePage() {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const latestUiSnapshotRef = useRef<SnapshotMessage>();
  const uiSnapshotTimerRef = useRef<number | undefined>(undefined);
  const hasPublishedInitialSnapshotRef = useRef(false);
  const location = useLocation();
  const navigate = useNavigate();
  const [snapshot, setSnapshot] = useState<SnapshotMessage>();
  const connectionStatus = useGameStore((state) => state.connectionStatus);
  const serverNotice = useGameStore((state) => state.serverNotice);
  const playerName = useGameStore((state) => state.playerName);
  const sessionMode = useGameStore((state) => state.sessionMode);
  const cameraTargetId = useGameStore((state) => state.cameraTargetId);
  const setCameraTargetId = useGameStore((state) => state.setCameraTargetId);
  const state = location.state as GameLocationState | null;

  useEffect(() => {
    const match = state?.match;
    const refresh = state?.refresh;
    const canvas = canvasRef.current;

    if (!match || !canvas) {
      return;
    }

    const engine = startGameEngine(canvas, match, {
      refreshMatch: async () => {
        if (!refresh) {
          throw new Error("missing refresh route");
        }

        if (refresh.sessionMode === "player") {
          return requestMatchAssignment("/api/matchmaking/join", {
            playerName: refresh.playerName || playerName,
            region: refresh.region,
          });
        }

        if (refresh.sessionMode === "spectator") {
          return requestMatchAssignment("/api/matchmaking/spectate", {
            region: refresh.region,
            secret: refresh.secret,
            lobbyId: match.lobbyId,
            viewerId: useGameStore.getState().viewerId ?? refresh.viewerId,
          });
        }

        return requestMatchAssignment("/api/matchmaking/debug-simulate", {
          region: refresh.region,
          secret: refresh.secret,
          botCount: refresh.botCount,
          seed: refresh.seed,
          lobbyId: match.lobbyId,
          viewerId: useGameStore.getState().viewerId ?? refresh.viewerId,
          debugSessionId:
            useGameStore.getState().debugSessionId ?? match.debugSessionId ?? refresh.debugSessionId,
        });
      },
    });
    hasPublishedInitialSnapshotRef.current = false;
    const publishSnapshot = () => {
      uiSnapshotTimerRef.current = undefined;
      if (latestUiSnapshotRef.current) {
        setSnapshot(latestUiSnapshotRef.current);
      }
    };
    const stopWatching = engine.onSnapshot((next: SnapshotMessage) => {
      latestUiSnapshotRef.current = next;
      if (!hasPublishedInitialSnapshotRef.current) {
        hasPublishedInitialSnapshotRef.current = true;
        setSnapshot(next);
        return;
      }
      if (uiSnapshotTimerRef.current !== undefined) {
        return;
      }
      uiSnapshotTimerRef.current = window.setTimeout(publishSnapshot, UI_SNAPSHOT_INTERVAL_MS);
    });

    return () => {
      if (uiSnapshotTimerRef.current !== undefined) {
        window.clearTimeout(uiSnapshotTimerRef.current);
        uiSnapshotTimerRef.current = undefined;
      }
      stopWatching();
      engine.dispose();
    };
  }, [playerName, state]);

  useEffect(() => {
    if (!snapshot) {
      return;
    }

    if (sessionMode === "player") {
      const localPlayerId = useGameStore.getState().localPlayerId;
      if (cameraTargetId !== localPlayerId) {
        setCameraTargetId(localPlayerId);
      }
      return;
    }

    const targets = getSpectatorTargets(snapshot, sessionMode);
    if (targets.length === 0) {
      if (cameraTargetId !== undefined) {
        setCameraTargetId(undefined);
      }
      return;
    }

    if (!cameraTargetId || !targets.some((player) => player.id === cameraTargetId)) {
      setCameraTargetId(targets[0]?.id);
    }
  }, [cameraTargetId, sessionMode, setCameraTargetId, snapshot]);

  useEffect(() => {
    if (sessionMode === "player") {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "[" && event.key !== "]" && event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
        return;
      }
      if (!latestUiSnapshotRef.current) {
        return;
      }

      const targets = getSpectatorTargets(latestUiSnapshotRef.current, sessionMode);
      if (targets.length === 0) {
        return;
      }

      const currentIndex = Math.max(
        targets.findIndex((player) => player.id === useGameStore.getState().cameraTargetId),
        0,
      );
      const delta = event.key === "[" || event.key === "ArrowLeft" ? -1 : 1;
      const next = targets[(currentIndex + delta + targets.length) % targets.length];
      if (next) {
        setCameraTargetId(next.id);
        event.preventDefault();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [sessionMode, setCameraTargetId]);

  useEffect(() => {
    if (!state?.match && connectionStatus === "idle") {
      navigate("/");
    }
  }, [connectionStatus, navigate, state]);

  if (!state?.match) {
    return (
      <section className="card">
        <h2>No active match</h2>
        <p className="muted">
          Matchmaking data was not available for this route. Return to the <Link to="/">main
          menu</Link> and join again.
        </p>
      </section>
    );
  }

  return (
    <section className="game-shell">
      <div className="game-stage">
        <canvas ref={canvasRef} />
        <HUD snapshot={snapshot} />
        <KillFeed />
        <LiveLeaderboard snapshot={snapshot} />
        <DeathScreen />
        <Scoreboard />
        <MiniMap snapshot={snapshot} />
        {serverNotice ? <div className="notice-banner">{serverNotice}</div> : null}
      </div>
    </section>
  );
}
