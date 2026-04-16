import { useNavigate } from "react-router-dom";
import { useGameStore } from "../store/gameStore";

const MODE_OPTIONS = [
  {
    id: "nebula",
    label: "Nebula",
    subtitle: "Free For All",
    selected: true,
    disabled: true,
  },
  {
    id: "pulsar",
    label: "Pulsar",
    subtitle: "Squadron",
    selected: false,
    disabled: true,
  },
  {
    id: "quasar",
    label: "Quasar",
    subtitle: "Gifted Drift",
    selected: false,
    disabled: true,
  },
] as const;

const DEFAULT_REGION = "local";
const DEFAULT_BUILD_LABEL = "v0.1.0";
const GAME_REGION_LABEL = (import.meta.env.VITE_GAME_REGION ?? DEFAULT_REGION).trim() || DEFAULT_REGION;
const BUILD_LABEL = (import.meta.env.VITE_APP_VERSION ?? DEFAULT_BUILD_LABEL).trim() || DEFAULT_BUILD_LABEL;

function formatCountdown(totalSeconds: number) {
  const safeSeconds = Math.max(totalSeconds, 0);
  const minutes = Math.floor(safeSeconds / 60);
  const seconds = safeSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export default function Scoreboard() {
  const navigate = useNavigate();
  const matchOver = useGameStore((state) => state.matchOver);
  const intermissionRemainingMs = useGameStore((state) => state.intermissionRemainingMs);
  const rows = useGameStore((state) => state.scoreboard);
  const localPlayerId = useGameStore((state) => state.localPlayerId);

  if (!matchOver) {
    return null;
  }

  const countdownSeconds = Math.max(Math.ceil(intermissionRemainingMs / 1000), 0);
  const countdownLabel = formatCountdown(countdownSeconds);
  const selectedMode = MODE_OPTIONS.find((mode) => mode.selected) ?? MODE_OPTIONS[0];
  const pilotLabel = rows.length === 1 ? "pilot" : "pilots";
  const lobbyCountLabel = `${rows.length} ${pilotLabel} in lobby`;

  return (
    <section className="scoreboard-overlay intermission-overlay">
      <header className="intermission-header">
        <h3 className="intermission-brand">astrodrift.io</h3>
        <div className="intermission-status-card">
          <div>
            <p className="eyebrow">Round Complete</p>
            <p className="intermission-status-copy">next drift in</p>
            <p className="intermission-countdown">{countdownLabel}</p>
          </div>
          <div className="intermission-status-meta">
            <p className="eyebrow">Just Played</p>
            <strong>{`${selectedMode.label} - ${selectedMode.subtitle}`}</strong>
          </div>
        </div>
      </header>

      <section className="intermission-mode-section">
        <p className="eyebrow">Choose Next Game Mode</p>
        <div className="intermission-mode-grid">
          {MODE_OPTIONS.map((mode) => (
            <button
              key={mode.id}
              type="button"
              disabled={mode.disabled}
              className={`intermission-mode-button${mode.selected ? " selected" : ""}`}
              aria-label={`${mode.label} mode`}
              aria-pressed={mode.selected}
            >
              <div className="intermission-mode-header">
                <strong>{mode.label}</strong>
                {mode.selected ? <span>Selected</span> : null}
              </div>
              <span>{mode.subtitle}</span>
            </button>
          ))}
        </div>
      </section>

      <section className="intermission-leaderboard-section">
        <strong className="intermission-leaderboard-title">Leaderboard</strong>
        <div className="intermission-leaderboard-header">
          <span>#</span>
          <span>Callsign</span>
          <span>Score</span>
          <span>Kills</span>
          <span>Mass Bonus</span>
        </div>
        <div className="scoreboard-list">
          {rows.map((row, index) => (
            <div className="scoreboard-row intermission-scoreboard-row" key={row.playerId}>
              <span>{index + 1}</span>
              <strong>
                {row.playerName}
                {localPlayerId === row.playerId ? <em className="intermission-you-badge">You</em> : null}
              </strong>
              <span>{row.totalScore.toLocaleString()}</span>
              <span>{row.kills}</span>
              <span>{row.massBonus}</span>
            </div>
          ))}
        </div>
      </section>

      <div className="intermission-actions">
        <button type="button" onClick={() => navigate("/")}>
          Leave Lobby
        </button>
      </div>

      <div className="intermission-meta">
        <span>{`Region: ${GAME_REGION_LABEL}`}</span>
        <span>{lobbyCountLabel}</span>
        <span>{`Build ${BUILD_LABEL}`}</span>
      </div>
    </section>
  );
}
