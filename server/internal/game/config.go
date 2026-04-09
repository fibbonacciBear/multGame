package game

import (
	"os"
	"time"
)

func defaultLobbyID() string {
	if value := os.Getenv("LOBBY_ID"); value != "" {
		return value
	}
	if podIP := os.Getenv("POD_IP"); podIP != "" {
		return "lobby-" + podIP
	}
	return "local-lobby"
}

func defaultPodIP() string {
	if value := os.Getenv("POD_IP"); value != "" {
		return value
	}
	return "127.0.0.1"
}

func registryHeartbeatTTL() time.Duration {
	return envDuration("REGISTRY_HEARTBEAT_TTL", 10*time.Second)
}
