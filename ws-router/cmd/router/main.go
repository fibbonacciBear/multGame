package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"multgame/ws-router/internal/router"
)

func main() {
	logger := log.New(os.Stdout, "[ws-router] ", log.LstdFlags|log.Lmicroseconds)
	cfg := router.LoadConfig()
	handler := router.New(cfg, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", handler.HandleHealthz)
	mux.HandleFunc("/ws/", handler.HandleWS)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	go func() {
		<-signals
		_ = server.Close()
	}()

	logger.Printf("listening on :%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("router failed: %v", err)
	}
}
