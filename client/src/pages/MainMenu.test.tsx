import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it("joins a match from the main menu", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () =>
          Promise.resolve({
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-1",
            lobbyId: "test",
            token: "token-1",
            sessionMode: "player",
          }),
      });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <MemoryRouter>
        <MainMenu />
      </MemoryRouter>,
    );

    expect(screen.getByText("somewhere between the stars")).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText("your name"), {
      target: { value: "Pilot One" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Drift" }));

    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/game", {
        state: {
          match: {
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-1",
            lobbyId: "test",
            token: "token-1",
            sessionMode: "player",
          },
          refresh: {
            sessionMode: "player",
            region: "local",
            playerName: "Pilot One",
          },
        },
      });
    });

    expect(localStorage.getItem("multgame.playerName")).toBe("Pilot One");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("shows region selector options from choose region button", () => {
    render(
      <MemoryRouter>
        <MainMenu />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Region: Local" }));
    expect(screen.getByRole("menu", { name: "Region options" })).toBeTruthy();
    expect(screen.getByRole("menuitemradio", { name: "Local" })).toBeTruthy();
  });

  it("hides observer controls when frontend spectator mode is disabled", () => {
    render(
      <MemoryRouter>
        <MainMenu />
      </MemoryRouter>,
    );

    expect(screen.queryByText("Observer Tools")).toBeNull();
    expect(screen.queryByRole("button", { name: "Spectate" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Debug Sim" })).toBeNull();
  });

  it("uses a default name when input is blank", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () =>
          Promise.resolve({
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-2",
            lobbyId: "test",
            token: "token-2",
            sessionMode: "player",
          }),
      });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <MemoryRouter>
        <MainMenu />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Drift" }));

    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/game", {
        state: {
          match: {
            wsUrl: "ws://localhost:8080/ws?lobby=test&token=token-2",
            lobbyId: "test",
            token: "token-2",
            sessionMode: "player",
          },
          refresh: {
            sessionMode: "player",
            region: "local",
            playerName: "Pilot",
          },
        },
      });
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      expect.stringContaining("/api/matchmaking/join"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ playerName: "Pilot", region: "local" }),
      }),
    );
    expect(localStorage.getItem("multgame.playerName")).toBe("Pilot");
  });
});
