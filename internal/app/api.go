package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cafeTelkom/internal/cache"
	"cafeTelkom/internal/config"
	"cafeTelkom/internal/db"
	"cafeTelkom/internal/http/router"
	"cafeTelkom/internal/logger"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func RunAPI(parent context.Context, stdout io.Writer, stderr io.Writer) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(cfg.Log.Level)
	if err != nil {
		fmt.Fprintf(stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var dbPool *pgxpool.Pool
	dsn := cfg.DatabaseURL()
	if dsn != "" {
		dbPool, err = db.NewPostgresPool(ctx, dsn, db.PoolOptions{
			MaxConns:        cfg.Database.MaxOpenConns,
			MinConns:        cfg.Database.MinIdleConns,
			MaxConnLifetime: time.Duration(cfg.Database.ConnMaxLifetimeM) * time.Minute,
			MaxConnIdleTime: time.Duration(cfg.Database.ConnMaxIdleTimeM) * time.Minute,
			HealthCheck:     time.Duration(cfg.Database.HealthcheckSecond) * time.Second,
			PingTimeout:     2 * time.Second,
		})
		if err != nil {
			if cfg.Database.Required {
				log.Fatal("postgres initialization failed", logger.Error(err))
			}
			log.Warn("postgres unavailable, continuing without db connection", logger.Error(err))
		}
	}
	if dbPool != nil {
		defer dbPool.Close()
	}

	var redisClient *redis.Client
	if cfg.Redis.Addr != "" {
		redisClient, err = cache.NewRedisClient(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			if cfg.Redis.Required {
				log.Fatal("redis initialization failed", logger.Error(err))
			}
			log.Warn("redis unavailable, continuing without cache connection", logger.Error(err))
		}
	}
	if redisClient != nil {
		defer redisClient.Close()
	}

	engine := router.New(cfg, log, dbPool, redisClient)
	server := &http.Server{
		Addr:              ":" + cfg.HTTP.Port,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("api server started", logger.String("addr", server.Addr), logger.String("env", cfg.App.Env))
		if serveErr := server.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Fatal("http server crashed", logger.Error(serveErr))
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.App.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", logger.Error(err))
		return
	}

	log.Info("api server stopped")
}


