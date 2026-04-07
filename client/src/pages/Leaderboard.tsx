import { useEffect, useState } from "react";
import type { LeaderboardEntry } from "../store/gameStore";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8081";

export default function LeaderboardPage() {
  const [entries, setEntries] = useState<LeaderboardEntry[]>([]);
  const [error, setError] = useState<string>();

  useEffect(() => {
    async function loadLeaderboard() {
      try {
        const response = await fetch(`${API_BASE_URL}/api/leaderboard?limit=25`);
        if (!response.ok) {
          throw new Error("Failed to load leaderboard");
        }
        setEntries((await response.json()) as LeaderboardEntry[]);
      } catch (caught) {
        setError(caught instanceof Error ? caught.message : "Unable to load leaderboard.");
      }
    }

    void loadLeaderboard();
  }, []);

  return (
    <section className="card">
      <div className="section-title">
        <div>
          <p className="eyebrow">Persistent scores</p>
          <h2>Leaderboard</h2>
        </div>
      </div>
      <p className="muted">
        The API stores the best reported result for each player in Redis. Score equals kills plus
        floor(final mass / 50).
      </p>
      {error ? <p className="danger">{error}</p> : null}
      <div className="leaderboard-list">
        {entries.length === 0 ? <p className="muted">No scores recorded yet.</p> : null}
        {entries.map((entry, index) => (
          <div className="leaderboard-row" key={`${entry.playerName}-${index}`}>
            <span>#{index + 1}</span>
            <div>
              <strong>{entry.playerName}</strong>
              <p className="muted">
                {entry.kills} kills / {entry.massBonus} mass bonus
              </p>
            </div>
            <strong>{entry.totalScore} pts</strong>
          </div>
        ))}
      </div>
    </section>
  );
}
