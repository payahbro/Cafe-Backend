package router

import (
	"cafeTelkom/internal/config"
	"cafeTelkom/internal/http/handler"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/integrations/supabase"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

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

	// Repository
	repo := repository.New(dbPool)

	// Service
	authService := service.NewAuthService(
		supabase.NewAuthClient(cfg.Supabase.URL, cfg.Supabase.AnonKey),
		repo,
	)
	userService := service.NewUserService(repo)

	// Handler/HTTP
	healthHandler := handler.NewHealthHandler(cfg, dbPool, redisClient)
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(userService, cfg.Supabase.URL)
	jwtVerifier := supabase.NewJWTVerifier(cfg.Supabase.URL)

	r.GET("/health", healthHandler.Get)

	v1 := r.Group("/api/v1")
	v1.GET("/health", healthHandler.Get)

	// auth
	v1Auth := v1.Group("/auth")
	v1Auth.POST("/register", authHandler.Register)

	// user
	v1Users := v1.Group("/users")
	v1Users.Use(middleware.AuthRequired(jwtVerifier, repo))
	v1Users.GET("/profile", userHandler.GetProfile)
	v1Users.PATCH("/profile", userHandler.UpdateProfile)

	return r
}
