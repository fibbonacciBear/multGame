package app

import "testing"

func TestBuildWSURLUsesRouterBaseURL(t *testing.T) {
	cfg := Config{WSRouterBaseURL: "wss://game.example.com/ws"}
	got := buildWSURL(cfg, "lobby-1", "token-1")
	want := "wss://game.example.com/ws/token-1"
	if got != want {
		t.Fatalf("buildWSURL() = %q, want %q", got, want)
	}
}

func TestBuildWSURLUsesDirectPhaseOneFallback(t *testing.T) {
	cfg := Config{PublicGameWSURL: "ws://localhost:8080/ws"}
	got := buildWSURL(cfg, "lobby-1", "token-1")
	want := "ws://localhost:8080/ws?lobby=lobby-1&token=token-1"
	if got != want {
		t.Fatalf("buildWSURL() = %q, want %q", got, want)
	}
}

func TestPickLobbyAssignmentExcludesDebugLobbiesForPlayers(t *testing.T) {
	server := &Server{}
	lobbies := []lobbyAssignment{
		{LobbyID: "debug", PodIP: "10.0.0.1", MatchKind: matchKindDebugBotSim},
		{LobbyID: "normal", PodIP: "10.0.0.2", MatchKind: matchKindNormal},
	}
	readyPods := map[string]registryRecord{
		"10.0.0.1": {State: registryStateReady, Port: "8080"},
		"10.0.0.2": {State: registryStateReady, Port: "8080"},
	}

	assignment, ok := server.pickLobbyAssignment(lobbies, readyPods, assignmentRequest{Mode: sessionModePlayer})
	if !ok {
		t.Fatalf("pickLobbyAssignment() ok = false, want true")
	}
	if assignment.LobbyID != "normal" {
		t.Fatalf("assignment.LobbyID = %q, want normal", assignment.LobbyID)
	}
}

func TestPickLobbyAssignmentAllowsSpectatorsIntoFullLobbyWithCapacity(t *testing.T) {
	server := &Server{}
	lobbies := []lobbyAssignment{{
		LobbyID:        "active",
		PodIP:          "10.0.0.1",
		Phase:          "active",
		MatchKind:      matchKindNormal,
		SpectatorCount: 1,
		MaxSpectators:  2,
	}}
	readyPods := map[string]registryRecord{
		"10.0.0.1": {State: registryStateFull, Port: "8080"},
	}

	assignment, ok := server.pickLobbyAssignment(lobbies, readyPods, assignmentRequest{Mode: sessionModeSpectator})
	if !ok {
		t.Fatalf("pickLobbyAssignment() ok = false, want true")
	}
	if assignment.LobbyID != "active" {
		t.Fatalf("assignment.LobbyID = %q, want active", assignment.LobbyID)
	}
}

func TestPickLobbyAssignmentFindsDebugSessionByID(t *testing.T) {
	server := &Server{}
	lobbies := []lobbyAssignment{
		{
			LobbyID:        "lobby-1",
			PodIP:          "10.0.0.1",
			MatchKind:      matchKindDebugBotSim,
			DebugSessionID: "debug-123",
			SpectatorCount: 0,
			MaxSpectators:  2,
		},
	}
	readyPods := map[string]registryRecord{
		"10.0.0.1": {State: registryStateReady, Port: "8080"},
	}

	assignment, ok := server.pickLobbyAssignment(lobbies, readyPods, assignmentRequest{
		Mode:           sessionModeDebugSimulation,
		LobbyID:        "lobby-1",
		DebugSessionID: "debug-123",
	})
	if !ok {
		t.Fatalf("pickLobbyAssignment() ok = false, want true")
	}
	if assignment.DebugSessionID != "debug-123" {
		t.Fatalf("assignment.DebugSessionID = %q, want debug-123", assignment.DebugSessionID)
	}
}
