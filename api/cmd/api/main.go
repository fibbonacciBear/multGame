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

	"multgame/api/internal/app"
)

func main() {
	logger := log.New(os.Stdout, "[api-server] ", log.LstdFlags|log.Lmicroseconds)
	cfg := app.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server, err := app.NewServer(ctx, cfg, logger)
	if err != nil {
		logger.Fatalf("failed to create api server: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.HandleHealthz)
	mux.HandleFunc("/api/matchmaking/join", server.HandleJoin)
	mux.HandleFunc("/api/leaderboard", server.HandleLeaderboard)
	mux.HandleFunc("/api/leaderboard/report", server.HandleLeaderboardReport)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.WithCORS(mux),
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
