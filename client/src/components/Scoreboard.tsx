import { useNavigate } from "react-router-dom";
import { useGameStore } from "../store/gameStore";

export default function Scoreboard() {
  const navigate = useNavigate();
  const matchOver = useGameStore((state) => state.matchOver);
  const rows = useGameStore((state) => state.scoreboard);

  if (!matchOver) {
    return null;
  }

  return (
    <section className="scoreboard-overlay">
      <div className="section-title">
        <div>
          <p className="eyebrow">Match Complete</p>
          <h3>Final standings</h3>
        </div>
        <button type="button" onClick={() => navigate("/")}>
          Play Again
        </button>
      </div>
      <div className="scoreboard-list">
        {rows.map((row, index) => (
          <div className="scoreboard-row" key={row.playerId}>
            <span>#{index + 1}</span>
            <strong>{row.playerName}</strong>
            <span>{row.kills} kills</span>
            <span>{row.massBonus} mass bonus</span>
            <strong>{row.totalScore} pts</strong>
          </div>
        ))}
      </div>
    </section>
  );
}
