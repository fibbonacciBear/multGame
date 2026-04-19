package game

import "testing"

func TestLobbyRegistryFieldsStringifyCustomEnums(t *testing.T) {
	cfg := Config{
		PodIP:        "10.0.0.5",
		Port:         "8080",
		TickRate:     60,
		SnapshotRate: 20,
		MaxPlayers:   10,
	}
	record := registryRecord{
		Phase:           phaseIntermission,
		MatchOver:       true,
		MatchKind:       matchKindDebugBotSim,
		ConnectedHumans: 0,
		SpectatorCount:  2,
		MaxSpectators:   8,
		DebugSessionID:  "debug-123",
	}

	fields := lobbyRegistryFields(cfg, record)

	if phase, ok := fields["phase"].(string); !ok || phase != string(phaseIntermission) {
		t.Fatalf("phase field = %#v, want string %q", fields["phase"], phaseIntermission)
	}
	if matchKind, ok := fields["match_kind"].(string); !ok || matchKind != string(matchKindDebugBotSim) {
		t.Fatalf("match_kind field = %#v, want string %q", fields["match_kind"], matchKindDebugBotSim)
	}
	if debugSessionID, ok := fields["debug_session_id"].(string); !ok || debugSessionID != "debug-123" {
		t.Fatalf("debug_session_id field = %#v, want %q", fields["debug_session_id"], "debug-123")
	}
}
