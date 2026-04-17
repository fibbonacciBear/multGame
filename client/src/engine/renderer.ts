import type { Projectile, SnapshotMessage, WorldObject, WorldPlayer } from "./types";
import { getRailgunSprites, railgunCullRadius, type RailgunSprite } from "./projectileSprites";
import testPlayerSpriteUrl from "../assets/sprites/players/test-player.png";

const BASE_HEALTH_BAR_WIDTH = 44;
const BASELINE_MAX_HEALTH = 100;
const STARFIELD_SEED = "game-space-v1";
const AMBIENT_STAR_COLORS = ["#cde4ff", "#b3d4ff", "#adc8ff", "#ffe9c6", "#ffe0bc"];
const HERO_STAR_COLORS = ["#dbeeff", "#cce5ff", "#d8ecff", "#ffdeb0", "#ffedd3"];
const MIN_PROJECTILE_HEADING_SPEED = 0.001;
const PLAYER_SPRITE_SCALE = 2.4;
const SHOW_HITBOX_DEBUG = import.meta.env.VITE_SHOW_HITBOX_DEBUG === "true";

let playerSpriteImage: HTMLImageElement | undefined;
let playerSpriteReady = false;
let playerSpriteFailed = false;

type StarPoint = {
  x: number;
  y: number;
  radius: number;
  alpha: number;
  color: string;
};

type ScreenStarfield = {
  width: number;
  height: number;
  ambient: StarPoint[];
  hero: StarPoint[];
};

type ProjectileRenderData = {
  projectile: Projectile;
  angle: number;
  glow: RailgunSprite;
  core: RailgunSprite;
};

let screenStarfieldCache: ScreenStarfield | undefined;
const projectileHeadingCache = new Map<string, number>();
let lastProjectileMatchId = "";

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function isWithinViewport(
  x: number,
  y: number,
  radius: number,
  camX: number,
  camY: number,
  width: number,
  height: number,
  margin = 48,
) {
  return (
    x + radius >= camX - margin &&
    x - radius <= camX + width + margin &&
    y + radius >= camY - margin &&
    y - radius <= camY + height + margin
  );
}

