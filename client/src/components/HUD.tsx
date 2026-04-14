import { useGameStore } from "../store/gameStore";

function formatTimer(ms: number) {
  const totalSeconds = Math.max(Math.floor(ms / 1000), 0);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

export default function HUD() {
  const self = useGameStore((state) => state.self);
  const matchTimerMs = useGameStore((state) => state.matchTimerMs);
  const health = self?.health ?? 0;
  const maxHealth = self?.maxHealth ?? 100;
  const healthPct = maxHealth > 0 ? Math.round((health / maxHealth) * 100) : 0;

  return (
    <div className="hud">
      <section className="hud-panel">
        <span>Score</span>
        <strong>{self?.score ?? 0}</strong>
      </section>
      <section className="hud-panel">
        <span>Mass</span>
        <strong>{(self?.mass ?? 0).toFixed(1)}</strong>
      </section>
      <section className="hud-panel">
        <span>Timer</span>
        <strong>{formatTimer(matchTimerMs)}</strong>
      </section>
      <section className="hud-panel">
        <span>Health</span>
        <strong>{`${Math.round(health)} / ${Math.round(maxHealth)} (${healthPct}%)`}</strong>
      </section>
      <section className="hud-panel">
        <span>Kills</span>
        <strong>{self?.kills ?? 0}</strong>
      </section>
      <section className="hud-panel">
        <span>Status</span>
        <strong>{self?.isAlive ? "Active" : "Respawning"}</strong>
      </section>
    </div>
  );
}
