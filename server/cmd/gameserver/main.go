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

	"multgame/server/internal/game"
)

func main() {
	logger := log.New(os.Stdout, "[game-server] ", log.LstdFlags|log.Lmicroseconds)
	cfg := game.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := game.NewServer(cfg, logger)
	srv.Start(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.HandleHealthz)
	mux.HandleFunc("/readyz", srv.HandleReadyz)
	mux.HandleFunc("/metrics", srv.HandleMetrics)
	mux.HandleFunc("/ws", srv.HandleWS)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		drainCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownDrainTimeout)
		defer cancel()

		srv.BeginDrain("server shutting down")
		srv.WaitForDrain(drainCtx)
		_ = srv.CleanupRegistry(drainCtx)
		srv.CloseAllConnections()

		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = httpServer.Shutdown(closeCtx)
	}()

	logger.Printf("listening on :%s", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("http server failed: %v", err)
	}
}
