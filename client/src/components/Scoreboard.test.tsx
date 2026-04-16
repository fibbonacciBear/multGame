import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useGameStore } from "../store/gameStore";
import Scoreboard from "./Scoreboard";

const navigateMock = vi.fn();

vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual<typeof import("react-router-dom")>("react-router-dom");
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

describe("Scoreboard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useGameStore.setState({
      matchOver: true,
      intermissionRemainingMs: 9500,
      scoreboard: [
        {
          playerId: "player-1",
          playerName: "Pilot",
          kills: 3,
          finalMass: 75,
          massBonus: 1,
          totalScore: 4,
          isBot: false,
        },
      ],
    });
  });

  it("shows the rematch countdown and leaves the match from the scoreboard", () => {
    render(
      <MemoryRouter>
        <Scoreboard />
      </MemoryRouter>,
    );

    expect(screen.getByText("00:10")).toBeTruthy();
    const nebulaMode = screen.getByRole("button", { name: "Nebula mode" }) as HTMLButtonElement;
    const pulsarMode = screen.getByRole("button", { name: "Pulsar mode" }) as HTMLButtonElement;
    const quasarMode = screen.getByRole("button", { name: "Quasar mode" }) as HTMLButtonElement;
    expect(nebulaMode.disabled).toBe(true);
    expect(pulsarMode.disabled).toBe(true);
    expect(quasarMode.disabled).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Leave Lobby" }));

    expect(navigateMock).toHaveBeenCalledWith("/");
  });
});