function hashSeed(seed: string): number {
  let hash = 2166136261;
  for (let index = 0; index < seed.length; index += 1) {
    hash ^= seed.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

function createSeededRng(seed: string): () => number {
  let state = hashSeed(seed);
  return () => {
    state += 0x6d2b79f5;
    let mixed = Math.imul(state ^ (state >>> 15), 1 | state);
    mixed ^= mixed + Math.imul(mixed ^ (mixed >>> 7), 61 | mixed);
    return ((mixed ^ (mixed >>> 14)) >>> 0) / 4294967296;
  };
}

function randomInRange(random: () => number, min: number, max: number): number {
  return min + random() * (max - min);
}

function buildScreenStarLayer(options: {
  random: () => number;
  width: number;
  height: number;
  count: number;
  minRadius: number;
  maxRadius: number;
  minAlpha: number;
  maxAlpha: number;
  colors: string[];
}): StarPoint[] {
  const stars: StarPoint[] = [];
  for (let index = 0; index < options.count; index += 1) {
    stars.push({
      x: randomInRange(options.random, 0, options.width),
      y: randomInRange(options.random, 0, options.height),
      radius: randomInRange(options.random, options.minRadius, options.maxRadius),
      alpha: randomInRange(options.random, options.minAlpha, options.maxAlpha),
      color:
        options.colors[Math.floor(options.random() * options.colors.length)] ?? options.colors[0],
    });
  }
  return stars;
}

function getScreenStarfield(width: number, height: number): ScreenStarfield {
  const cached = screenStarfieldCache;
  if (cached && cached.width === width && cached.height === height) {
    return cached;
  }

  const random = createSeededRng(`${STARFIELD_SEED}:${width}x${height}`);
  const area = width * height;
  const ambientCount = Math.round(clamp(area / 9000, 120, 420));
  const heroCount = Math.round(clamp(area / 42000, 24, 110));

  const starfield: ScreenStarfield = {
    width,
    height,
    ambient: buildScreenStarLayer({
      random,
      width,
      height,
      count: ambientCount,
      minRadius: 0.5,
      maxRadius: 1.35,
      minAlpha: 0.4,
      maxAlpha: 0.9,
      colors: AMBIENT_STAR_COLORS,
    }),
    hero: buildScreenStarLayer({
      random,
      width,
      height,
      count: heroCount,
      minRadius: 1.2,
      maxRadius: 2.3,
      minAlpha: 0.62,
      maxAlpha: 0.98,
      colors: HERO_STAR_COLORS,
    }),
  };

  screenStarfieldCache = starfield;
  return starfield;
}

function wrap(value: number, size: number): number {
  if (size <= 0) {
    return value;
  }
  return ((value % size) + size) % size;
}

function drawStarLayer(
  ctx: CanvasRenderingContext2D,
  stars: StarPoint[],
  width: number,
  height: number,
  camX: number,
  camY: number,
  parallax: number,
) {
  for (const star of stars) {
    const x = wrap(star.x - camX * parallax, width);
    const y = wrap(star.y - camY * parallax, height);
    ctx.beginPath();
    ctx.arc(x, y, star.radius, 0, Math.PI * 2);
    ctx.globalAlpha = star.alpha;
    ctx.fillStyle = star.color;
    ctx.fill();
  }
}

function drawSpaceBackground(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  camX: number,
  camY: number,
) {
  ctx.fillStyle = "#01040b";
  ctx.fillRect(0, 0, width, height);

  const glow = ctx.createRadialGradient(
    width * 0.5,
    height * 0.3,
    0,
    width * 0.5,
    height * 0.3,
    Math.max(width, height) * 0.95,
  );
  glow.addColorStop(0, "rgba(34, 71, 128, 0.2)");
  glow.addColorStop(0.45, "rgba(16, 35, 75, 0.13)");
  glow.addColorStop(1, "rgba(2, 5, 13, 0)");
  ctx.fillStyle = glow;
  ctx.fillRect(0, 0, width, height);

  const starfield = getScreenStarfield(width, height);
  drawStarLayer(ctx, starfield.ambient, width, height, camX, camY, 0.006);
  drawStarLayer(ctx, starfield.hero, width, height, camX, camY, 0.014);
  ctx.globalAlpha = 1;
}

function objectMassColor(mass: number) {
  const t = clamp((mass - 0.35) / 0.8, 0, 1);
  const r = Math.round(72 + 160 * t);
  const g = Math.round(224 - 100 * t);
  return `rgb(${r},${g},50)`;
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

function ensurePlayerSpriteLoaded() {
  if (playerSpriteReady || playerSpriteFailed || playerSpriteImage) {
    return;
  }
  if (typeof Image === "undefined") {
    playerSpriteFailed = true;
    return;
  }
  const image = new Image();
  image.onload = () => {
    playerSpriteReady = true;
  };
  image.onerror = () => {
    playerSpriteFailed = true;
  };
  image.src = testPlayerSpriteUrl;
  playerSpriteImage = image;
}

function drawPlayerCircleFallback(ctx: CanvasRenderingContext2D, player: WorldPlayer, isSelf: boolean) {
  ctx.beginPath();
  ctx.arc(player.x, player.y, player.radius, 0, Math.PI * 2);
  ctx.fillStyle = isSelf ? "#68e1fd" : player.color;
  ctx.fill();
  ctx.lineWidth = isSelf ? 2.5 : 1.5;
  ctx.strokeStyle = isSelf ? "rgba(255,255,255,0.8)" : "rgba(255,255,255,0.28)";
  ctx.stroke();
}

function drawPlayerSprite(ctx: CanvasRenderingContext2D, player: WorldPlayer) {
  ensurePlayerSpriteLoaded();
  const image = playerSpriteImage;
  if (!playerSpriteReady || !image || !image.complete) {
    return false;
  }

  const targetLength = Math.max(player.radius * PLAYER_SPRITE_SCALE, 28);
  const sourceWidth = image.naturalWidth || image.width;
  const sourceHeight = image.naturalHeight || image.height;
  const sourceLength = Math.max(sourceWidth, sourceHeight, 1);
  const scale = targetLength / sourceLength;
  const targetWidth = sourceWidth * scale;
  const targetHeight = sourceHeight * scale;

  ctx.save();
  ctx.translate(player.x, player.y);
  ctx.rotate(player.angle);
  ctx.drawImage(
    image,
    -targetWidth / 2,
    -targetHeight / 2,
    targetWidth,
    targetHeight,
  );
  ctx.restore();
  return true;
}

function projectileAngle(projectile: Projectile) {
  const speed = Math.hypot(projectile.vx, projectile.vy);
  if (Number.isFinite(speed) && speed > MIN_PROJECTILE_HEADING_SPEED) {
    const angle = Math.atan2(projectile.vy, projectile.vx);
    projectileHeadingCache.set(projectile.id, angle);
    return angle;
  }

  return projectileHeadingCache.get(projectile.id) ?? 0;
}

function pruneProjectileAngleCache(projectiles: Projectile[], matchId: string) {
  if (lastProjectileMatchId !== matchId) {
    projectileHeadingCache.clear();
    lastProjectileMatchId = matchId;
    return;
  }

  if (projectileHeadingCache.size <= projectiles.length + 64) {
    return;
  }

  const activeIds = new Set(projectiles.map((projectile) => projectile.id));
  for (const id of projectileHeadingCache.keys()) {
    if (!activeIds.has(id)) {
      projectileHeadingCache.delete(id);
    }
  }
}

function drawProjectileSprite(ctx: CanvasRenderingContext2D, data: ProjectileRenderData, sprite: RailgunSprite) {
  ctx.save();
  ctx.translate(data.projectile.x, data.projectile.y);
  ctx.rotate(data.angle);
  ctx.drawImage(sprite.canvas, -sprite.originX, -sprite.originY, sprite.width, sprite.height);
  ctx.restore();
}

function drawProjectiles(
  ctx: CanvasRenderingContext2D,
  projectiles: Projectile[],
  matchId: string,
  camX: number,
  camY: number,
  width: number,
  height: number,
) {
  ctx.globalAlpha = 1;
  pruneProjectileAngleCache(projectiles, matchId);

  const renderables: ProjectileRenderData[] = [];
  const dpr = typeof window === "undefined" ? 1 : window.devicePixelRatio || 1;
  const cullRadius = railgunCullRadius();

  for (const projectile of projectiles) {
    if (projectile.type !== "railgun") {
      continue;
    }

    if (!isWithinViewport(projectile.x, projectile.y, cullRadius, camX, camY, width, height)) {
      continue;
    }

    const sprites = getRailgunSprites(projectile, dpr);
    renderables.push({
      projectile,
      angle: projectileAngle(projectile),
      glow: sprites.glow,
      core: sprites.core,
    });
  }

  if (renderables.length === 0) {
    ctx.globalAlpha = 1;
    return;
  }

  ctx.save();

  ctx.globalCompositeOperation = "lighter";
  for (const renderable of renderables) {
    drawProjectileSprite(ctx, renderable, renderable.glow);
  }

  ctx.globalCompositeOperation = "source-over";
  for (const renderable of renderables) {
    drawProjectileSprite(ctx, renderable, renderable.core);
  }

  ctx.restore();
  ctx.globalCompositeOperation = "source-over";
  ctx.globalAlpha = 1;
}

function drawPlayer(ctx: CanvasRenderingContext2D, player: WorldPlayer, isSelf: boolean) {
  ctx.globalAlpha = 1;
  drawHealthBar(ctx, player);

  if (!drawPlayerSprite(ctx, player)) {
    drawPlayerCircleFallback(ctx, player, isSelf);
  }

  if (SHOW_HITBOX_DEBUG) {
    // Debug boundary: authoritative server collision radius.
    ctx.beginPath();
    ctx.arc(player.x, player.y, player.radius, 0, Math.PI * 2);
    ctx.lineWidth = 1.5;
    ctx.strokeStyle = isSelf ? "rgba(104, 225, 253, 0.9)" : "rgba(255, 255, 255, 0.65)";
    ctx.stroke();
  }

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
  drawSpaceBackground(ctx, width, height, camX, camY);

  ctx.save();
  ctx.translate(-camX, -camY);

  ctx.strokeStyle = "rgba(255,80,80,0.35)";
  ctx.lineWidth = 2;
  ctx.strokeRect(0, 0, snapshot.world.width, snapshot.world.height);

  drawObjects(ctx, snapshot.objects, camX, camY, width, height);

  drawProjectiles(ctx, snapshot.projectiles, snapshot.matchId, camX, camY, width, height);

  snapshot.players
    .filter((player) => player.isAlive)
    .forEach((player) => drawPlayer(ctx, player, player.id === localPlayerId));

  ctx.restore();
  ctx.globalAlpha = 1;
}
