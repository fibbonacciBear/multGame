import type { Projectile } from "./types";

const RAILGUN_WAKE_LENGTH = 72;
const RAILGUN_OUTER_WAKE_WIDTH = 5;
const RAILGUN_CORE_WIDTH = 1;
const RAILGUN_TIP_HALO_RADIUS = 10;
const RAILGUN_FORWARD_MARGIN = 10;
const MAX_GLOW_CACHE_ENTRIES = 32;

export type RailgunSprite = {
  canvas: HTMLCanvasElement;
  originX: number;
  originY: number;
  width: number;
  height: number;
  cullRadius: number;
};

export type RailgunSprites = {
  core: RailgunSprite;
  glow: RailgunSprite;
};

const coreCache = new Map<string, RailgunSprite>();
const glowCache = new Map<string, RailgunSprite>();
let cachedDpr = 0;

function normalizeDpr(dpr: number) {
  if (!Number.isFinite(dpr) || dpr <= 0) {
    return 1;
  }
  return Math.round(dpr * 100) / 100;
}

function cacheKey(parts: Array<string | number>) {
  return parts.join("|");
}

function clearCachesForDpr(dpr: number) {
  const normalized = normalizeDpr(dpr);
  if (cachedDpr === normalized) {
    return;
  }
  cachedDpr = normalized;
  coreCache.clear();
  glowCache.clear();
}

function colorWithAlpha(color: string, alpha: number) {
  const clampedAlpha = Math.min(Math.max(alpha, 0), 1);
  const hex = color.trim();
  const shortHexMatch = /^#([0-9a-fA-F]{3})$/.exec(hex);
  if (shortHexMatch) {
    const [, value] = shortHexMatch;
    const r = parseInt(value[0] + value[0], 16);
    const g = parseInt(value[1] + value[1], 16);
    const b = parseInt(value[2] + value[2], 16);
    return `rgba(${r}, ${g}, ${b}, ${clampedAlpha})`;
  }

  const longHexMatch = /^#([0-9a-fA-F]{6})$/.exec(hex);
  if (longHexMatch) {
    const [, value] = longHexMatch;
    const r = parseInt(value.slice(0, 2), 16);
    const g = parseInt(value.slice(2, 4), 16);
    const b = parseInt(value.slice(4, 6), 16);
    return `rgba(${r}, ${g}, ${b}, ${clampedAlpha})`;
  }

  const rgbMatch = /^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)(?:\s*,\s*[\d.]+)?\s*\)$/i.exec(hex);
  if (rgbMatch) {
    const [, r, g, b] = rgbMatch;
    return `rgba(${r}, ${g}, ${b}, ${clampedAlpha})`;
  }

  return color;
}

