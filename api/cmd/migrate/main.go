package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"multgame/api/internal/app"
)

func main() {
	logger := log.New(os.Stdout, "[api-migrate] ", log.LstdFlags|log.Lmicroseconds)
	cfg := app.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.RunMigrations(ctx, cfg, logger); err != nil {
		logger.Fatalf("migrations failed: %v", err)
	}
	logger.Printf("migrations complete")
}
