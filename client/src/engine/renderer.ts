import type { Projectile, SnapshotMessage, WorldObject, WorldPlayer } from "./types";
import { getRailgunSprites, railgunCullRadius, type RailgunSprite } from "./projectileSprites";
import {
  DEFAULT_PLAYER_SPRITE_ID,
  getPlayerSpriteIdForVariant,
  getPlayerSpriteUrl,
  hasPlayerSpriteId,
} from "./playerSprites";

const BASE_HEALTH_BAR_WIDTH = 44;
const BASELINE_MAX_HEALTH = 100;
const STARFIELD_SEED = "game-space-v1";
const AMBIENT_STAR_COLORS = ["#cde4ff", "#b3d4ff", "#adc8ff", "#ffe9c6", "#ffe0bc"];
const HERO_STAR_COLORS = ["#dbeeff", "#cce5ff", "#d8ecff", "#ffdeb0", "#ffedd3"];
const MIN_PROJECTILE_HEADING_SPEED = 0.001;
const PLAYER_SPRITE_SCALE = 2.4;
const SHOW_HITBOX_DEBUG = import.meta.env.VITE_SHOW_HITBOX_DEBUG === "true";
const MASS_COLOR_BUCKETS = 64;
const DEBRIS_BASE_FILL = "#4a576d";
const DEBRIS_EDGE_STROKE = "rgba(232, 240, 255, 0.2)";
const DEBRIS_HIGHLIGHT_FILL = "rgba(239, 245, 255, 0.24)";
const SALVAGE_CORE_SPRITE_BASE_RADIUS = 12;
const SALVAGE_CORE_SPRITE_LOGICAL_SIZE = SALVAGE_CORE_SPRITE_BASE_RADIUS * 4;
const PERIMETER_OUTER_FIELD_DEPTH = 34;
const PERIMETER_INNER_RAIL_DEPTH = 24;
const PERIMETER_MODULE_SPACING = 128;
const PERIMETER_CORNER_ANCHOR_SIZE = 56;
const PERIMETER_VIEW_MARGIN = 96;

type PlayerSpriteImageState = {
  image?: HTMLImageElement;
  ready: boolean;
  failed: boolean;
};

const playerSpriteImageCache = new Map<string, PlayerSpriteImageState>();
const salvageCoreSpriteCache = new Map<string, SalvageCoreSprite>();
const horizontalPerimeterModuleSpriteCache = new Map<string, CachedPerimeterSprite>();
const verticalPerimeterModuleSpriteCache = new Map<string, CachedPerimeterSprite>();
const perimeterCornerSpriteCache = new Map<string, CachedPerimeterSprite>();

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

type CachedBackgroundLayers = {
  width: number;
  height: number;
  dpr: number;
  base?: HTMLCanvasElement;
  ambient?: HTMLCanvasElement;
  hero?: HTMLCanvasElement;
};

type SalvageCoreSprite = {
  canvas: HTMLCanvasElement;
  logicalSize: number;
};

type CachedPerimeterSprite = {
  canvas: HTMLCanvasElement;
  logicalWidth: number;
  logicalHeight: number;
};

type ProjectileRenderData = {
  projectile: Projectile;
  angle: number;
  glow: RailgunSprite;
  core: RailgunSprite;
};

type TrackedProjectile = {
  x: number;
  y: number;
  color: string;
  radius: number;
};

type ImpactEffect = {
  active: boolean;
  x: number;
  y: number;
  color: string;
  colorRgb: string;
  radius: number;
  createdAtMs: number;
  ttlMs: number;
  seed: number;
};

type ExplosionEffect = {
  active: boolean;
  x: number;
  y: number;
  color: string;
  colorRgb: string;
  radius: number;
  createdAtMs: number;
  ttlMs: number;
  seed: number;
};

type PickupTextEffect = {
  active: boolean;
  x: number;
  y: number;
  massGain: number;
  healthGain: number;
  createdAtMs: number;
  ttlMs: number;
};

type ExhaustNozzle = {
  x: number;
  y: number;
  scale: number;
};

const DEFAULT_EXHAUST_NOZZLES: ExhaustNozzle[] = [
  { x: -0.34, y: -0.12, scale: 1 },
  { x: -0.34, y: 0.12, scale: 1 },
];

const EXHAUST_NOZZLES_BY_SPRITE: Record<string, ExhaustNozzle[]> = {
  "test-player": [
    { x: -0.38, y: -0.16, scale: 1.04 },
    { x: -0.38, y: 0.16, scale: 1.04 },
  ],
  blue_no_bg: [{ x: -0.42, y: 0, scale: 1.12 }],
  purple_sprite: [{ x: -0.42, y: 0, scale: 1.12 }],
};

const EXHAUST_MIN_SPEED = 24;
const EXHAUST_MAX_SPEED = 620;
const EXHAUST_SIZE_BOOST = 1.85;
const MAX_IMPACT_EFFECTS = 120;
const MAX_EXPLOSION_EFFECTS = 20;
const MAX_PICKUP_TEXT_EFFECTS = 20;
const EXPLOSION_HEAVY_LOAD_THRESHOLD = 6;
const FIRE_RING_RGB = "255, 179, 110";
const SHOCK_RING_RGB = "231, 246, 255";
const EFFECT_TRACKING_RESET_GAP_MS = 1200;
const IMPACT_SYNC_VIEWPORT_MARGIN = 320;

let screenStarfieldCache: ScreenStarfield | undefined;
let backgroundLayerCache: CachedBackgroundLayers | undefined;
const projectileHeadingCache = new Map<string, number>();
const objectMassColorCache = new Map<number, string>();
const playerExhaustPhaseCache = new Map<string, number>();
const colorRgbCache = new Map<string, string>();
let lastProjectileMatchId = "";
let lastEffectsMatchId = "";
let lastEffectsSyncedServerTime = 0;
let lastPickupFeedbackSequence = 0;
const projectileRenderablesScratch: ProjectileRenderData[] = [];
const trackedProjectiles = new Map<string, TrackedProjectile>();
const trackedPlayerAlive = new Map<string, boolean>();
const currentProjectileIdsScratch = new Set<string>();
const currentPlayerIdsScratch = new Set<string>();
const activeImpactEffectIndices: number[] = [];
const activeExplosionEffectIndices: number[] = [];
const activePickupTextEffectIndices: number[] = [];
const impactActiveListIndexBySlot: number[] = Array.from(
  { length: MAX_IMPACT_EFFECTS },
  () => -1,
);
const explosionActiveListIndexBySlot: number[] = Array.from(
  { length: MAX_EXPLOSION_EFFECTS },
  () => -1,
);
const pickupTextActiveListIndexBySlot: number[] = Array.from(
  { length: MAX_PICKUP_TEXT_EFFECTS },
  () => -1,
);
const impactEffects: ImpactEffect[] = Array.from({ length: MAX_IMPACT_EFFECTS }, () => ({
  active: false,
  x: 0,
  y: 0,
  color: "#8fe3ff",
  colorRgb: "143, 227, 255",
  radius: 1,
  createdAtMs: 0,
  ttlMs: 0,
  seed: 0,
}));
const explosionEffects: ExplosionEffect[] = Array.from({ length: MAX_EXPLOSION_EFFECTS }, () => ({
  active: false,
  x: 0,
  y: 0,
  color: "#9fd6ff",
  colorRgb: "159, 214, 255",
  radius: 1,
  createdAtMs: 0,
  ttlMs: 0,
  seed: 0,
}));
const pickupTextEffects: PickupTextEffect[] = Array.from({ length: MAX_PICKUP_TEXT_EFFECTS }, () => ({
  active: false,
  x: 0,
  y: 0,
  massGain: 0,
  healthGain: 0,
  createdAtMs: 0,
  ttlMs: 0,
}));
let nextImpactEffectIndex = 0;
let nextExplosionEffectIndex = 0;
let nextPickupTextEffectIndex = 0;

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

function drawStarLayerToCanvas(ctx: CanvasRenderingContext2D, stars: StarPoint[]) {
  for (const star of stars) {
    ctx.beginPath();
    ctx.arc(star.x, star.y, star.radius, 0, Math.PI * 2);
    ctx.globalAlpha = star.alpha;
    ctx.fillStyle = star.color;
    ctx.fill();
  }
}

function createLayerCanvas() {
  if (typeof document === "undefined") {
    return undefined;
  }
  const canvas = document.createElement("canvas");
  return canvas;
}

function normalizeDpr(dpr: number) {
  if (!Number.isFinite(dpr) || dpr <= 0) {
    return 1;
  }
  return Math.round(dpr * 100) / 100;
}

function salvageCoreSpriteCacheKey(accentColor: string, dpr: number) {
  return `${accentColor}|${normalizeDpr(dpr)}`;
}

function perimeterSpriteCacheKey(kind: string, dpr: number) {
  return `${kind}|${normalizeDpr(dpr)}`;
}

function getOrCreateSalvageCoreSprite(accentColor: string, dpr: number) {
  const key = salvageCoreSpriteCacheKey(accentColor, dpr);
  const cached = salvageCoreSpriteCache.get(key);
  if (cached) {
    return cached;
  }

  const canvas = createLayerCanvas();
  if (!canvas) {
    return undefined;
  }

  const logicalSize = SALVAGE_CORE_SPRITE_LOGICAL_SIZE;
  const normalizedDpr = normalizeDpr(dpr);
  canvas.width = Math.ceil(logicalSize * normalizedDpr);
  canvas.height = Math.ceil(logicalSize * normalizedDpr);

  const spriteCtx = canvas.getContext("2d");
  if (!spriteCtx) {
    return undefined;
  }

  spriteCtx.setTransform(normalizedDpr, 0, 0, normalizedDpr, 0, 0);
  spriteCtx.translate(logicalSize / 2, logicalSize / 2);
  drawSalvageCore(
    spriteCtx,
    SALVAGE_CORE_SPRITE_BASE_RADIUS,
    rgbTripletForColor(accentColor),
    1,
  );

  const sprite = { canvas, logicalSize };
  salvageCoreSpriteCache.set(key, sprite);
  return sprite;
}

