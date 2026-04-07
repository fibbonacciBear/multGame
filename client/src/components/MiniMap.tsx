import { useEffect, useRef } from "react";
import type { SnapshotMessage } from "../engine/types";
import { useGameStore } from "../store/gameStore";

type MiniMapProps = {
  snapshot?: SnapshotMessage;
};

export default function MiniMap({ snapshot }: MiniMapProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const localPlayerId = useGameStore((state) => state.localPlayerId);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !snapshot) {
      return;
    }

    const ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    const size = 160;
    canvas.width = size;
    canvas.height = size;

    ctx.clearRect(0, 0, size, size);
    ctx.fillStyle = "#05101b";
    ctx.fillRect(0, 0, size, size);
    ctx.strokeStyle = "rgba(255,255,255,0.2)";
    ctx.strokeRect(1, 1, size - 2, size - 2);

    for (const player of snapshot.players) {
      const x = (player.x / snapshot.world.width) * size;
      const y = (player.y / snapshot.world.height) * size;
      ctx.beginPath();
      ctx.arc(x, y, player.id === localPlayerId ? 4 : 2.5, 0, Math.PI * 2);
      ctx.fillStyle = player.id === localPlayerId ? "#68e1fd" : player.color;
      ctx.fill();
    }
  }, [localPlayerId, snapshot]);

  return (
    <aside className="minimap-card">
      <canvas ref={canvasRef} />
    </aside>
  );
}
