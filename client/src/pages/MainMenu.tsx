import { CSSProperties, FormEvent, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useGameStore } from "../store/gameStore";
import type { MatchJoinResponse } from "../engine/types";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "";
const DEFAULT_PLAYER_NAME = "Pilot";
const AVAILABLE_REGIONS = [{ value: "local", label: "Local" }];
const DEFAULT_STAR_SEED = (import.meta.env.VITE_MENU_STAR_SEED as string | undefined)?.trim();
const STAR_SEED_QUERY_PARAM = "starSeed";
const AMBIENT_STAR_COLORS = [
  "rgba(205, 228, 255, 0.82)",
  "rgba(179, 212, 255, 0.8)",
  "rgba(173, 200, 255, 0.74)",
  "rgba(255, 233, 198, 0.68)",
  "rgba(255, 224, 188, 0.62)",
];
const HERO_STAR_COLORS = [
  "rgba(219, 237, 255, 0.94)",
  "rgba(204, 229, 255, 0.9)",
  "rgba(216, 236, 255, 0.92)",
  "rgba(255, 222, 176, 0.8)",
  "rgba(255, 237, 211, 0.78)",
];

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

function buildStarLayer(options: {
  random: () => number;
  count: number;
  colors: string[];
  minSizePx: number;
  maxSizePx: number;
}): string {
  const gradients: string[] = [];
  for (let star = 0; star < options.count; star += 1) {
    const x = randomInRange(options.random, 0, 100);
    const y = randomInRange(options.random, 0, 100);
    const size = randomInRange(options.random, options.minSizePx, options.maxSizePx);
    const fadeSize = size + randomInRange(options.random, 0.6, 1.8);
    const color = options.colors[Math.floor(options.random() * options.colors.length)] ?? options.colors[0];
    gradients.push(
      `radial-gradient(circle at ${x.toFixed(2)}% ${y.toFixed(2)}%, ${color} 0 ${size.toFixed(
        2,
      )}px, transparent ${fadeSize.toFixed(2)}px)`,
    );
  }
  return gradients.join(", ");
}

function resolveStarSeed(): string | undefined {
  if (typeof window === "undefined") {
    return DEFAULT_STAR_SEED;
  }
  const querySeed = new URLSearchParams(window.location.search).get(STAR_SEED_QUERY_PARAM)?.trim();
  if (querySeed) {
    return querySeed;
  }
  return DEFAULT_STAR_SEED;
}

