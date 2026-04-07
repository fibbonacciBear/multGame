import { NavLink, Route, Routes } from "react-router-dom";
import MainMenu from "./pages/MainMenu";
import GamePage from "./pages/Game";
import LeaderboardPage from "./pages/Leaderboard";

export default function App() {
  return (
    <div className="app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">Server-authoritative IO prototype</p>
          <h1>MultGame</h1>
        </div>
        <nav className="topnav">
          <NavLink to="/">Menu</NavLink>
          <NavLink to="/leaderboard">Leaderboard</NavLink>
        </nav>
      </header>
      <main className="app-main">
        <Routes>
          <Route path="/" element={<MainMenu />} />
          <Route path="/game" element={<GamePage />} />
          <Route path="/leaderboard" element={<LeaderboardPage />} />
        </Routes>
      </main>
    </div>
  );
}
