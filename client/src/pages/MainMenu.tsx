import { FormEvent, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useGameStore } from "../store/gameStore";
import type { MatchJoinResponse } from "../engine/types";

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "";
const DEFAULT_PLAYER_NAME = "Pilot";
const AVAILABLE_REGIONS = [{ value: "local", label: "Local" }];

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
    <section className="menu-shell">
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
