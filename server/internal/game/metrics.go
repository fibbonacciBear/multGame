package game

import "github.com/prometheus/client_golang/prometheus"

var (
	ActivePlayers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "active_players_per_pod",
		Help: "Connected human players in this game-server pod.",
	})

	TickDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "game_tick_duration_seconds",
		Help:    "Time spent executing a single game tick.",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05},
	})

	SnapshotBroadcastDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "game_snapshot_broadcast_duration_seconds",
		Help:    "Time spent building and sending snapshot broadcasts.",
		Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05},
	})

	WSMessagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "game_ws_messages_received_total",
		Help: "Total WebSocket input messages received from clients.",
	})

	WSConnectionsOpened = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "game_ws_connections_opened_total",
		Help: "Total WebSocket connections opened.",
	})

	WSConnectionsClosed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "game_ws_connections_closed_total",
		Help: "Total WebSocket connections closed.",
	})

	PlayerKills = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "game_player_kills_total",
		Help: "Total player kills (includes bot-on-bot).",
	})

	CrashContacts = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "game_crash_contacts_total",
		Help: "Total crash contacts resolved by the authoritative combat loop.",
	})

	CrashLethalOutcomes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "game_crash_lethal_outcomes_total",
		Help: "Total player deaths attributed to crash damage.",
	})

	PlayerMass = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "game_player_mass",
		Help:    "Observed alive-player mass values during game ticks.",
		Buckets: []float64{10, 20, 40, 60, 80, 120, 180, 250},
	})

	MatchesCompleted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "game_matches_completed_total",
		Help: "Total matches that reached their time limit.",
	})

	LobbyPlayerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "game_lobby_player_count",
		Help: "Total players in lobby (humans + bots).",
	})

	SpectatorConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "game_spectator_connections",
		Help: "Connected spectator/debug observer sessions.",
	})
)

func RegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		ActivePlayers,
		TickDuration,
		SnapshotBroadcastDuration,
		WSMessagesReceived,
		WSConnectionsOpened,
		WSConnectionsClosed,
		PlayerKills,
		CrashContacts,
		CrashLethalOutcomes,
		PlayerMass,
		MatchesCompleted,
		LobbyPlayerCount,
		SpectatorConnections,
	)
}
