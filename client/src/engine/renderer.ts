import type { SnapshotMessage, WorldObject, WorldPlayer } from "./types";

const GRID_SPACING = 100;
const BASE_HEALTH_BAR_WIDTH = 44;
const BASELINE_MAX_HEALTH = 100;

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function objectMassColor(mass: number) {
  const t = clamp((mass - 0.35) / 0.8, 0, 1);
  const r = Math.round(72 + 160 * t);
  const g = Math.round(224 - 100 * t);
  return `rgb(${r},${g},50)`;
}

function drawGrid(
  ctx: CanvasRenderingContext2D,
  camX: number,
  camY: number,
  width: number,
  height: number,
) {
  ctx.strokeStyle = "rgba(255,255,255,0.06)";
  ctx.lineWidth = 1;

  const startCol = Math.floor(camX / GRID_SPACING);
  const endCol = Math.ceil((camX + width) / GRID_SPACING);
  const startRow = Math.floor(camY / GRID_SPACING);
  const endRow = Math.ceil((camY + height) / GRID_SPACING);

  ctx.beginPath();
  for (let col = startCol; col <= endCol; col += 1) {
    const x = col * GRID_SPACING;
    ctx.moveTo(x, camY);
    ctx.lineTo(x, camY + height);
  }

  for (let row = startRow; row <= endRow; row += 1) {
    const y = row * GRID_SPACING;
    ctx.moveTo(camX, y);
    ctx.lineTo(camX + width, y);
  }
  ctx.stroke();
}

function drawObjects(ctx: CanvasRenderingContext2D, objects: WorldObject[], camX: number, camY: number, width: number, height: number) {
  for (const object of objects) {
    if (
      object.x + object.radius < camX ||
      object.x - object.radius > camX + width ||
      object.y + object.radius < camY ||
      object.y - object.radius > camY + height
    ) {
      continue;
    }

    ctx.beginPath();
    ctx.arc(object.x, object.y, object.radius, 0, Math.PI * 2);
    ctx.fillStyle = objectMassColor(object.mass);
    ctx.fill();
  }
}

function drawHealthBar(ctx: CanvasRenderingContext2D, player: WorldPlayer) {
  const ratio = player.maxHealth > 0 ? clamp(player.health / player.maxHealth, 0, 1) : 0;
  const scaledWidth = BASE_HEALTH_BAR_WIDTH * (player.maxHealth / BASELINE_MAX_HEALTH);
  const width = clamp(scaledWidth, 32, 84);
  const height = 7;
  const x = player.x - width / 2;
  const y = player.y + player.radius + 10;

  ctx.fillStyle = "rgba(5,9,20,0.9)";
  ctx.fillRect(x, y, width, height);
  ctx.fillStyle = ratio > 0.35 ? "rgba(113, 255, 169, 0.92)" : "rgba(255, 139, 102, 0.92)";
  ctx.fillRect(x, y, width * ratio, height);
}

function drawPlayer(ctx: CanvasRenderingContext2D, player: WorldPlayer, isSelf: boolean) {
  drawHealthBar(ctx, player);

  ctx.beginPath();
  ctx.arc(player.x, player.y, player.radius, 0, Math.PI * 2);
  ctx.fillStyle = isSelf ? "#68e1fd" : player.color;
  ctx.fill();
  ctx.lineWidth = isSelf ? 2.5 : 1.5;
  ctx.strokeStyle = isSelf ? "rgba(255,255,255,0.8)" : "rgba(255,255,255,0.28)";
  ctx.stroke();

  const facingLength = player.radius + 14;
  ctx.beginPath();
  ctx.moveTo(player.x, player.y);
  ctx.lineTo(
    player.x + Math.cos(player.angle) * facingLength,
    player.y + Math.sin(player.angle) * facingLength,
  );
  ctx.lineWidth = 2;
  ctx.strokeStyle = "rgba(255,255,255,0.18)";
  ctx.stroke();

  ctx.fillStyle = "rgba(239,245,255,0.92)";
  ctx.font = "12px Space Grotesk, sans-serif";
  ctx.textAlign = "center";
  ctx.fillText(player.name, player.x, player.y - player.radius - 12);
}

export function renderWorld(
  ctx: CanvasRenderingContext2D,
  snapshot: SnapshotMessage,
  localPlayerId?: string,
) {
  const width = ctx.canvas.clientWidth || ctx.canvas.width;
  const height = ctx.canvas.clientHeight || ctx.canvas.height;
  const selfPlayer =
    snapshot.players.find((player) => player.id === localPlayerId) ?? snapshot.players[0];

  if (!selfPlayer) {
    ctx.clearRect(0, 0, width, height);
    return;
  }

  const camX = selfPlayer.x - width / 2;
  const camY = selfPlayer.y - height / 2;

  ctx.clearRect(0, 0, width, height);
  ctx.fillStyle = "#050914";
  ctx.fillRect(0, 0, width, height);

  ctx.save();
  ctx.translate(-camX, -camY);

  drawGrid(ctx, camX, camY, width, height);

  ctx.strokeStyle = "rgba(255,80,80,0.35)";
  ctx.lineWidth = 2;
  ctx.strokeRect(0, 0, snapshot.world.width, snapshot.world.height);

  drawObjects(ctx, snapshot.objects, camX, camY, width, height);

  for (const projectile of snapshot.projectiles) {
    ctx.beginPath();
    ctx.arc(projectile.x, projectile.y, projectile.radius, 0, Math.PI * 2);
    ctx.fillStyle = projectile.color;
    ctx.fill();
  }

  snapshot.players
    .filter((player) => player.isAlive)
    .forEach((player) => drawPlayer(ctx, player, player.id === localPlayerId));

  ctx.restore();
}
