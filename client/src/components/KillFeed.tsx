import { useGameStore } from "../store/gameStore";

export default function KillFeed() {
  const entries = useGameStore((state) => state.killFeed);

  return (
    <aside className="kill-feed">
      <strong>Kill Feed</strong>
      <ul>
        {entries.length === 0 ? <li>No eliminations yet.</li> : null}
        {entries.map((entry) => (
          <li key={entry.id}>
            <strong>{entry.killerName}</strong> eliminated {entry.victimName}
          </li>
        ))}
      </ul>
    </aside>
  );
}
