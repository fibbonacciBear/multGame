import { useMemo } from "react";
import type { SnapshotMessage } from "../engine/types";

type LiveLeaderboardProps = {
  snapshot?: SnapshotMessage;
};

export default function LiveLeaderboard({ snapshot }: LiveLeaderboardProps) {
  const topByMass = useMemo(
    () =>
      snapshot?.players
        .slice()
        .sort((a, b) => b.mass - a.mass)
        .slice(0, 3) ?? [],
    [snapshot],
  );

  return (
    <aside className="live-leaderboard">
      <strong>Leaderboard</strong>
      <ul>
        {topByMass.length === 0 ? <li>No players yet.</li> : null}
        {topByMass.map((player, index) => (
          <li key={player.id}>
            <span>#{index + 1}</span>
            <strong>{player.name}</strong>
            <span>{player.mass.toFixed(1)} mass</span>
          </li>
        ))}
      </ul>
    </aside>
  );
}
