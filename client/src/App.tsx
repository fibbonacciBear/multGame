import { Route, Routes, useLocation } from "react-router-dom";
import MainMenu from "./pages/MainMenu";
import GamePage from "./pages/Game";

export default function App() {
  const location = useLocation();
  const isMainMenuRoute = location.pathname === "/";
  const isGameRoute = location.pathname === "/game";

  return (
    <div
      className={`app-shell${isMainMenuRoute ? " app-shell-menu" : ""}${isGameRoute ? " app-shell-game" : ""}`}
    >
      <main className="app-main">
        <Routes>
          <Route path="/" element={<MainMenu />} />
          <Route path="/game" element={<GamePage />} />
        </Routes>
      </main>
    </div>
  );
}
