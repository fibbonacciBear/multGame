import { useGameStore } from "../store/gameStore";

export default function DeathScreen() {
  const self = useGameStore((state) => state.self);

  if (!self || self.isAlive) {
    return null;
  }

  return (
    <section className="death-overlay">
      <p className="eyebrow">Eliminated</p>
      <h3>{self.killedBy ? `Killed by ${self.killedBy}` : "You were destroyed"}</h3>
      <p className="muted">
        {self.deathReason ?? "The server will respawn you automatically."}
      </p>
      <p>
        Respawning in <strong>{Math.ceil(self.respawnInMs / 1000)}</strong>s
      </p>
    </section>
  );
}
