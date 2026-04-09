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
