import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useGameStore, type LeaderboardEntry } from "../store/gameStore";
import type { MatchJoinResponse } from "../engine/types";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "";
const DEFAULT_PLAYER_NAME = "Pilot";
const DEFAULT_MATCHMAKING_CONFIG = {
  tickRate: 60,
  snapshotRate: 20,
  maxPlayers: 10,
};

export default function MainMenu() {
  const navigate = useNavigate();
  const [playerName, setPlayerName] = useState(() => localStorage.getItem("multgame.playerName") ?? "");
  const [region, setRegion] = useState("local");
  const [isJoining, setIsJoining] = useState(false);
  const [error, setError] = useState<string>();
  const [matchmakingConfig, setMatchmakingConfig] = useState(DEFAULT_MATCHMAKING_CONFIG);
  const leaderboardPreview = useGameStore((state) => state.leaderboardPreview);
  const setLeaderboardPreview = useGameStore((state) => state.setLeaderboardPreview);
  const setStoredPlayerName = useGameStore((state) => state.setPlayerName);

  useEffect(() => {
    async function loadLeaderboardPreview() {
      try {
        const response = await fetch(`${API_BASE_URL}/api/leaderboard?limit=5`);
        if (!response.ok) {
          throw new Error("Leaderboard request failed");
        }
        const entries = (await response.json()) as LeaderboardEntry[];
        setLeaderboardPreview(entries);
      } catch {
        setLeaderboardPreview([]);
      }
    }

    async function loadMatchmakingConfig() {
      try {
        const response = await fetch(`${API_BASE_URL}/api/matchmaking/config`);
        if (!response.ok) {
          throw new Error("Matchmaking config request failed");
        }
        const config = (await response.json()) as {
          tickRate: number;
          snapshotRate: number;
          maxPlayers: number;
        };
        setMatchmakingConfig({
          tickRate: config.tickRate,
          snapshotRate: config.snapshotRate,
          maxPlayers: config.maxPlayers,
        });
      } catch {
        setMatchmakingConfig(DEFAULT_MATCHMAKING_CONFIG);
      }
    }

    void loadLeaderboardPreview();
    void loadMatchmakingConfig();
  }, [setLeaderboardPreview]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsJoining(true);
    setError(undefined);

    try {
      const trimmedName = playerName.trim().slice(0, 18);
      const submittedName = trimmedName || DEFAULT_PLAYER_NAME;

      const response = await fetch(`${API_BASE_URL}/api/matchmaking/join`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ playerName: submittedName, region }),
      });

      if (!response.ok) {
        throw new Error("Join request failed.");
      }

      const match = (await response.json()) as MatchJoinResponse;
      localStorage.setItem("multgame.playerName", submittedName);
      setStoredPlayerName(submittedName);
      navigate("/game", { state: { match } });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to join a match.");
    } finally {
      setIsJoining(false);
    }
  }

  return (
    <div className="page-grid">
      <section className="card hero-card">
        <div className="hero-content">
          <p className="eyebrow">Phase 1 / Local vertical slice</p>
          <h2>Mouse-driven inertia, authoritative combat, five-minute rounds.</h2>
          <p>
            The client only renders and submits input. Movement, collisions, kills, respawns,
            bots, and scoring stay on the server.
          </p>
          <div className="stat-row">
            <div className="stat-pill">
              <strong>{matchmakingConfig.tickRate} Hz</strong>
              <span className="muted">authoritative simulation</span>
            </div>
            <div className="stat-pill">
              <strong>{matchmakingConfig.snapshotRate} Hz</strong>
              <span className="muted">snapshot broadcast cadence</span>
            </div>
            <div className="stat-pill">
              <strong>{matchmakingConfig.maxPlayers}</strong>
              <span className="muted">max players per lobby with bot fill</span>
            </div>
          </div>
        </div>
      </section>

      <div className="stack">
        <section className="card">
          <div className="section-title">
            <div>
              <p className="eyebrow">Matchmaking</p>
              <h3>Join the local arena</h3>
            </div>
          </div>
          <form
            className="form-stack"
            onSubmit={(event) => {
              void handleSubmit(event);
            }}
          >
            <label className="stack">
              <span>Name</span>
              <input
                maxLength={18}
                placeholder="Pilot callsign"
                value={playerName}
                onChange={(event) => setPlayerName(event.target.value)}
              />
            </label>

            <label className="stack">
              <span>Region</span>
              <select value={region} onChange={(event) => setRegion(event.target.value)}>
                <option value="local">Local</option>
              </select>
            </label>

            <div className="form-actions">
              <button type="submit" disabled={isJoining}>
                {isJoining ? "Joining..." : "Play"}
              </button>
              <span className="hint">Direct WS handoff comes from the API server.</span>
            </div>
            {error ? <p className="danger">{error}</p> : null}
          </form>
        </section>

        <section className="card">
          <div className="section-title">
            <div>
              <p className="eyebrow">Top Scores</p>
              <h3>Leaderboard preview</h3>
            </div>
          </div>
          <div className="preview-list">
            {leaderboardPreview.length === 0 ? (
              <p className="muted">No posted results yet.</p>
            ) : (
              leaderboardPreview.map((entry, index) => (
                <div className="preview-row" key={`${entry.playerName}-${index}`}>
                  <span>#{index + 1}</span>
                  <strong>{entry.playerName}</strong>
                  <strong>{entry.totalScore} pts</strong>
                </div>
              ))
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
