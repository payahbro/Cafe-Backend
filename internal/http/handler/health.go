package handler

import (
	"context"
	"net/http"
	"time"

	"cafeTelkom/internal/config"
	"cafeTelkom/internal/http/dto"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	cfg   config.Config
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewHealthHandler(cfg config.Config, db *pgxpool.Pool, redis *redis.Client) *HealthHandler {
	return &HealthHandler{cfg: cfg, db: db, redis: redis}
}

func (h *HealthHandler) Get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 1500*time.Millisecond)
	defer cancel()

	services := map[string]string{
		"api":      "up",
		"postgres": h.checkPostgres(ctx),
		"redis":    h.checkRedis(ctx),
	}

	overall := "ok"
	statusCode := http.StatusOK
	for _, state := range services {
		if state == "down" {
			overall = "degraded"
			statusCode = http.StatusServiceUnavailable
			break
		}
	}

	data := gin.H{
		"name":     h.cfg.App.Name,
		"env":      h.cfg.App.Env,
		"version":  h.cfg.App.Version,
		"status":   overall,
		"services": services,
		"time":     time.Now().UTC().Format(time.RFC3339),
	}

	dto.WriteSuccess(c, statusCode, data, "Service health checked")
}

func (h *HealthHandler) checkPostgres(ctx context.Context) string {
	if h.db == nil {
		return "disabled"
	}
	if err := h.db.Ping(ctx); err != nil {
		return "down"
	}
	return "up"
}

func (h *HealthHandler) checkRedis(ctx context.Context) string {
	if h.redis == nil {
		return "disabled"
	}
	if err := h.redis.Ping(ctx).Err(); err != nil {
		return "down"
	}
	return "up"
}