export default function MainMenu() {
  const navigate = useNavigate();
  const [playerName, setPlayerName] = useState(() => localStorage.getItem("multgame.playerName") ?? "");
  const [region, setRegion] = useState("local");
  const [isRegionMenuOpen, setIsRegionMenuOpen] = useState(false);
  const [isJoining, setIsJoining] = useState(false);
  const [error, setError] = useState<string>();
  const [isNameFocused, setIsNameFocused] = useState(false);
  const [hasRangeSelection, setHasRangeSelection] = useState(false);
  const [caretIndex, setCaretIndex] = useState(playerName.length);
  const [cursorOffsetPx, setCursorOffsetPx] = useState(0);
  const [isCursorVisible, setIsCursorVisible] = useState(true);
  const [cursorBlinkSeed, setCursorBlinkSeed] = useState(0);
  const cursorMeasureRef = useRef<HTMLSpanElement>(null);
  const setStoredPlayerName = useGameStore((state) => state.setPlayerName);
  const selectedRegionLabel =
    AVAILABLE_REGIONS.find((entry) => entry.value === region)?.label ?? AVAILABLE_REGIONS[0].label;
  const configuredStarSeed = useMemo(() => resolveStarSeed(), []);
  const starBackground = useMemo(() => {
    const random = configuredStarSeed ? createSeededRng(configuredStarSeed) : Math.random;
    return {
      ambient: buildStarLayer({
        random,
        count: 90,
        colors: AMBIENT_STAR_COLORS,
        minSizePx: 0.8,
        maxSizePx: 1.2,
      }),
      hero: buildStarLayer({
        random,
        count: 26,
        colors: HERO_STAR_COLORS,
        minSizePx: 1.6,
        maxSizePx: 2.25,
      }),
    };
  }, [configuredStarSeed]);
  const shouldShowBlockCursor = isNameFocused && !hasRangeSelection;
  const cursorCharacter = playerName.charAt(caretIndex) || "\u00a0";

  useLayoutEffect(() => {
    setCursorOffsetPx(cursorMeasureRef.current?.getBoundingClientRect().width ?? 0);
  }, [caretIndex, playerName]);

  useEffect(() => {
    if (!shouldShowBlockCursor) {
      return;
    }

    setIsCursorVisible(true);

    let blinkIntervalId: number | undefined;
    const idleTimeoutId = window.setTimeout(() => {
      setIsCursorVisible(false);
      blinkIntervalId = window.setInterval(() => {
        setIsCursorVisible((current) => !current);
      }, 500);
    }, 520);

    return () => {
      window.clearTimeout(idleTimeoutId);
      if (blinkIntervalId !== undefined) {
        window.clearInterval(blinkIntervalId);
      }
    };
  }, [shouldShowBlockCursor, cursorBlinkSeed]);

  function resetCursorBlink() {
    setIsCursorVisible(true);
    setCursorBlinkSeed((current) => current + 1);
  }

  function syncCaretFromInput(inputElement: HTMLInputElement) {
    const selectionStart = inputElement.selectionStart ?? inputElement.value.length;
    const selectionEnd = inputElement.selectionEnd ?? selectionStart;
    setCaretIndex(selectionStart);
    setHasRangeSelection(selectionStart !== selectionEnd);
    resetCursorBlink();
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setIsJoining(true);
    setError(undefined);

    try {
      const trimmedName = playerName.trim().slice(0, 18);
      const submittedName = trimmedName || DEFAULT_PLAYER_NAME;

      const response = await fetch(`${API_BASE_URL}/api/matchmaking/join`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ playerName: submittedName, region }),
      });

      if (!response.ok) {
        throw new Error("Join request failed.");
      }

      const match = (await response.json()) as MatchJoinResponse;
      localStorage.setItem("multgame.playerName", submittedName);
      setStoredPlayerName(submittedName);
      navigate("/game", { state: { match } });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Unable to join a match.");
    } finally {
      setIsJoining(false);
    }
  }

  return (
    <section
      className="menu-shell"
      style={
        {
          "--menu-stars-ambient": starBackground.ambient,
          "--menu-stars-hero": starBackground.hero,
        } as CSSProperties
      }
    >
      <p className="terminal-brand">astrodrift.io</p>
      <section className="card matchmaking-card">
        <div className="menu-status-row">
          <span>drift console // low-light transit</span>
          <span>{selectedRegionLabel.toLowerCase()} link</span>
        </div>
        <div className="section-title">
          <div className="terminal-title-box">
            <h2>somewhere between the stars</h2>
          </div>
        </div>
        <p className="muted menu-copy">Enter your callsign and initialize drift sequence.</p>
        <form
          className="form-stack"
          onSubmit={(event) => {
            void handleSubmit(event);
          }}
        >
          <label className="stack">
            <span className="field-label">Callsign</span>
            <div className="terminal-input-wrap">
              <input
                maxLength={18}
                placeholder="your name"
                value={playerName}
                onChange={(event) => {
                  setPlayerName(event.target.value);
                  syncCaretFromInput(event.target);
                }}
                onFocus={(event) => {
                  setIsNameFocused(true);
                  syncCaretFromInput(event.target);
                }}
                onBlur={() => {
                  setIsNameFocused(false);
                  setHasRangeSelection(false);
                  setIsCursorVisible(false);
                }}
                onSelect={(event) => syncCaretFromInput(event.currentTarget)}
                onClick={(event) => syncCaretFromInput(event.currentTarget)}
                onKeyDown={(event) => syncCaretFromInput(event.currentTarget)}
                onKeyUp={(event) => syncCaretFromInput(event.currentTarget)}
              />
              <span className="terminal-input-cursor-track" aria-hidden>
                <span ref={cursorMeasureRef}>{playerName.slice(0, caretIndex)}</span>
              </span>
              <span
                className={`terminal-input-cursor-block${
                  shouldShowBlockCursor && isCursorVisible ? " visible" : " hidden"
                }`}
                style={{ left: `calc(1px + var(--terminal-input-pad-x) + ${cursorOffsetPx}px)` }}
                aria-hidden
              >
                {cursorCharacter}
              </span>
            </div>
          </label>

          <div className="form-actions">
            <button type="submit" disabled={isJoining}>
              {isJoining ? "Linking..." : "Drift"}
            </button>
          </div>
          {error ? <p className="danger">{error}</p> : null}
        </form>
        <div className="menu-footer-meta">
          <span>cockpit sync: nominal</span>
          <span>build v0.1.0</span>
        </div>
        <button
          className="corner-region-button"
          type="button"
          aria-haspopup="menu"
          aria-expanded={isRegionMenuOpen}
          onClick={() => setIsRegionMenuOpen((current) => !current)}
        >
          Region: {selectedRegionLabel}
        </button>
        {isRegionMenuOpen ? (
          <div className="region-menu" role="menu" aria-label="Region options">
            {AVAILABLE_REGIONS.map((entry) => (
              <button
                key={entry.value}
                className={`region-menu-item${region === entry.value ? " active" : ""}`}
                type="button"
                role="menuitemradio"
                aria-checked={region === entry.value}
                onClick={() => {
                  setRegion(entry.value);
                  setIsRegionMenuOpen(false);
                }}
              >
                {entry.label}
              </button>
            ))}
          </div>
        ) : null}
      </section>
    </section>
  );
}
