import { FormEvent, useState } from "react";
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
  const setStoredPlayerName = useGameStore((state) => state.setPlayerName);
  const selectedRegionLabel =
    AVAILABLE_REGIONS.find((entry) => entry.value === region)?.label ?? AVAILABLE_REGIONS[0].label;

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
      <p className="terminal-brand">astrodrift</p>
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
            <span>Callsign</span>
            <input
              maxLength={18}
              placeholder="your name"
              value={playerName}
              onChange={(event) => setPlayerName(event.target.value)}
            />
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
