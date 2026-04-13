package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"cafeTelkom/internal/config"
	"cafeTelkom/internal/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := logger.New(cfg.Log.Level)
	if err != nil {
		panic(err)
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Info("worker started", logger.String("name", "outbox-scheduler-worker"))
	for {
		select {
		case <-ctx.Done():
			log.Info("worker stopped")
			return
		case <-ticker.C:
			log.Debug("worker heartbeat")
		}
	}
}


