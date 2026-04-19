import { useEffect, useRef } from "react";
import type { SnapshotMessage } from "../engine/types";
import { useGameStore } from "../store/gameStore";

type MiniMapProps = {
  snapshot?: SnapshotMessage;
};

const MINIMAP_SIZE = 160;

export default function MiniMap({ snapshot }: MiniMapProps) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const localPlayerId = useGameStore((state) => state.localPlayerId);
  const cameraTargetId = useGameStore((state) => state.cameraTargetId);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) {
      return;
    }
    canvas.width = MINIMAP_SIZE;
    canvas.height = MINIMAP_SIZE;
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !snapshot) {
      return;
    }

    const ctx = canvas.getContext("2d");
    if (!ctx) {
      return;
    }

    ctx.clearRect(0, 0, MINIMAP_SIZE, MINIMAP_SIZE);
    ctx.fillStyle = "#05101b";
    ctx.fillRect(0, 0, MINIMAP_SIZE, MINIMAP_SIZE);
    ctx.strokeStyle = "rgba(255,255,255,0.2)";
    ctx.strokeRect(1, 1, MINIMAP_SIZE - 2, MINIMAP_SIZE - 2);

    for (const player of snapshot.players) {
      const x = (player.x / snapshot.world.width) * MINIMAP_SIZE;
      const y = (player.y / snapshot.world.height) * MINIMAP_SIZE;
      const isTarget = player.id === (cameraTargetId ?? localPlayerId);
      ctx.beginPath();
      ctx.arc(x, y, isTarget ? 4 : 2.5, 0, Math.PI * 2);
      ctx.fillStyle = isTarget ? "#68e1fd" : player.color;
      ctx.fill();
    }
  }, [cameraTargetId, localPlayerId, snapshot]);

  return (
    <aside className="minimap-card">
      <canvas ref={canvasRef} />
    </aside>
  );
}