function getBackgroundLayers(width: number, height: number, dpr: number): CachedBackgroundLayers {
  const normalizedDpr = normalizeDpr(dpr);
  const cached = backgroundLayerCache;
  if (
    cached &&
    cached.width === width &&
    cached.height === height &&
    cached.dpr === normalizedDpr
  ) {
    return cached;
  }

  const starfield = getScreenStarfield(width, height);
  const layers: CachedBackgroundLayers = { width, height, dpr: normalizedDpr };

  const baseCanvas = createLayerCanvas();
  if (baseCanvas) {
    baseCanvas.width = Math.ceil(width * normalizedDpr);
    baseCanvas.height = Math.ceil(height * normalizedDpr);
    const baseCtx = baseCanvas.getContext("2d");
    if (baseCtx) {
      baseCtx.setTransform(normalizedDpr, 0, 0, normalizedDpr, 0, 0);
      baseCtx.fillStyle = "#01040b";
      baseCtx.fillRect(0, 0, width, height);
      const glow = baseCtx.createRadialGradient(
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
      baseCtx.fillStyle = glow;
      baseCtx.fillRect(0, 0, width, height);
      layers.base = baseCanvas;
    }
  }

  const ambientCanvas = createLayerCanvas();
  if (ambientCanvas) {
    ambientCanvas.width = Math.ceil(width * normalizedDpr);
    ambientCanvas.height = Math.ceil(height * normalizedDpr);
    const ambientCtx = ambientCanvas.getContext("2d");
    if (ambientCtx) {
      ambientCtx.setTransform(normalizedDpr, 0, 0, normalizedDpr, 0, 0);
      drawStarLayerToCanvas(ambientCtx, starfield.ambient);
      ambientCtx.globalAlpha = 1;
      layers.ambient = ambientCanvas;
    }
  }

  const heroCanvas = createLayerCanvas();
  if (heroCanvas) {
    heroCanvas.width = Math.ceil(width * normalizedDpr);
    heroCanvas.height = Math.ceil(height * normalizedDpr);
    const heroCtx = heroCanvas.getContext("2d");
    if (heroCtx) {
      heroCtx.setTransform(normalizedDpr, 0, 0, normalizedDpr, 0, 0);
      drawStarLayerToCanvas(heroCtx, starfield.hero);
      heroCtx.globalAlpha = 1;
      layers.hero = heroCanvas;
    }
  }

  backgroundLayerCache = layers;
  return layers;
}

function getOrCreateHorizontalPerimeterModuleSprite(dpr: number) {
  const key = perimeterSpriteCacheKey("horizontal-module", dpr);
  const cached = horizontalPerimeterModuleSpriteCache.get(key);
  if (cached) {
    return cached;
  }

  const canvas = createLayerCanvas();
  if (!canvas) {
    return undefined;
  }

  const logicalWidth = PERIMETER_MODULE_SPACING;
  const logicalHeight = PERIMETER_INNER_RAIL_DEPTH;
  const normalizedDpr = normalizeDpr(dpr);
  canvas.width = Math.ceil(logicalWidth * normalizedDpr);
  canvas.height = Math.ceil(logicalHeight * normalizedDpr);

  const ctx = canvas.getContext("2d");
  if (!ctx) {
    return undefined;
  }

  ctx.setTransform(normalizedDpr, 0, 0, normalizedDpr, 0, 0);
  const moduleCenter = logicalWidth / 2;
  const housingX = moduleCenter - 20;
  const towerX = moduleCenter - 7;

  ctx.fillStyle = "#253140";
  ctx.fillRect(housingX, 4, 40, 12);
  ctx.fillStyle = "#0f1621";
  ctx.fillRect(towerX, 0, 14, 18);
  ctx.fillStyle = "rgba(232, 240, 255, 0.12)";
  ctx.fillRect(housingX + 5, 6, 30, 1);
  ctx.fillStyle = "rgba(255, 179, 71, 0.72)";
  ctx.fillRect(moduleCenter - 2, 10, 4, 4);
  ctx.fillStyle = "rgba(91, 121, 164, 0.88)";
  ctx.fillRect(moduleCenter - 30, 9, 8, 6);
  ctx.fillRect(moduleCenter + 22, 9, 8, 6);

  const sprite = { canvas, logicalWidth, logicalHeight };
  horizontalPerimeterModuleSpriteCache.set(key, sprite);
  return sprite;
}

function getOrCreateVerticalPerimeterModuleSprite(dpr: number) {
  const key = perimeterSpriteCacheKey("vertical-module", dpr);
  const cached = verticalPerimeterModuleSpriteCache.get(key);
  if (cached) {
    return cached;
  }

  const canvas = createLayerCanvas();
  if (!canvas) {
    return undefined;
  }

  const logicalWidth = PERIMETER_INNER_RAIL_DEPTH;
  const logicalHeight = PERIMETER_MODULE_SPACING;
  const normalizedDpr = normalizeDpr(dpr);
  canvas.width = Math.ceil(logicalWidth * normalizedDpr);
  canvas.height = Math.ceil(logicalHeight * normalizedDpr);

  const ctx = canvas.getContext("2d");
  if (!ctx) {
    return undefined;
  }

  ctx.setTransform(normalizedDpr, 0, 0, normalizedDpr, 0, 0);
  const moduleCenter = logicalHeight / 2;
  const housingY = moduleCenter - 20;
  const towerY = moduleCenter - 7;

  ctx.fillStyle = "#253140";
  ctx.fillRect(4, housingY, 12, 40);
  ctx.fillStyle = "#0f1621";
  ctx.fillRect(0, towerY, 18, 14);
  ctx.fillStyle = "rgba(232, 240, 255, 0.12)";
  ctx.fillRect(6, housingY + 5, 1, 30);
  ctx.fillStyle = "rgba(255, 179, 71, 0.72)";
  ctx.fillRect(10, moduleCenter - 2, 4, 4);
  ctx.fillStyle = "rgba(91, 121, 164, 0.88)";
  ctx.fillRect(9, moduleCenter - 30, 6, 8);
  ctx.fillRect(9, moduleCenter + 22, 6, 8);

  const sprite = { canvas, logicalWidth, logicalHeight };
  verticalPerimeterModuleSpriteCache.set(key, sprite);
  return sprite;
}

function getOrCreatePerimeterCornerSprite(dpr: number) {
  const key = perimeterSpriteCacheKey("corner-anchor", dpr);
  const cached = perimeterCornerSpriteCache.get(key);
  if (cached) {
    return cached;
  }

  const canvas = createLayerCanvas();
  if (!canvas) {
    return undefined;
  }

  const logicalWidth = PERIMETER_CORNER_ANCHOR_SIZE;
  const logicalHeight = PERIMETER_CORNER_ANCHOR_SIZE;
  const normalizedDpr = normalizeDpr(dpr);
  canvas.width = Math.ceil(logicalWidth * normalizedDpr);
  canvas.height = Math.ceil(logicalHeight * normalizedDpr);

  const ctx = canvas.getContext("2d");
  if (!ctx) {
    return undefined;
  }

  ctx.setTransform(normalizedDpr, 0, 0, normalizedDpr, 0, 0);

  fillPolygon(
    ctx,
    [
      { x: 0, y: 14 },
      { x: 14, y: 0 },
      { x: logicalWidth, y: 0 },
      { x: logicalWidth, y: 18 },
      { x: 18, y: logicalHeight },
      { x: 0, y: logicalHeight },
    ],
    "#1f2935",
  );
  fillPolygon(
    ctx,
    [
      { x: 0, y: 8 },
      { x: 8, y: 0 },
      { x: logicalWidth - 10, y: 0 },
      { x: logicalWidth - 10, y: 9 },
      { x: 9, y: logicalHeight - 10 },
      { x: 0, y: logicalHeight - 10 },
    ],
    "rgba(255, 255, 255, 0.06)",
  );
  strokePolygon(
    ctx,
    [
      { x: 0, y: 14 },
      { x: 14, y: 0 },
      { x: logicalWidth, y: 0 },
      { x: logicalWidth, y: 18 },
      { x: 18, y: logicalHeight },
      { x: 0, y: logicalHeight },
    ],
    "rgba(232, 240, 255, 0.12)",
    1.2,
  );

  ctx.fillStyle = "rgba(127, 231, 255, 0.2)";
  ctx.beginPath();
  ctx.arc(24, 24, 16, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = "rgba(239, 255, 255, 0.82)";
  ctx.beginPath();
  ctx.arc(24, 24, 4.5, 0, Math.PI * 2);
  ctx.fill();

  ctx.strokeStyle = "rgba(104, 225, 253, 0.52)";
  ctx.lineWidth = 1.6;
  ctx.beginPath();
  ctx.moveTo(18, 38);
  ctx.lineTo(38, 18);
  ctx.stroke();

  const sprite = { canvas, logicalWidth, logicalHeight };
  perimeterCornerSpriteCache.set(key, sprite);
  return sprite;
}

function drawWrappedLayer(
  ctx: CanvasRenderingContext2D,
  layer: HTMLCanvasElement,
  width: number,
  height: number,
  offsetX: number,
  offsetY: number,
) {
  const startX = -wrap(offsetX, width);
  const startY = -wrap(offsetY, height);
  const scaleX = width > 0 ? layer.width / width : 1;
  const scaleY = height > 0 ? layer.height / height : 1;
  ctx.drawImage(layer, startX, startY, width, height);

  const seamWidth = Math.ceil(-startX);
  const seamHeight = Math.ceil(-startY);

  if (seamWidth > 0) {
    ctx.drawImage(
      layer,
      0,
      0,
      seamWidth * scaleX,
      height * scaleY,
      width - seamWidth,
      startY,
      seamWidth,
      height,
    );
  }

  if (seamHeight > 0) {
    ctx.drawImage(
      layer,
      0,
      0,
      width * scaleX,
      seamHeight * scaleY,
      startX,
      height - seamHeight,
      width,
      seamHeight,
    );
  }

  if (seamWidth > 0 && seamHeight > 0) {
    ctx.drawImage(
      layer,
      0,
      0,
      seamWidth * scaleX,
      seamHeight * scaleY,
      width - seamWidth,
      height - seamHeight,
      seamWidth,
      seamHeight,
    );
  }
}

function drawSpaceBackground(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  camX: number,
  camY: number,
) {
  const dpr = typeof window === "undefined" ? 1 : window.devicePixelRatio || 1;
  const layers = getBackgroundLayers(width, height, dpr);
  if (layers.base) {
    ctx.drawImage(layers.base, 0, 0, width, height);
  } else {
    ctx.fillStyle = "#01040b";
    ctx.fillRect(0, 0, width, height);
  }

  if (layers.ambient) {
    drawWrappedLayer(ctx, layers.ambient, width, height, camX * 0.006, camY * 0.006);
  }
  if (layers.hero) {
    drawWrappedLayer(ctx, layers.hero, width, height, camX * 0.014, camY * 0.014);
  }
  ctx.globalAlpha = 1;
}

function objectMassColor(mass: number) {
  const t = clamp(mass - 1, 0, 1);
  const bucket = Math.round(t * MASS_COLOR_BUCKETS);
  const cached = objectMassColorCache.get(bucket);
  if (cached) {
    return cached;
  }

  const normalized = bucket / MASS_COLOR_BUCKETS;
  const r = Math.round(112 + 143 * normalized);
  const g = Math.round(227 - 18 * normalized);
  const b = Math.round(255 - 147 * normalized);
  const color = `rgb(${r},${g},${b})`;
  objectMassColorCache.set(bucket, color);
  return color;
}

function rgbTripletForColor(color: string) {
  const normalized = color.trim().toLowerCase();
  const cached = colorRgbCache.get(normalized);
  if (cached) {
    return cached;
  }

  const shortHex = /^#([0-9a-f]{3})$/.exec(normalized);
  if (shortHex) {
    const value = shortHex[1];
    const rgb = `${parseInt(value[0] + value[0], 16)}, ${parseInt(value[1] + value[1], 16)}, ${parseInt(
      value[2] + value[2],
      16,
    )}`;
    colorRgbCache.set(normalized, rgb);
    return rgb;
  }

  const longHex = /^#([0-9a-f]{6})$/.exec(normalized);
  if (longHex) {
    const value = longHex[1];
    const rgb = `${parseInt(value.slice(0, 2), 16)}, ${parseInt(value.slice(2, 4), 16)}, ${parseInt(
      value.slice(4, 6),
      16,
    )}`;
    colorRgbCache.set(normalized, rgb);
    return rgb;
  }

  const rgbMatch = /^rgba?\(\s*([\d.]+)\s*,\s*([\d.]+)\s*,\s*([\d.]+)(?:\s*,\s*[\d.]+)?\s*\)$/i.exec(
    normalized,
  );
  if (rgbMatch) {
    const rgb = `${rgbMatch[1]}, ${rgbMatch[2]}, ${rgbMatch[3]}`;
    colorRgbCache.set(normalized, rgb);
    return rgb;
  }

  const fallback = "143, 227, 255";
  colorRgbCache.set(normalized, fallback);
  return fallback;
}

function rgbaFromTriplet(rgbTriplet: string, alpha: number) {
  return `rgba(${rgbTriplet}, ${clamp(alpha, 0, 1)})`;
}

function edgeOffsetStart(boundary: number, offset: number, thickness: number, direction: 1 | -1) {
  return direction > 0 ? boundary + offset : boundary - offset - thickness;
}

function alignedLoopStart(start: number, spacing: number) {
  return Math.floor(start / spacing) * spacing;
}

function tracePolygon(ctx: CanvasRenderingContext2D, points: Array<{ x: number; y: number }>) {
  if (points.length === 0) {
    return;
  }
  ctx.beginPath();
  ctx.moveTo(points[0].x, points[0].y);
  for (let index = 1; index < points.length; index += 1) {
    ctx.lineTo(points[index].x, points[index].y);
  }
  ctx.closePath();
}

function fillPolygon(ctx: CanvasRenderingContext2D, points: Array<{ x: number; y: number }>, fillStyle: string) {
  tracePolygon(ctx, points);
  ctx.fillStyle = fillStyle;
  ctx.fill();
}

function strokePolygon(
  ctx: CanvasRenderingContext2D,
  points: Array<{ x: number; y: number }>,
  strokeStyle: string,
  lineWidth: number,
) {
  tracePolygon(ctx, points);
  ctx.strokeStyle = strokeStyle;
  ctx.lineWidth = lineWidth;
  ctx.stroke();
}

function drawSalvageCore(ctx: CanvasRenderingContext2D, radius: number, accentRgb: string, pulse: number) {
  const shellPieces = [
    [
      { x: -radius * 0.98, y: -radius * 0.16 },
      { x: -radius * 0.7, y: -radius * 0.58 },
      { x: -radius * 0.22, y: -radius * 0.38 },
      { x: -radius * 0.4, y: radius * 0.06 },
      { x: -radius * 0.88, y: radius * 0.16 },
    ],
    [
      { x: radius * 0.28, y: -radius * 0.64 },
      { x: radius * 0.82, y: -radius * 0.38 },
      { x: radius * 0.92, y: radius * 0.08 },
      { x: radius * 0.54, y: radius * 0.34 },
      { x: radius * 0.18, y: -radius * 0.02 },
    ],
    [
      { x: -radius * 0.34, y: radius * 0.24 },
      { x: radius * 0.1, y: radius * 0.38 },
      { x: radius * 0.2, y: radius * 0.9 },
      { x: -radius * 0.22, y: radius * 0.98 },
      { x: -radius * 0.58, y: radius * 0.58 },
    ],
  ];
  const highlightPieces = [
    [
      { x: -radius * 0.84, y: -radius * 0.12 },
      { x: -radius * 0.64, y: -radius * 0.42 },
      { x: -radius * 0.34, y: -radius * 0.32 },
      { x: -radius * 0.48, y: -radius * 0.02 },
    ],
    [
      { x: radius * 0.36, y: -radius * 0.5 },
      { x: radius * 0.7, y: -radius * 0.34 },
      { x: radius * 0.72, y: -radius * 0.04 },
      { x: radius * 0.48, y: -radius * 0.12 },
    ],
    [
      { x: -radius * 0.22, y: radius * 0.36 },
      { x: radius * 0.02, y: radius * 0.44 },
      { x: radius * 0.08, y: radius * 0.74 },
      { x: -radius * 0.16, y: radius * 0.8 },
    ],
  ];

  const glowRadius = radius * 1.55 * pulse;
  const halo = ctx.createRadialGradient(0, 0, radius * 0.08, 0, 0, glowRadius);
  halo.addColorStop(0, `rgba(255, 255, 255, ${0.96 * pulse})`);
  halo.addColorStop(0.28, `rgba(${accentRgb}, ${0.78 * pulse})`);
  halo.addColorStop(0.66, `rgba(${accentRgb}, ${0.22 * pulse})`);
  halo.addColorStop(1, "rgba(0, 0, 0, 0)");
  ctx.fillStyle = halo;
  ctx.beginPath();
  ctx.arc(0, 0, glowRadius, 0, Math.PI * 2);
  ctx.fill();

  for (let index = 0; index < shellPieces.length; index += 1) {
    fillPolygon(ctx, shellPieces[index], DEBRIS_BASE_FILL);
    fillPolygon(ctx, highlightPieces[index], DEBRIS_HIGHLIGHT_FILL);
    strokePolygon(ctx, shellPieces[index], DEBRIS_EDGE_STROKE, Math.max(0.8, radius * 0.1));
  }

  ctx.lineCap = "round";
  ctx.strokeStyle = "rgba(236, 244, 255, 0.26)";
  ctx.lineWidth = Math.max(1, radius * 0.14);
  const ringRadius = radius * 0.74;
  ctx.beginPath();
  ctx.arc(0, 0, ringRadius, -Math.PI * 0.08, Math.PI * 0.62);
  ctx.stroke();
  ctx.beginPath();
  ctx.arc(0, 0, ringRadius, Math.PI * 0.92, Math.PI * 1.56);
  ctx.stroke();

  const coreRadius = radius * 0.54;
  const core = ctx.createRadialGradient(-radius * 0.08, -radius * 0.1, 0, 0, 0, coreRadius);
  core.addColorStop(0, "rgba(255, 255, 255, 0.98)");
  core.addColorStop(0.28, "rgba(245, 252, 255, 0.95)");
  core.addColorStop(0.62, `rgba(${accentRgb}, 0.92)`);
  core.addColorStop(1, `rgba(${accentRgb}, 0.18)`);
  ctx.fillStyle = core;
  ctx.beginPath();
  ctx.arc(0, 0, coreRadius, 0, Math.PI * 2);
  ctx.fill();

  ctx.strokeStyle = "rgba(255, 255, 255, 0.34)";
  ctx.lineWidth = Math.max(0.9, radius * 0.07);
  ctx.beginPath();
  ctx.arc(0, 0, coreRadius * 0.64, 0, Math.PI * 2);
  ctx.stroke();

  ctx.strokeStyle = "rgba(255, 255, 255, 0.28)";
  ctx.lineWidth = Math.max(0.7, radius * 0.05);
  ctx.beginPath();
  ctx.moveTo(-radius * 0.12, 0);
  ctx.lineTo(radius * 0.12, 0);
  ctx.moveTo(0, -radius * 0.12);
  ctx.lineTo(0, radius * 0.12);
  ctx.stroke();

  ctx.fillStyle = "rgba(255, 255, 255, 0.5)";
  ctx.beginPath();
  ctx.arc(-radius * 0.16, -radius * 0.16, radius * 0.13, 0, Math.PI * 2);
  ctx.fill();
}

function drawDebrisObject(ctx: CanvasRenderingContext2D, object: WorldObject, dpr: number) {
  const accentColor = objectMassColor(object.mass);
  const accentRgb = rgbTripletForColor(accentColor);
  const seed = hashSeed(`debris:${object.id}`);
  const radius = object.radius;
  const renderRadius = radius * 0.82;
  const rotation = ((seed >>> 3) % 360) * (Math.PI / 180);
  const pulsePhase = ((seed >>> 12) % 360) * (Math.PI / 180);
  const nowSeconds = (typeof performance === "undefined" ? Date.now() : performance.now()) / 1000;
  const pulse = 0.96 + (Math.sin(nowSeconds * 2.2 + pulsePhase) * 0.5 + 0.5) * 0.12;
  const sprite = getOrCreateSalvageCoreSprite(accentColor, dpr);

  ctx.save();
  ctx.translate(object.x, object.y);
  ctx.rotate(rotation);

  ctx.beginPath();
  ctx.arc(0, 0, renderRadius * 1.14, 0, Math.PI * 2);
  ctx.fillStyle = "rgba(1, 4, 10, 0.22)";
  ctx.fill();

  if (sprite) {
    const drawSize = sprite.logicalSize * (renderRadius / SALVAGE_CORE_SPRITE_BASE_RADIUS) * pulse;
    ctx.drawImage(sprite.canvas, -drawSize / 2, -drawSize / 2, drawSize, drawSize);
  } else {
    drawSalvageCore(ctx, renderRadius, accentRgb, pulse);
  }

  ctx.restore();
}

function drawObjects(ctx: CanvasRenderingContext2D, objects: WorldObject[], camX: number, camY: number, width: number, height: number) {
  const dpr = typeof window === "undefined" ? 1 : window.devicePixelRatio || 1;
  for (const object of objects) {
    if (
      object.x + object.radius < camX ||
      object.x - object.radius > camX + width ||
      object.y + object.radius < camY ||
      object.y - object.radius > camY + height
    ) {
      continue;
    }
    drawDebrisObject(ctx, object, dpr);
  }
}

function drawHorizontalPerimeterEdge(
  ctx: CanvasRenderingContext2D,
  boundaryY: number,
  insideDirection: 1 | -1,
  startX: number,
  endX: number,
  dpr: number,
  nowMs: number,
) {
  const length = endX - startX;
  if (length <= 0) {
    return;
  }

  const outsideDirection = insideDirection === 1 ? -1 : 1;
  const energyTravel = (nowMs * 0.05) % PERIMETER_MODULE_SPACING;
  const shimmer = Math.sin(nowMs * 0.0024 + boundaryY * 0.01) * 0.08;

  ctx.fillStyle = `rgba(14, 42, 76, ${0.18 + shimmer})`;
  ctx.fillRect(
    startX,
    edgeOffsetStart(boundaryY, 0, PERIMETER_OUTER_FIELD_DEPTH, outsideDirection),
    length,
    PERIMETER_OUTER_FIELD_DEPTH,
  );

  for (let band = 0; band < 4; band += 1) {
    const offset = 4 + band * 6 + Math.sin(nowMs * 0.004 + band * 1.3 + boundaryY * 0.015) * 1.5;
    ctx.fillStyle = `rgba(123, 224, 255, ${0.05 + band * 0.025})`;
    ctx.fillRect(startX, edgeOffsetStart(boundaryY, offset, 1.1, outsideDirection), length, 1.1);
  }

  ctx.fillStyle = "#161e29";
  ctx.fillRect(
    startX,
    edgeOffsetStart(boundaryY, 0, PERIMETER_INNER_RAIL_DEPTH, insideDirection),
    length,
    PERIMETER_INNER_RAIL_DEPTH,
  );
  ctx.fillStyle = "rgba(232, 240, 255, 0.08)";
  ctx.fillRect(startX, edgeOffsetStart(boundaryY, 6, 1, insideDirection), length, 1);
  ctx.fillStyle = "rgba(43, 58, 79, 0.95)";
  ctx.fillRect(startX, edgeOffsetStart(boundaryY, 13, 7, insideDirection), length, 7);
  ctx.fillStyle = "rgba(234, 243, 255, 0.16)";
  ctx.fillRect(startX, edgeOffsetStart(boundaryY, 20, 1, insideDirection), length, 1);

  ctx.fillStyle = "rgba(127, 231, 255, 0.72)";
  ctx.fillRect(startX, boundaryY - 1.2, length, 2.4);
  ctx.fillStyle = "rgba(239, 255, 255, 0.9)";
  ctx.fillRect(startX, boundaryY - 0.45, length, 0.9);

  const pulseWidth = 40;
  for (
    let pulseX = alignedLoopStart(startX - PERIMETER_MODULE_SPACING + energyTravel, PERIMETER_MODULE_SPACING);
    pulseX < endX + PERIMETER_MODULE_SPACING;
    pulseX += PERIMETER_MODULE_SPACING
  ) {
    ctx.fillStyle = "rgba(232, 252, 255, 0.82)";
    ctx.fillRect(pulseX, boundaryY - 1.8, pulseWidth, 3.6);
    ctx.fillStyle = "rgba(52, 191, 255, 0.32)";
    ctx.fillRect(pulseX - 8, boundaryY - 4, pulseWidth + 16, 8);
  }

  const moduleSprite = getOrCreateHorizontalPerimeterModuleSprite(dpr);
  for (
    let moduleStartX = alignedLoopStart(startX, PERIMETER_MODULE_SPACING) - PERIMETER_MODULE_SPACING / 2;
    moduleStartX < endX + PERIMETER_MODULE_SPACING;
    moduleStartX += PERIMETER_MODULE_SPACING
  ) {
    if (moduleSprite) {
      if (insideDirection === 1) {
        ctx.drawImage(
          moduleSprite.canvas,
          moduleStartX,
          boundaryY,
          moduleSprite.logicalWidth,
          moduleSprite.logicalHeight,
        );
      } else {
        ctx.save();
        ctx.translate(moduleStartX, boundaryY);
        ctx.scale(1, -1);
        ctx.drawImage(moduleSprite.canvas, 0, 0, moduleSprite.logicalWidth, moduleSprite.logicalHeight);
        ctx.restore();
      }
      continue;
    }

    const moduleCenter = moduleStartX + PERIMETER_MODULE_SPACING / 2;
    const housingX = moduleCenter - 20;
    const towerX = moduleCenter - 7;

    ctx.fillStyle = "#253140";
    ctx.fillRect(housingX, edgeOffsetStart(boundaryY, 4, 12, insideDirection), 40, 12);
    ctx.fillStyle = "#0f1621";
    ctx.fillRect(towerX, edgeOffsetStart(boundaryY, 0, 18, insideDirection), 14, 18);
    ctx.fillStyle = "rgba(232, 240, 255, 0.12)";
    ctx.fillRect(housingX + 5, edgeOffsetStart(boundaryY, 6, 1, insideDirection), 30, 1);
    ctx.fillStyle = "rgba(255, 179, 71, 0.72)";
    ctx.fillRect(moduleCenter - 2, edgeOffsetStart(boundaryY, 10, 4, insideDirection), 4, 4);
    ctx.fillStyle = "rgba(91, 121, 164, 0.88)";
    ctx.fillRect(moduleCenter - 30, edgeOffsetStart(boundaryY, 9, 6, insideDirection), 8, 6);
    ctx.fillRect(moduleCenter + 22, edgeOffsetStart(boundaryY, 9, 6, insideDirection), 8, 6);
  }
}

function drawVerticalPerimeterEdge(
  ctx: CanvasRenderingContext2D,
  boundaryX: number,
  insideDirection: 1 | -1,
  startY: number,
  endY: number,
  dpr: number,
  nowMs: number,
) {
  const length = endY - startY;
  if (length <= 0) {
    return;
  }

  const outsideDirection = insideDirection === 1 ? -1 : 1;
  const energyTravel = (nowMs * 0.05) % PERIMETER_MODULE_SPACING;
  const shimmer = Math.sin(nowMs * 0.0022 + boundaryX * 0.012) * 0.08;

  ctx.fillStyle = `rgba(14, 42, 76, ${0.18 + shimmer})`;
  ctx.fillRect(
    edgeOffsetStart(boundaryX, 0, PERIMETER_OUTER_FIELD_DEPTH, outsideDirection),
    startY,
    PERIMETER_OUTER_FIELD_DEPTH,
    length,
  );

  for (let band = 0; band < 4; band += 1) {
    const offset = 4 + band * 6 + Math.sin(nowMs * 0.004 + band * 1.3 + boundaryX * 0.015) * 1.5;
    ctx.fillStyle = `rgba(123, 224, 255, ${0.05 + band * 0.025})`;
    ctx.fillRect(edgeOffsetStart(boundaryX, offset, 1.1, outsideDirection), startY, 1.1, length);
  }

  ctx.fillStyle = "#161e29";
  ctx.fillRect(
    edgeOffsetStart(boundaryX, 0, PERIMETER_INNER_RAIL_DEPTH, insideDirection),
    startY,
    PERIMETER_INNER_RAIL_DEPTH,
    length,
  );
  ctx.fillStyle = "rgba(232, 240, 255, 0.08)";
  ctx.fillRect(edgeOffsetStart(boundaryX, 6, 1, insideDirection), startY, 1, length);
  ctx.fillStyle = "rgba(43, 58, 79, 0.95)";
  ctx.fillRect(edgeOffsetStart(boundaryX, 13, 7, insideDirection), startY, 7, length);
  ctx.fillStyle = "rgba(234, 243, 255, 0.16)";
  ctx.fillRect(edgeOffsetStart(boundaryX, 20, 1, insideDirection), startY, 1, length);

  ctx.fillStyle = "rgba(127, 231, 255, 0.72)";
  ctx.fillRect(boundaryX - 1.2, startY, 2.4, length);
  ctx.fillStyle = "rgba(239, 255, 255, 0.9)";
  ctx.fillRect(boundaryX - 0.45, startY, 0.9, length);

  const pulseHeight = 40;
  for (
    let pulseY = alignedLoopStart(startY - PERIMETER_MODULE_SPACING + energyTravel, PERIMETER_MODULE_SPACING);
    pulseY < endY + PERIMETER_MODULE_SPACING;
    pulseY += PERIMETER_MODULE_SPACING
  ) {
    ctx.fillStyle = "rgba(232, 252, 255, 0.82)";
    ctx.fillRect(boundaryX - 1.8, pulseY, 3.6, pulseHeight);
    ctx.fillStyle = "rgba(52, 191, 255, 0.32)";
    ctx.fillRect(boundaryX - 4, pulseY - 8, 8, pulseHeight + 16);
  }

  const moduleSprite = getOrCreateVerticalPerimeterModuleSprite(dpr);
  for (
    let moduleStartY = alignedLoopStart(startY, PERIMETER_MODULE_SPACING) - PERIMETER_MODULE_SPACING / 2;
    moduleStartY < endY + PERIMETER_MODULE_SPACING;
    moduleStartY += PERIMETER_MODULE_SPACING
  ) {
    if (moduleSprite) {
      if (insideDirection === 1) {
        ctx.drawImage(
          moduleSprite.canvas,
          boundaryX,
          moduleStartY,
          moduleSprite.logicalWidth,
          moduleSprite.logicalHeight,
        );
      } else {
        ctx.save();
        ctx.translate(boundaryX, moduleStartY);
        ctx.scale(-1, 1);
        ctx.drawImage(moduleSprite.canvas, 0, 0, moduleSprite.logicalWidth, moduleSprite.logicalHeight);
        ctx.restore();
      }
      continue;
    }

    const moduleCenter = moduleStartY + PERIMETER_MODULE_SPACING / 2;
    const housingY = moduleCenter - 20;
    const towerY = moduleCenter - 7;

    ctx.fillStyle = "#253140";
    ctx.fillRect(edgeOffsetStart(boundaryX, 4, 12, insideDirection), housingY, 12, 40);
    ctx.fillStyle = "#0f1621";
    ctx.fillRect(edgeOffsetStart(boundaryX, 0, 18, insideDirection), towerY, 18, 14);
    ctx.fillStyle = "rgba(232, 240, 255, 0.12)";
    ctx.fillRect(edgeOffsetStart(boundaryX, 6, 1, insideDirection), housingY + 5, 1, 30);
    ctx.fillStyle = "rgba(255, 179, 71, 0.72)";
    ctx.fillRect(edgeOffsetStart(boundaryX, 10, 4, insideDirection), moduleCenter - 2, 4, 4);
    ctx.fillStyle = "rgba(91, 121, 164, 0.88)";
    ctx.fillRect(edgeOffsetStart(boundaryX, 9, 6, insideDirection), moduleCenter - 30, 6, 8);
    ctx.fillRect(edgeOffsetStart(boundaryX, 9, 6, insideDirection), moduleCenter + 22, 6, 8);
  }
}

function drawCornerAnchor(
  ctx: CanvasRenderingContext2D,
  cornerX: number,
  cornerY: number,
  xDirection: 1 | -1,
  yDirection: 1 | -1,
  dpr: number,
  nowMs: number,
) {
  const cornerSprite = getOrCreatePerimeterCornerSprite(dpr);
  if (cornerSprite) {
    ctx.save();
    ctx.translate(cornerX, cornerY);
    ctx.scale(xDirection, yDirection);
    ctx.drawImage(
      cornerSprite.canvas,
      0,
      0,
      cornerSprite.logicalWidth,
      cornerSprite.logicalHeight,
    );
    ctx.restore();
  } else {
    const point = (x: number, y: number) => ({
      x: cornerX + x * xDirection,
      y: cornerY + y * yDirection,
    });

    fillPolygon(
      ctx,
      [
        point(0, 14),
        point(14, 0),
        point(PERIMETER_CORNER_ANCHOR_SIZE, 0),
        point(PERIMETER_CORNER_ANCHOR_SIZE, 18),
        point(18, PERIMETER_CORNER_ANCHOR_SIZE),
        point(0, PERIMETER_CORNER_ANCHOR_SIZE),
      ],
      "#1f2935",
    );
    fillPolygon(
      ctx,
      [
        point(0, 8),
        point(8, 0),
        point(PERIMETER_CORNER_ANCHOR_SIZE - 10, 0),
        point(PERIMETER_CORNER_ANCHOR_SIZE - 10, 9),
        point(9, PERIMETER_CORNER_ANCHOR_SIZE - 10),
        point(0, PERIMETER_CORNER_ANCHOR_SIZE - 10),
      ],
      "rgba(255, 255, 255, 0.06)",
    );
    strokePolygon(
      ctx,
      [
        point(0, 14),
        point(14, 0),
        point(PERIMETER_CORNER_ANCHOR_SIZE, 0),
        point(PERIMETER_CORNER_ANCHOR_SIZE, 18),
        point(18, PERIMETER_CORNER_ANCHOR_SIZE),
        point(0, PERIMETER_CORNER_ANCHOR_SIZE),
      ],
      "rgba(232, 240, 255, 0.12)",
      1.2,
    );
  }

  const pulse = 0.76 + (Math.sin(nowMs * 0.004 + cornerX * 0.01 + cornerY * 0.008) * 0.5 + 0.5) * 0.34;
  ctx.fillStyle = `rgba(127, 231, 255, ${0.22 * pulse})`;
  ctx.beginPath();
  ctx.arc(cornerX + 24 * xDirection, cornerY + 24 * yDirection, 16, 0, Math.PI * 2);
  ctx.fill();

  ctx.fillStyle = `rgba(239, 255, 255, ${0.9 * pulse})`;
  ctx.beginPath();
  ctx.arc(cornerX + 24 * xDirection, cornerY + 24 * yDirection, 4.5, 0, Math.PI * 2);
  ctx.fill();

  ctx.strokeStyle = "rgba(104, 225, 253, 0.52)";
  ctx.lineWidth = 1.6;
  ctx.beginPath();
  ctx.moveTo(cornerX + 18 * xDirection, cornerY + 38 * yDirection);
  ctx.lineTo(cornerX + 38 * xDirection, cornerY + 18 * yDirection);
  ctx.stroke();
}

function drawWorldPerimeter(
  ctx: CanvasRenderingContext2D,
  worldWidth: number,
  worldHeight: number,
  camX: number,
  camY: number,
  width: number,
  height: number,
  nowMs: number,
) {
  const dpr = typeof window === "undefined" ? 1 : window.devicePixelRatio || 1;
  const visibleLeft = camX - PERIMETER_VIEW_MARGIN;
  const visibleRight = camX + width + PERIMETER_VIEW_MARGIN;
  const visibleTop = camY - PERIMETER_VIEW_MARGIN;
  const visibleBottom = camY + height + PERIMETER_VIEW_MARGIN;
  const topEdgeStartX = clamp(visibleLeft, 0, worldWidth);
  const topEdgeEndX = clamp(visibleRight, 0, worldWidth);
  const leftEdgeStartY = clamp(visibleTop, 0, worldHeight);
  const leftEdgeEndY = clamp(visibleBottom, 0, worldHeight);

  if (visibleTop <= PERIMETER_OUTER_FIELD_DEPTH + PERIMETER_INNER_RAIL_DEPTH) {
    drawHorizontalPerimeterEdge(ctx, 0, 1, topEdgeStartX, topEdgeEndX, dpr, nowMs);
  }
  if (visibleBottom >= worldHeight - (PERIMETER_OUTER_FIELD_DEPTH + PERIMETER_INNER_RAIL_DEPTH)) {
    drawHorizontalPerimeterEdge(ctx, worldHeight, -1, topEdgeStartX, topEdgeEndX, dpr, nowMs);
  }
  if (visibleLeft <= PERIMETER_OUTER_FIELD_DEPTH + PERIMETER_INNER_RAIL_DEPTH) {
    drawVerticalPerimeterEdge(ctx, 0, 1, leftEdgeStartY, leftEdgeEndY, dpr, nowMs);
  }
  if (visibleRight >= worldWidth - (PERIMETER_OUTER_FIELD_DEPTH + PERIMETER_INNER_RAIL_DEPTH)) {
    drawVerticalPerimeterEdge(ctx, worldWidth, -1, leftEdgeStartY, leftEdgeEndY, dpr, nowMs);
  }

  if (
    visibleLeft <= PERIMETER_CORNER_ANCHOR_SIZE &&
    visibleTop <= PERIMETER_CORNER_ANCHOR_SIZE
  ) {
    drawCornerAnchor(ctx, 0, 0, 1, 1, dpr, nowMs);
  }
  if (
    visibleRight >= worldWidth - PERIMETER_CORNER_ANCHOR_SIZE &&
    visibleTop <= PERIMETER_CORNER_ANCHOR_SIZE
  ) {
    drawCornerAnchor(ctx, worldWidth, 0, -1, 1, dpr, nowMs);
  }
  if (
    visibleLeft <= PERIMETER_CORNER_ANCHOR_SIZE &&
    visibleBottom >= worldHeight - PERIMETER_CORNER_ANCHOR_SIZE
  ) {
    drawCornerAnchor(ctx, 0, worldHeight, 1, -1, dpr, nowMs);
  }
  if (
    visibleRight >= worldWidth - PERIMETER_CORNER_ANCHOR_SIZE &&
    visibleBottom >= worldHeight - PERIMETER_CORNER_ANCHOR_SIZE
  ) {
    drawCornerAnchor(ctx, worldWidth, worldHeight, -1, -1, dpr, nowMs);
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

function resolvePlayerSpriteId(spriteId?: string, spriteVariant?: number): string {
  const candidateSpriteId = spriteId ?? "";
  if (hasPlayerSpriteId(candidateSpriteId)) {
    return candidateSpriteId;
  }
  const variantSpriteId = getPlayerSpriteIdForVariant(spriteVariant);
  if (hasPlayerSpriteId(variantSpriteId)) {
    return variantSpriteId;
  }
  return DEFAULT_PLAYER_SPRITE_ID;
}

function getOrCreatePlayerSpriteImageState(resolvedSpriteId: string): PlayerSpriteImageState {
  const cached = playerSpriteImageCache.get(resolvedSpriteId);
  if (cached) {
    return cached;
  }

  const state: PlayerSpriteImageState = {
    ready: false,
    failed: false,
  };

  if (typeof Image === "undefined") {
    state.failed = true;
    playerSpriteImageCache.set(resolvedSpriteId, state);
    return state;
  }

  const image = new Image();
  image.onload = () => {
    state.ready = true;
  };
  image.onerror = () => {
    state.failed = true;
  };
  image.src = getPlayerSpriteUrl(resolvedSpriteId);
  state.image = image;

  playerSpriteImageCache.set(resolvedSpriteId, state);
  return state;
}

function getPlayerExhaustPhase(playerId: string) {
  const cached = playerExhaustPhaseCache.get(playerId);
  if (cached !== undefined) {
    return cached;
  }
  const phase = (hashSeed(playerId) / 0xffffffff) * Math.PI * 2;
  playerExhaustPhaseCache.set(playerId, phase);
  return phase;
}

function getExhaustNozzles(spriteId: string) {
  return EXHAUST_NOZZLES_BY_SPRITE[spriteId] ?? DEFAULT_EXHAUST_NOZZLES;
}

function drawPlayerExhaust(
  ctx: CanvasRenderingContext2D,
  player: WorldPlayer,
  spriteId: string,
  targetWidth: number,
  targetHeight: number,
  isSelf: boolean,
) {
  const speed = Math.hypot(player.vx, player.vy);
  const throttle = clamp((speed - EXHAUST_MIN_SPEED) / (EXHAUST_MAX_SPEED - EXHAUST_MIN_SPEED), 0, 1);
  if (throttle <= 0.03) {
    return;
  }
  const baseIntensity = 0.2 + throttle * 0.9;
  const nowSeconds = (typeof performance === "undefined" ? Date.now() : performance.now()) / 1000;
  const phase = getPlayerExhaustPhase(player.id);
  const flicker =
    0.88 + Math.sin(nowSeconds * 24 + phase) * 0.12 + Math.sin(nowSeconds * 41 + phase * 1.7) * 0.05;
  const intensity = clamp(baseIntensity * flicker, 0.2, 1.25);
  const nozzles = getExhaustNozzles(spriteId);

  ctx.globalCompositeOperation = "lighter";
  for (let index = 0; index < nozzles.length; index += 1) {
    const nozzle = nozzles[index];
    const nozzleX = nozzle.x * targetWidth;
    const nozzleY = nozzle.y * targetHeight;
    const nozzleScale = nozzle.scale;
    const pulse =
      1 +
      Math.sin(nowSeconds * (16 + index * 2) + phase * (1 + index * 0.23)) * 0.08;

    const plumeLength =
      targetWidth * (0.19 + intensity * 0.3) * nozzleScale * pulse * EXHAUST_SIZE_BOOST;
    const wakeWidth =
      targetHeight * (0.035 + intensity * 0.032) * nozzleScale * EXHAUST_SIZE_BOOST;
    const coreWidth = wakeWidth * 0.45;
    const hue = isSelf ? "145, 244, 255" : "127, 219, 255";

    const wakeGradient = ctx.createLinearGradient(nozzleX, nozzleY, nozzleX - plumeLength, nozzleY);
    wakeGradient.addColorStop(0, `rgba(${hue}, ${0.55 + intensity * 0.25})`);
    wakeGradient.addColorStop(0.5, `rgba(${hue}, ${0.3 + intensity * 0.15})`);
    wakeGradient.addColorStop(1, "rgba(0, 0, 0, 0)");
    ctx.strokeStyle = wakeGradient;
    ctx.lineCap = "round";
    ctx.lineWidth = wakeWidth;
    ctx.beginPath();
    ctx.moveTo(nozzleX, nozzleY);
    ctx.lineTo(nozzleX - plumeLength, nozzleY);
    ctx.stroke();

    const coreGradient = ctx.createLinearGradient(nozzleX, nozzleY, nozzleX - plumeLength * 0.74, nozzleY);
    coreGradient.addColorStop(0, "rgba(245, 255, 255, 0.98)");
    coreGradient.addColorStop(0.55, "rgba(168, 242, 255, 0.62)");
    coreGradient.addColorStop(1, "rgba(0, 0, 0, 0)");
    ctx.strokeStyle = coreGradient;
    ctx.lineWidth = coreWidth;
    ctx.beginPath();
    ctx.moveTo(nozzleX, nozzleY);
    ctx.lineTo(nozzleX - plumeLength * 0.74, nozzleY);
    ctx.stroke();

    const haloRadius = wakeWidth * (1.2 + intensity * 0.7) * 1.15;
    const halo = ctx.createRadialGradient(nozzleX, nozzleY, 0, nozzleX, nozzleY, haloRadius);
    halo.addColorStop(0, "rgba(250, 255, 255, 0.9)");
    halo.addColorStop(0.55, "rgba(130, 220, 255, 0.48)");
    halo.addColorStop(1, "rgba(0, 0, 0, 0)");
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(nozzleX, nozzleY, haloRadius, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalCompositeOperation = "source-over";
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

function drawPlayerSprite(
  ctx: CanvasRenderingContext2D,
  player: WorldPlayer,
  isSelf: boolean,
): "drawn" | "loading" | "failed" {
  const resolvedSpriteId = resolvePlayerSpriteId(player.spriteId, player.spriteVariant);
  const spriteState = getOrCreatePlayerSpriteImageState(resolvedSpriteId);
  const image = spriteState.image;
  if (spriteState.failed || !image) {
    return "failed";
  }
  if (!spriteState.ready || !image.complete) {
    return "loading";
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
  drawPlayerExhaust(ctx, player, resolvedSpriteId, targetWidth, targetHeight, isSelf);
  ctx.drawImage(
    image,
    -targetWidth / 2,
    -targetHeight / 2,
    targetWidth,
    targetHeight,
  );
  ctx.restore();
  return "drawn";
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

function clearAllEffects() {
  playerExhaustPhaseCache.clear();
  trackedProjectiles.clear();
  trackedPlayerAlive.clear();
  currentProjectileIdsScratch.clear();
  currentPlayerIdsScratch.clear();
  activeImpactEffectIndices.length = 0;
  activeExplosionEffectIndices.length = 0;
  activePickupTextEffectIndices.length = 0;
  impactActiveListIndexBySlot.fill(-1);
  explosionActiveListIndexBySlot.fill(-1);
  pickupTextActiveListIndexBySlot.fill(-1);
  for (let index = 0; index < impactEffects.length; index += 1) {
    impactEffects[index].active = false;
  }
  for (let index = 0; index < explosionEffects.length; index += 1) {
    explosionEffects[index].active = false;
  }
  for (let index = 0; index < pickupTextEffects.length; index += 1) {
    pickupTextEffects[index].active = false;
  }
  nextImpactEffectIndex = 0;
  nextExplosionEffectIndex = 0;
  nextPickupTextEffectIndex = 0;
  lastPickupFeedbackSequence = 0;
  lastEffectsSyncedServerTime = 0;
}

function clearEffectTrackingState() {
  trackedProjectiles.clear();
  trackedPlayerAlive.clear();
  currentProjectileIdsScratch.clear();
  currentPlayerIdsScratch.clear();
}

function removeActiveIndex(list: number[], indexBySlot: number[], indexInList: number) {
  const lastIndex = list.length - 1;
  if (indexInList < 0 || indexInList > lastIndex) {
    return;
  }
  const removedSlot = list[indexInList];
  indexBySlot[removedSlot] = -1;
  if (indexInList === lastIndex) {
    list.pop();
    return;
  }
  const movedSlot = list[lastIndex];
  list[indexInList] = movedSlot;
  indexBySlot[movedSlot] = indexInList;
  list.pop();
}

function nextImpactEffectSlot() {
  const slotIndex = nextImpactEffectIndex;
  const slot = impactEffects[slotIndex];
  nextImpactEffectIndex = (nextImpactEffectIndex + 1) % impactEffects.length;
  return { slotIndex, slot };
}

function nextExplosionEffectSlot() {
  const slotIndex = nextExplosionEffectIndex;
  const slot = explosionEffects[slotIndex];
  nextExplosionEffectIndex = (nextExplosionEffectIndex + 1) % explosionEffects.length;
  return { slotIndex, slot };
}

function nextPickupTextEffectSlot() {
  const slotIndex = nextPickupTextEffectIndex;
  const slot = pickupTextEffects[slotIndex];
  nextPickupTextEffectIndex = (nextPickupTextEffectIndex + 1) % pickupTextEffects.length;
  return { slotIndex, slot };
}

function spawnImpactEffect(
  x: number,
  y: number,
  color: string,
  radius: number,
  nowMs: number,
  seedSource: string,
) {
  const { slotIndex, slot: effect } = nextImpactEffectSlot();
  if (impactActiveListIndexBySlot[slotIndex] === -1) {
    impactActiveListIndexBySlot[slotIndex] = activeImpactEffectIndices.length;
    activeImpactEffectIndices.push(slotIndex);
  }
  effect.active = true;
  effect.x = x;
  effect.y = y;
  effect.color = color;
  effect.colorRgb = rgbTripletForColor(color);
  effect.radius = radius;
  effect.createdAtMs = nowMs;
  effect.ttlMs = 180;
  effect.seed = hashSeed(seedSource) / 0xffffffff;
}

function spawnExplosionEffect(
  x: number,
  y: number,
  color: string,
  radius: number,
  nowMs: number,
  seedSource: string,
) {
  const { slotIndex, slot: effect } = nextExplosionEffectSlot();
  if (explosionActiveListIndexBySlot[slotIndex] === -1) {
    explosionActiveListIndexBySlot[slotIndex] = activeExplosionEffectIndices.length;
    activeExplosionEffectIndices.push(slotIndex);
  }
  effect.active = true;
  effect.x = x;
  effect.y = y;
  effect.color = color;
  effect.colorRgb = rgbTripletForColor(color);
  effect.radius = radius;
  effect.createdAtMs = nowMs;
  effect.ttlMs = 620;
  effect.seed = hashSeed(seedSource) / 0xffffffff;
}

function spawnPickupTextEffect(x: number, y: number, massGain: number, healthGain: number, nowMs: number) {
  const { slotIndex, slot: effect } = nextPickupTextEffectSlot();
  if (pickupTextActiveListIndexBySlot[slotIndex] === -1) {
    pickupTextActiveListIndexBySlot[slotIndex] = activePickupTextEffectIndices.length;
    activePickupTextEffectIndices.push(slotIndex);
  }
  effect.active = true;
  effect.x = x;
  effect.y = y;
  effect.massGain = massGain;
  effect.healthGain = healthGain;
  effect.createdAtMs = nowMs;
  effect.ttlMs = 1000;
}

function syncCombatEffects(
  snapshot: SnapshotMessage,
  nowMs: number,
  camX: number,
  camY: number,
  width: number,
  height: number,
) {
  if (lastEffectsMatchId !== snapshot.matchId) {
    clearAllEffects();
    lastEffectsMatchId = snapshot.matchId;
  }
  if (lastEffectsSyncedServerTime > 0) {
    const serverTimeGap = snapshot.serverTime - lastEffectsSyncedServerTime;
    if (serverTimeGap < 0 || serverTimeGap > EFFECT_TRACKING_RESET_GAP_MS) {
      clearEffectTrackingState();
    }
  }

  currentProjectileIdsScratch.clear();
  for (let index = 0; index < snapshot.projectiles.length; index += 1) {
    const projectile = snapshot.projectiles[index];
    currentProjectileIdsScratch.add(projectile.id);
    const tracked = trackedProjectiles.get(projectile.id);
    if (tracked) {
      tracked.x = projectile.x;
      tracked.y = projectile.y;
      tracked.color = projectile.color;
      tracked.radius = projectile.radius;
      continue;
    }
    trackedProjectiles.set(projectile.id, {
      x: projectile.x,
      y: projectile.y,
      color: projectile.color,
      radius: projectile.radius,
    });
  }

  for (const [projectileId, tracked] of trackedProjectiles) {
    if (currentProjectileIdsScratch.has(projectileId)) {
      continue;
    }
    const impactCullRadius = Math.max(tracked.radius * 3.2, 8);
    if (
      isWithinViewport(
        tracked.x,
        tracked.y,
        impactCullRadius,
        camX,
        camY,
        width,
        height,
        IMPACT_SYNC_VIEWPORT_MARGIN,
      )
    ) {
      spawnImpactEffect(
        tracked.x,
        tracked.y,
        tracked.color,
        tracked.radius,
        nowMs,
        `${snapshot.matchId}|impact|${projectileId}`,
      );
    }
    trackedProjectiles.delete(projectileId);
  }

  currentPlayerIdsScratch.clear();
  for (let index = 0; index < snapshot.players.length; index += 1) {
    const player = snapshot.players[index];
    currentPlayerIdsScratch.add(player.id);
    const wasAlive = trackedPlayerAlive.get(player.id);
    if (wasAlive === true && !player.isAlive) {
      spawnExplosionEffect(
        player.x,
        player.y,
        player.color,
        player.radius,
        nowMs,
        `${snapshot.matchId}|explosion|${player.id}|${nowMs}`,
      );
    }
    trackedPlayerAlive.set(player.id, player.isAlive);
  }

  for (const playerId of trackedPlayerAlive.keys()) {
    if (!currentPlayerIdsScratch.has(playerId)) {
      trackedPlayerAlive.delete(playerId);
    }
  }

  const pickupFeedback = snapshot.you?.pickupFeedback;
  if (pickupFeedback && pickupFeedback.sequence > lastPickupFeedbackSequence) {
    const selfPlayer = snapshot.players.find((player) => player.id === snapshot.you?.playerId);
    if (selfPlayer) {
      spawnPickupTextEffect(
        selfPlayer.x,
        selfPlayer.y - selfPlayer.radius * 0.9,
        pickupFeedback.massGain,
        pickupFeedback.healthGain,
        nowMs,
      );
    }
    lastPickupFeedbackSequence = pickupFeedback.sequence;
  }

  lastEffectsSyncedServerTime = snapshot.serverTime;
}

function drawImpactEffects(
  ctx: CanvasRenderingContext2D,
  nowMs: number,
  camX: number,
  camY: number,
  width: number,
  height: number,
) {
  ctx.save();
  ctx.globalCompositeOperation = "lighter";
  for (let activeIndex = activeImpactEffectIndices.length - 1; activeIndex >= 0; activeIndex -= 1) {
    const effectIndex = activeImpactEffectIndices[activeIndex];
    const effect = impactEffects[effectIndex];
    if (!effect.active) {
      removeActiveIndex(activeImpactEffectIndices, impactActiveListIndexBySlot, activeIndex);
      continue;
    }
    const age = nowMs - effect.createdAtMs;
    if (age >= effect.ttlMs) {
      effect.active = false;
      removeActiveIndex(activeImpactEffectIndices, impactActiveListIndexBySlot, activeIndex);
      continue;
    }
    const progress = age / effect.ttlMs;
    const fade = 1 - progress;
    const flashRadius = Math.max(effect.radius * 3.2, 8) * (0.6 + progress * 0.8);
    if (!isWithinViewport(effect.x, effect.y, flashRadius, camX, camY, width, height, 48)) {
      continue;
    }

    ctx.fillStyle = rgbaFromTriplet(effect.colorRgb, 0.24 * fade);
    ctx.beginPath();
    ctx.arc(effect.x, effect.y, flashRadius, 0, Math.PI * 2);
    ctx.fill();

    const ringRadius = flashRadius * (1.1 + progress * 0.9);
    ctx.strokeStyle = rgbaFromTriplet(effect.colorRgb, 0.56 * fade);
    ctx.lineWidth = 1.2 + fade * 1.4;
    ctx.beginPath();
    ctx.arc(effect.x, effect.y, ringRadius, 0, Math.PI * 2);
    ctx.stroke();

    const sparkCount = 3;
    for (let sparkIndex = 0; sparkIndex < sparkCount; sparkIndex += 1) {
      const angle = effect.seed * Math.PI * 2 + sparkIndex * ((Math.PI * 2) / sparkCount) + progress * 6;
      const sparkLength = flashRadius * (0.55 + fade * 0.5);
      ctx.strokeStyle = rgbaFromTriplet(effect.colorRgb, 0.7 * fade);
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(effect.x, effect.y);
      ctx.lineTo(effect.x + Math.cos(angle) * sparkLength, effect.y + Math.sin(angle) * sparkLength);
      ctx.stroke();
    }
  }
  ctx.restore();
}

function drawExplosionEffects(
  ctx: CanvasRenderingContext2D,
  nowMs: number,
  camX: number,
  camY: number,
  width: number,
  height: number,
) {
  ctx.save();
  ctx.globalCompositeOperation = "lighter";
  const heavyLoadMode = activeExplosionEffectIndices.length > EXPLOSION_HEAVY_LOAD_THRESHOLD;
  for (let activeIndex = activeExplosionEffectIndices.length - 1; activeIndex >= 0; activeIndex -= 1) {
    const effectIndex = activeExplosionEffectIndices[activeIndex];
    const effect = explosionEffects[effectIndex];
    if (!effect.active) {
      removeActiveIndex(activeExplosionEffectIndices, explosionActiveListIndexBySlot, activeIndex);
      continue;
    }
    const age = nowMs - effect.createdAtMs;
    if (age >= effect.ttlMs) {
      effect.active = false;
      removeActiveIndex(activeExplosionEffectIndices, explosionActiveListIndexBySlot, activeIndex);
      continue;
    }
    const progress = age / effect.ttlMs;
    const fade = 1 - progress;
    const blastRadius = Math.max(effect.radius * 2.8, 28) * (0.58 + progress * 1.72);
    if (!isWithinViewport(effect.x, effect.y, blastRadius, camX, camY, width, height, 64)) {
      continue;
    }

    // Fireball core and bloom.
    const fireballRadius = blastRadius * (0.72 + fade * 0.3);
    if (heavyLoadMode) {
      ctx.fillStyle = rgbaFromTriplet(effect.colorRgb, 0.42 * fade);
    } else {
      const fireball = ctx.createRadialGradient(effect.x, effect.y, 0, effect.x, effect.y, fireballRadius);
      fireball.addColorStop(0, `rgba(255, 251, 235, ${0.82 * fade + 0.12})`);
      fireball.addColorStop(0.32, `rgba(255, 193, 123, ${0.72 * fade})`);
      fireball.addColorStop(0.66, `rgba(255, 112, 68, ${0.46 * fade})`);
      fireball.addColorStop(0.84, rgbaFromTriplet(effect.colorRgb, 0.28 * fade));
      fireball.addColorStop(1, "rgba(0, 0, 0, 0)");
      ctx.fillStyle = fireball;
    }
    ctx.beginPath();
    ctx.arc(effect.x, effect.y, fireballRadius, 0, Math.PI * 2);
    ctx.fill();

    // Hot ring around the fireball.
    ctx.strokeStyle = rgbaFromTriplet(FIRE_RING_RGB, 0.64 * fade);
    ctx.lineWidth = 1.4 + fade * 2.8;
    ctx.beginPath();
    ctx.arc(effect.x, effect.y, blastRadius * (0.84 + progress * 0.2), 0, Math.PI * 2);
    ctx.stroke();

    // Debris scatter streaks.
    const debrisCount = heavyLoadMode ? 4 : 8;
    for (let debrisIndex = 0; debrisIndex < debrisCount; debrisIndex += 1) {
      const baseAngle = effect.seed * Math.PI * 2 + debrisIndex * ((Math.PI * 2) / debrisCount);
      const wobble = Math.sin(progress * 9 + debrisIndex * 1.7 + effect.seed * 10) * 0.18;
      const angle = baseAngle + wobble;
      const travel = blastRadius * (0.35 + progress * 1.55);
      const shardX = effect.x + Math.cos(angle) * travel;
      const shardY = effect.y + Math.sin(angle) * travel;
      const shardLength = blastRadius * (0.13 + fade * 0.1);
      const shardWidth = 0.9 + fade * 1.9;

      ctx.strokeStyle = rgbaFromTriplet(effect.colorRgb, 0.62 * fade);
      ctx.lineWidth = shardWidth;
      ctx.beginPath();
      ctx.moveTo(shardX, shardY);
      ctx.lineTo(
        shardX - Math.cos(angle) * shardLength,
        shardY - Math.sin(angle) * shardLength,
      );
      ctx.stroke();

      const emberRadius = shardWidth * 0.6;
      ctx.fillStyle = rgbaFromTriplet(effect.colorRgb, 0.48 * fade);
      ctx.beginPath();
      ctx.arc(shardX, shardY, emberRadius, 0, Math.PI * 2);
      ctx.fill();
    }

    // Expanding shock ring that fades out.
    const shockRadius = blastRadius * (1.18 + progress * 0.62);
    ctx.strokeStyle = rgbaFromTriplet(SHOCK_RING_RGB, 0.38 * fade);
    ctx.lineWidth = 0.8 + fade * 1.7;
    ctx.beginPath();
    ctx.arc(effect.x, effect.y, shockRadius, 0, Math.PI * 2);
    ctx.stroke();
  }
  ctx.restore();
}

function formatPickupStat(value: number) {
  const rounded = Math.round(value * 10) / 10;
  if (Math.abs(rounded - Math.round(rounded)) < 0.001) {
    return `${Math.round(rounded)}`;
  }
  return rounded.toFixed(1);
}

function drawPickupTextEffects(
  ctx: CanvasRenderingContext2D,
  nowMs: number,
  camX: number,
  camY: number,
  width: number,
  height: number,
) {
  ctx.save();
  ctx.textAlign = "center";
  ctx.font = "600 12px Space Grotesk, sans-serif";
  for (let activeIndex = activePickupTextEffectIndices.length - 1; activeIndex >= 0; activeIndex -= 1) {
    const effectIndex = activePickupTextEffectIndices[activeIndex];
    const effect = pickupTextEffects[effectIndex];
    if (!effect.active) {
      removeActiveIndex(activePickupTextEffectIndices, pickupTextActiveListIndexBySlot, activeIndex);
      continue;
    }

    const age = nowMs - effect.createdAtMs;
    if (age >= effect.ttlMs) {
      effect.active = false;
      removeActiveIndex(activePickupTextEffectIndices, pickupTextActiveListIndexBySlot, activeIndex);
      continue;
    }

    const progress = age / effect.ttlMs;
    const fade = 1 - progress;
    const label = `+${formatPickupStat(effect.massGain)} mass  +${formatPickupStat(effect.healthGain)} health`;
    const textX = effect.x;
    const textY = effect.y - progress * 28;
    if (!isWithinViewport(textX, textY, 42, camX, camY, width, height, 72)) {
      continue;
    }

    ctx.globalAlpha = 0.22 * fade;
    ctx.fillStyle = "#04111b";
    ctx.fillText(label, textX, textY + 1.5);

    ctx.globalAlpha = 0.72 * fade;
    ctx.fillStyle = "rgba(232, 242, 255, 0.92)";
    ctx.fillText(label, textX, textY);
  }
  ctx.restore();
}

function drawProjectileSprite(ctx: CanvasRenderingContext2D, data: ProjectileRenderData, sprite: RailgunSprite) {
  const { x, y } = data.projectile;
  ctx.translate(x, y);
  ctx.rotate(data.angle);
  ctx.drawImage(sprite.canvas, -sprite.originX, -sprite.originY, sprite.width, sprite.height);
  ctx.rotate(-data.angle);
  ctx.translate(-x, -y);
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

  let renderableCount = 0;
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
    let renderable = projectileRenderablesScratch[renderableCount];
    if (!renderable) {
      renderable = {
        projectile,
        angle: 0,
        glow: sprites.glow,
        core: sprites.core,
      };
      projectileRenderablesScratch[renderableCount] = renderable;
    }
    renderable.projectile = projectile;
    renderable.angle = projectileAngle(projectile);
    renderable.glow = sprites.glow;
    renderable.core = sprites.core;
    renderableCount += 1;
  }

  if (renderableCount === 0) {
    ctx.globalAlpha = 1;
    return;
  }

  ctx.save();

  ctx.globalCompositeOperation = "lighter";
  for (let index = 0; index < renderableCount; index += 1) {
    const renderable = projectileRenderablesScratch[index];
    drawProjectileSprite(ctx, renderable, renderable.glow);
  }

  ctx.globalCompositeOperation = "source-over";
  for (let index = 0; index < renderableCount; index += 1) {
    const renderable = projectileRenderablesScratch[index];
    drawProjectileSprite(ctx, renderable, renderable.core);
  }

  ctx.restore();
  ctx.globalCompositeOperation = "source-over";
  ctx.globalAlpha = 1;
}

function drawPlayer(ctx: CanvasRenderingContext2D, player: WorldPlayer, isSelf: boolean) {
  ctx.globalAlpha = 1;
  drawHealthBar(ctx, player);

  const spriteRenderState = drawPlayerSprite(ctx, player, isSelf);
  if (spriteRenderState !== "drawn") {
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

function playerCullRadius(player: WorldPlayer) {
  const spriteHalfLength = Math.max(player.radius * PLAYER_SPRITE_SCALE, 28) / 2;
  return Math.max(player.radius, spriteHalfLength);
}

function selectCameraPlayer(players: WorldPlayer[], preferredId?: string) {
  if (preferredId) {
    const preferred = players.find((player) => player.id === preferredId);
    if (preferred) {
      return preferred;
    }
  }

  return [...players].sort((left, right) => {
    if (left.isAlive !== right.isAlive) {
      return left.isAlive ? -1 : 1;
    }
    if (left.name !== right.name) {
      return left.name.localeCompare(right.name);
    }
    return left.id.localeCompare(right.id);
  })[0];
}

export function renderWorld(
  ctx: CanvasRenderingContext2D,
  snapshot: SnapshotMessage,
  localPlayerId?: string,
  effectsSnapshot?: SnapshotMessage,
  cameraTargetId?: string,
) {
  const width = ctx.canvas.clientWidth || ctx.canvas.width;
  const height = ctx.canvas.clientHeight || ctx.canvas.height;
  const preferredCameraId = cameraTargetId ?? localPlayerId;
  const selfPlayer = selectCameraPlayer(snapshot.players, preferredCameraId);
  const defaultCamX = snapshot.world.width / 2 - width / 2;
  const defaultCamY = snapshot.world.height / 2 - height / 2;
  const camX = selfPlayer ? selfPlayer.x - width / 2 : defaultCamX;
  const camY = selfPlayer ? selfPlayer.y - height / 2 : defaultCamY;
  const nowMs = typeof performance === "undefined" ? Date.now() : performance.now();
  const effectSource = effectsSnapshot ?? snapshot;
  const effectCameraPlayer = selectCameraPlayer(effectSource.players, preferredCameraId) ?? selfPlayer;
  const effectCamX = effectCameraPlayer ? effectCameraPlayer.x - width / 2 : defaultCamX;
  const effectCamY = effectCameraPlayer ? effectCameraPlayer.y - height / 2 : defaultCamY;
  if (effectSource.serverTime !== lastEffectsSyncedServerTime) {
    syncCombatEffects(effectSource, nowMs, effectCamX, effectCamY, width, height);
  }

  ctx.clearRect(0, 0, width, height);
  drawSpaceBackground(ctx, width, height, camX, camY);

  ctx.save();
  ctx.translate(-camX, -camY);

  drawWorldPerimeter(ctx, snapshot.world.width, snapshot.world.height, camX, camY, width, height, nowMs);

  drawObjects(ctx, snapshot.objects, camX, camY, width, height);

  drawProjectiles(ctx, snapshot.projectiles, snapshot.matchId, camX, camY, width, height);
  drawImpactEffects(ctx, nowMs, camX, camY, width, height);
  drawExplosionEffects(ctx, nowMs, camX, camY, width, height);

  for (let index = 0; index < snapshot.players.length; index += 1) {
    const player = snapshot.players[index];
    if (!player.isAlive) {
      continue;
    }
    if (
      !isWithinViewport(
        player.x,
        player.y,
        playerCullRadius(player),
        camX,
        camY,
        width,
        height,
        64,
      )
    ) {
      continue;
    }
    drawPlayer(ctx, player, player.id === localPlayerId);
  }

  drawPickupTextEffects(ctx, nowMs, camX, camY, width, height);

  ctx.restore();
  ctx.globalAlpha = 1;
}
