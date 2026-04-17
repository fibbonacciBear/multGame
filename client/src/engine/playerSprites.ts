type SpriteModule = {
  default: string;
};

const PLAYER_SPRITE_MODULES = import.meta.glob<SpriteModule>(
  "../assets/sprites/players/*.{png,jpg,jpeg,webp}",
  { eager: true },
);

function pathToSpriteId(path: string) {
  const filename = path.split("/").pop() ?? "sprite";
  return filename.replace(/\.[^/.]+$/, "").toLowerCase();
}

function buildSpriteUrlMap() {
  const entries = Object.entries(PLAYER_SPRITE_MODULES)
    .map(([path, mod]) => [pathToSpriteId(path), mod.default] as const)
    .sort(([left], [right]) => left.localeCompare(right));

  return Object.fromEntries(entries) as Record<string, string>;
}

const PLAYER_SPRITE_URLS = buildSpriteUrlMap();
const PLAYER_SPRITE_IDS = Object.keys(PLAYER_SPRITE_URLS);

export const DEFAULT_PLAYER_SPRITE_ID = PLAYER_SPRITE_IDS[0] ?? "test-player";

export function hasPlayerSpriteId(spriteId?: string) {
  const candidate = (spriteId ?? "").toLowerCase();
  return Boolean(candidate && PLAYER_SPRITE_URLS[candidate]);
}

export function getPlayerSpriteUrl(spriteId?: string) {
  const candidate = (spriteId ?? "").toLowerCase();
  if (candidate && PLAYER_SPRITE_URLS[candidate]) {
    return PLAYER_SPRITE_URLS[candidate];
  }
  return PLAYER_SPRITE_URLS[DEFAULT_PLAYER_SPRITE_ID] ?? "";
}

export function getPlayerSpriteIdForVariant(spriteVariant?: number) {
  if (PLAYER_SPRITE_IDS.length === 0) {
    return DEFAULT_PLAYER_SPRITE_ID;
  }
  const numericVariant = Number.isFinite(spriteVariant) ? Math.abs(Math.trunc(spriteVariant as number)) : 0;
  const index = numericVariant % PLAYER_SPRITE_IDS.length;
  return PLAYER_SPRITE_IDS[index];
}

export function getAvailablePlayerSpriteIds() {
  return [...PLAYER_SPRITE_IDS];
}
