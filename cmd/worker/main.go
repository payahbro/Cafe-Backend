package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"cafeTelkom/internal/config"
	"cafeTelkom/internal/db"
	firebaseintegration "cafeTelkom/internal/integrations/firebase"
	"cafeTelkom/internal/logger"
	"cafeTelkom/internal/outbox"
	"cafeTelkom/internal/repository"

	"go.uber.org/zap"
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

	if !cfg.FCM.Enabled {
		log.Info("worker started", logger.String("name", "outbox-scheduler-worker"), logger.String("fcm_enabled", "false"))
		<-ctx.Done()
		log.Info("worker stopped")
		return
	}

	processor, cleanup, err := newOutboxProcessor(ctx, cfg)
	if err != nil {
		log.Fatal("failed to initialize outbox processor", logger.Error(err))
	}
	defer cleanup()

	pollSeconds := cfg.FCM.PollSeconds
	if pollSeconds <= 0 {
		pollSeconds = 30
	}
	ticker := time.NewTicker(time.Duration(pollSeconds) * time.Second)
	defer ticker.Stop()

	log.Info("worker started", logger.String("name", "outbox-scheduler-worker"), logger.String("fcm_enabled", "true"))
	processOutbox(ctx, log, processor, cfg.FCM.BatchSize)
	for {
		select {
		case <-ctx.Done():
			log.Info("worker stopped")
			return
		case <-ticker.C:
			processOutbox(ctx, log, processor, cfg.FCM.BatchSize)
		}
	}
}

func newOutboxProcessor(ctx context.Context, cfg config.Config) (*outbox.Processor, func(), error) {
	if cfg.FCM.ProjectID == "" {
		return nil, nil, fmt.Errorf("FCM_PROJECT_ID cannot be empty when FCM_ENABLED=true")
	}
	if cfg.FCM.CredentialFile == "" {
		return nil, nil, fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS cannot be empty when FCM_ENABLED=true")
	}

	dsn := cfg.DatabaseURL()
	if dsn == "" {
		return nil, nil, fmt.Errorf("database connection is required for outbox worker")
	}

	dbPool, err := db.NewPostgresPool(ctx, dsn, db.PoolOptions{
		MaxConns:        cfg.Database.MaxOpenConns,
		MinConns:        cfg.Database.MinIdleConns,
		MaxConnLifetime: time.Duration(cfg.Database.ConnMaxLifetimeM) * time.Minute,
		MaxConnIdleTime: time.Duration(cfg.Database.ConnMaxIdleTimeM) * time.Minute,
		HealthCheck:     time.Duration(cfg.Database.HealthcheckSecond) * time.Second,
		PingTimeout:     2 * time.Second,
	})
	if err != nil {
		return nil, nil, err
	}

	fcmClient, err := firebaseintegration.NewFCMClient(
		cfg.FCM.ProjectID,
		cfg.FCM.TopicNewProducts,
		cfg.FCM.CredentialFile,
		nil,
	)
	if err != nil {
		dbPool.Close()
		return nil, nil, err
	}

	repo := repository.New(dbPool)
	processor := outbox.NewProcessor(repo, fcmClient, outbox.ProcessorOptions{
		MaxRetries: cfg.FCM.MaxRetries,
		RetryDelay: time.Duration(cfg.FCM.RetryDelaySeconds) * time.Second,
	})
	return processor, dbPool.Close, nil
}

func processOutbox(ctx context.Context, log *zap.Logger, processor *outbox.Processor, batchSize int32) {
	processed, err := processor.ProcessBatch(ctx, batchSize)
	if err != nil {
		log.Error("outbox processing failed", logger.Error(err))
		return
	}
	if processed > 0 {
		log.Info("outbox batch processed", logger.Int("processed", processed))
	}
}