function createSpriteCanvas(width: number, height: number, dpr: number) {
  const canvas = document.createElement("canvas");
  canvas.width = Math.ceil(width * dpr);
  canvas.height = Math.ceil(height * dpr);

  const ctx = canvas.getContext("2d");
  if (ctx) {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  return { canvas, ctx };
}

function spriteMetadata(canvas: HTMLCanvasElement, width: number, height: number): RailgunSprite {
  return {
    canvas,
    width,
    height,
    originX: RAILGUN_WAKE_LENGTH + RAILGUN_TIP_HALO_RADIUS,
    originY: height / 2,
    cullRadius: RAILGUN_WAKE_LENGTH + RAILGUN_TIP_HALO_RADIUS + RAILGUN_FORWARD_MARGIN,
  };
}

function buildCoreSprite(type: string, dpr: number): RailgunSprite {
  const width = RAILGUN_WAKE_LENGTH + RAILGUN_TIP_HALO_RADIUS + RAILGUN_FORWARD_MARGIN;
  const height = RAILGUN_TIP_HALO_RADIUS * 2 + 8;
  const { canvas, ctx } = createSpriteCanvas(width, height, dpr);
  const sprite = spriteMetadata(canvas, width, height);

  if (!ctx) {
    return sprite;
  }

  const y = sprite.originY;
  const tipX = sprite.originX;
  const tailX = tipX - RAILGUN_WAKE_LENGTH;
  const diamondBackX = tipX - 10;
  const diamondMidX = tipX - 5;

  const coreGradient = ctx.createLinearGradient(tailX, y, tipX, y);
  coreGradient.addColorStop(0, "rgba(120, 145, 255, 0.2)");
  coreGradient.addColorStop(0.6, "rgba(188, 234, 255, 0.92)");
  coreGradient.addColorStop(1, "rgba(255, 255, 255, 1)");

  ctx.lineCap = "round";
  ctx.strokeStyle = coreGradient;
  ctx.lineWidth = RAILGUN_CORE_WIDTH;
  ctx.beginPath();
  ctx.moveTo(tailX, y);
  ctx.lineTo(tipX, y);
  ctx.stroke();

  ctx.fillStyle = "rgba(235, 255, 255, 0.98)";
  ctx.beginPath();
  ctx.moveTo(tipX, y);
  ctx.lineTo(diamondMidX, y - 3);
  ctx.lineTo(diamondBackX, y);
  ctx.lineTo(diamondMidX, y + 3);
  ctx.closePath();
  ctx.fill();

  ctx.strokeStyle = type === "railgun" ? "rgba(150, 230, 255, 0.92)" : "rgba(255,255,255,0.7)";
  ctx.lineWidth = 0.8;
  ctx.stroke();

  return sprite;
}

function buildGlowSprite(color: string, dpr: number): RailgunSprite {
  const width = RAILGUN_WAKE_LENGTH + RAILGUN_TIP_HALO_RADIUS + RAILGUN_FORWARD_MARGIN;
  const height = RAILGUN_TIP_HALO_RADIUS * 2 + 8;
  const { canvas, ctx } = createSpriteCanvas(width, height, dpr);
  const sprite = spriteMetadata(canvas, width, height);

  if (!ctx) {
    return sprite;
  }

  const y = sprite.originY;
  const tipX = sprite.originX;
  const tailX = tipX - RAILGUN_WAKE_LENGTH;

  const wakeGradient = ctx.createLinearGradient(tailX, y, tipX, y);
  wakeGradient.addColorStop(0, "rgba(0, 0, 0, 0)");
  wakeGradient.addColorStop(0.28, colorWithAlpha(color, 0.15));
  wakeGradient.addColorStop(0.78, colorWithAlpha(color, 0.53));
  wakeGradient.addColorStop(1, "rgba(230, 255, 255, 0.98)");

  ctx.strokeStyle = wakeGradient;
  ctx.lineCap = "round";
  ctx.lineWidth = RAILGUN_OUTER_WAKE_WIDTH;
  ctx.beginPath();
  ctx.moveTo(tailX, y);
  ctx.lineTo(tipX, y);
  ctx.stroke();

  const halo = ctx.createRadialGradient(tipX, y, 0, tipX, y, RAILGUN_TIP_HALO_RADIUS);
  halo.addColorStop(0, "rgba(245, 255, 255, 0.95)");
  halo.addColorStop(0.38, colorWithAlpha(color, 0.67));
  halo.addColorStop(1, "rgba(0, 0, 0, 0)");

  ctx.fillStyle = halo;
  ctx.beginPath();
  ctx.arc(tipX, y, RAILGUN_TIP_HALO_RADIUS, 0, Math.PI * 2);
  ctx.fill();

  return sprite;
}

export function getRailgunSprites(projectile: Projectile, dpr: number): RailgunSprites {
  const normalizedDpr = normalizeDpr(dpr);
  clearCachesForDpr(normalizedDpr);

  const type = projectile.type || "railgun";
  const coreKey = cacheKey([normalizedDpr, type]);
  let core = coreCache.get(coreKey);
  if (!core) {
    core = buildCoreSprite(type, normalizedDpr);
    coreCache.set(coreKey, core);
  }

  const glowKey = cacheKey([normalizedDpr, type, projectile.color]);
  let glow = glowCache.get(glowKey);
  if (!glow) {
    if (glowCache.size >= MAX_GLOW_CACHE_ENTRIES) {
      const oldestKey = glowCache.keys().next().value;
      if (oldestKey !== undefined) {
        glowCache.delete(oldestKey);
      }
    }
    glow = buildGlowSprite(projectile.color, normalizedDpr);
    glowCache.set(glowKey, glow);
  }

  return { core, glow };
}

export function railgunCullRadius() {
  return RAILGUN_WAKE_LENGTH + RAILGUN_TIP_HALO_RADIUS + RAILGUN_FORWARD_MARGIN;
}
