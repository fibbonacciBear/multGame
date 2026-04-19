import type { SnapshotMessage } from "../engine/types";
import { useGameStore } from "../store/gameStore";

function formatTimer(ms: number) {
  const totalSeconds = Math.max(Math.floor(ms / 1000), 0);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

type HUDProps = {
  snapshot?: SnapshotMessage;
};

export default function HUD({ snapshot }: HUDProps) {
  const self = useGameStore((state) => state.self);
  const matchTimerMs = useGameStore((state) => state.matchTimerMs);
  const sessionMode = useGameStore((state) => state.sessionMode);
  const cameraTargetId = useGameStore((state) => state.cameraTargetId);
  const phase = useGameStore((state) => state.phase);
  const matchKind = useGameStore((state) => state.matchKind);
  const target = snapshot?.players.find((player) => player.id === cameraTargetId);

  if (sessionMode !== "player") {
    return (
      <div className="hud">
        <section className="hud-panel">
          <span>{matchKind === "debug_bot_sim" ? "Debug" : "Spectating"}</span>
          <strong>{target?.name ?? (phase === "idle" ? "Idle world" : "Acquiring target")}</strong>
        </section>
        <section className="hud-panel">
          <span>Phase</span>
          <strong>{phase}</strong>
        </section>
        <section className="hud-panel">
          <span>Timer</span>
          <strong>{formatTimer(matchTimerMs)}</strong>
        </section>
      </div>
    );
  }

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
    </div>
  );
}
