import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import DeathScreen from "../components/DeathScreen";
import HUD from "../components/HUD";
import KillFeed from "../components/KillFeed";
import MiniMap from "../components/MiniMap";
import Scoreboard from "../components/Scoreboard";
import { startGameEngine } from "../engine";
import type { MatchJoinResponse, SnapshotMessage } from "../engine/types";
import { useGameStore } from "../store/gameStore";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8081";

type GameLocationState = {
  match?: MatchJoinResponse;
};

export default function GamePage() {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const location = useLocation();
  const navigate = useNavigate();
  const [snapshot, setSnapshot] = useState<SnapshotMessage>();
  const connectionStatus = useGameStore((state) => state.connectionStatus);
  const connectionError = useGameStore((state) => state.connectionError);
  const serverNotice = useGameStore((state) => state.serverNotice);
  const playerName = useGameStore((state) => state.playerName);
  const state = location.state as GameLocationState | null;

  useEffect(() => {
    const match = state?.match;
    const canvas = canvasRef.current;

    if (!match || !canvas) {
      return;
    }

    const engine = startGameEngine(canvas, match, {
      refreshMatch: async () => {
        const response = await fetch(`${API_BASE_URL}/api/matchmaking/join`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({ playerName, region: "local" }),
        });
        if (!response.ok) {
          throw new Error("failed to refresh route");
        }
        return (await response.json()) as MatchJoinResponse;
      },
    });
    const stopWatching = engine.onSnapshot((next: SnapshotMessage) => setSnapshot(next));

    return () => {
      stopWatching();
      engine.dispose();
    };
  }, [playerName, state]);

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
        <HUD />
        <KillFeed />
        <DeathScreen />
        <Scoreboard />
        <MiniMap snapshot={snapshot} />
        {serverNotice ? <div className="notice-banner">{serverNotice}</div> : null}
      </div>
      <section className="card">
        <div className="section-title">
          <div>
            <p className="eyebrow">Connection</p>
            <h3>{connectionStatus}</h3>
          </div>
          <Link to="/leaderboard">View global leaderboard</Link>
        </div>
        <p className="muted">
          Move with the mouse. Hold click or press Space to fire. The server owns the world state,
          so every collision and score update is authoritative.
        </p>
        {connectionError ? <p className="danger">{connectionError}</p> : null}
      </section>
    </section>
  );
}
