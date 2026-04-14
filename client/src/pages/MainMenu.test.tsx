import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import MainMenu from "./MainMenu";

const navigateMock = vi.fn();

vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual<typeof import("react-router-dom")>("react-router-dom");
  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

describe("MainMenu", () => {
  it("loads leaderboard preview and joins a match", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve([{ playerName: "Ace", kills: 3, massBonus: 2, totalScore: 5 }]),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () =>
          Promise.resolve({
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-1",
            lobbyId: "test",
            token: "token-1",
          }),
      });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <MemoryRouter>
        <MainMenu />
      </MemoryRouter>,
    );

    expect(await screen.findByText("Ace")).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText("Pilot callsign"), {
      target: { value: "Pilot One" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Play" }));

    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/game", {
        state: {
          match: {
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-1",
            lobbyId: "test",
            token: "token-1",
          },
        },
      });
    });

    expect(localStorage.getItem("multgame.playerName")).toBe("Pilot One");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("uses a default name when input is blank", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve([]),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () =>
          Promise.resolve({
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-2",
            lobbyId: "test",
            token: "token-2",
          }),
      });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <MemoryRouter>
        <MainMenu />
      </MemoryRouter>,
    );

    await screen.findByText("No posted results yet.");

    fireEvent.click(screen.getByRole("button", { name: "Play" }));

    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/game", {
        state: {
          match: {
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-2",
            lobbyId: "test",
            token: "token-2",
          },
        },
      });
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      expect.stringContaining("/api/matchmaking/join"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ playerName: "Pilot", region: "local" }),
      }),
    );
    expect(localStorage.getItem("multgame.playerName")).toBe("Pilot");
  });
});
