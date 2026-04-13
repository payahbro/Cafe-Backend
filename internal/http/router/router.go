package router

import (
	"cafeTelkom/internal/config"
	"cafeTelkom/internal/http/handler"
	"cafeTelkom/internal/http/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func New(cfg config.Config, log *zap.Logger, dbPool *pgxpool.Pool, redisClient *redis.Client) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger(log))

	healthHandler := handler.NewHealthHandler(cfg, dbPool, redisClient)
	r.GET("/health", healthHandler.Get)

	v1 := r.Group("/api/v1")
	v1.GET("/health", healthHandler.Get)

	return r
}

