package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"multgame/api/internal/app"
)

func main() {
	logger := log.New(os.Stdout, "[api-server] ", log.LstdFlags|log.Lmicroseconds)
	cfg := app.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := prometheus.DefaultRegisterer
	app.RegisterMetrics(reg)

	server, err := app.NewServer(ctx, cfg, logger)
	if err != nil {
		logger.Fatalf("failed to create api server: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.HandleHealthz)
	mux.HandleFunc("/api/matchmaking/config", server.HandleMatchmakingConfig)
	mux.HandleFunc("/api/matchmaking/join", server.HandleJoin)
	mux.HandleFunc("/api/matchmaking/spectate", server.HandleSpectate)
	mux.HandleFunc("/api/matchmaking/debug-simulate", server.HandleDebugSimulate)
	mux.HandleFunc("/api/leaderboard", server.HandleLeaderboard)
	mux.HandleFunc("/api/leaderboard/report", server.HandleLeaderboardReport)
	mux.HandleFunc("/api/match-metrics/report", server.HandleMatchMetricsReport)
	mux.Handle("/metrics", promhttp.Handler())

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.WithMetrics(server.WithCORS(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Printf("listening on :%s", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("http server failed: %v", err)
	}
}
