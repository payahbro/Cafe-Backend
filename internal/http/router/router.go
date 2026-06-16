package router

import (
	"cafeTelkom/internal/cache"
	"cafeTelkom/internal/config"
	"cafeTelkom/internal/http/handler"
	"cafeTelkom/internal/http/middleware"
	midtransintegration "cafeTelkom/internal/integrations/midtrans"
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
	productTxRunner := service.NewProductTxRunner(dbPool, repo)
	var productCache service.ProductCacheInvalidator
	if redisClient != nil {
		productCache = cache.NewProductCache(redisClient)
	}
	productService := service.NewProductService(repo, productTxRunner, productCache)
	cartTxRunner := service.NewCartTxRunner(dbPool, repo)
	cartService := service.NewCartService(repo, cartTxRunner)
	orderTxRunner := service.NewOrderTxRunner(dbPool, repo)
	orderService := service.NewOrderService(repo, orderTxRunner, nil)
	paymentTxRunner := service.NewPaymentTxRunner(dbPool, repo)
	midtransClient := midtransintegration.NewClient(
		cfg.Midtrans.ServerKey,
		cfg.Midtrans.SnapBaseURL,
		cfg.Midtrans.WebhookURL,
		nil,
	)
	paymentService := service.NewPaymentService(
		repo,
		paymentTxRunner,
		service.NewMidtransSnapClient(midtransClient),
		orderService,
		cartService,
		service.PaymentServiceOptions{
			ServerKey:  cfg.Midtrans.ServerKey,
			WebhookURL: cfg.Midtrans.WebhookURL,
		},
	)

	// Handler/HTTP
	healthHandler := handler.NewHealthHandler(cfg, dbPool, redisClient)
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(userService, cfg.Supabase.URL)
	productHandler := handler.NewProductHandler(productService, cfg.Supabase.URL)
	cartHandler := handler.NewCartHandler(cartService, cfg.Internal.APIKey)
	orderHandler := handler.NewOrderHandler(
		orderService,
		handler.WithOrderCartClearer(cartService),
		handler.WithOrderInternalAPIKey(cfg.Internal.APIKey),
	)
	paymentHandler := handler.NewPaymentHandler(paymentService, handler.WithPaymentLogger(log))
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

	// product
	v1Products := v1.Group("/products")
	v1Products.GET("", productHandler.ListProducts)
	v1Products.POST("", middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRoleADMIN), productHandler.CreateProduct)
	v1Products.GET("/:id", productHandler.GetProduct)
	v1Products.PUT("/:id", middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRoleADMIN), productHandler.UpdateProduct)
	v1Products.PATCH("/:id/status", middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRolePEGAWAI, repository.UserRoleADMIN), productHandler.UpdateProductStatus)
	v1Products.DELETE("/:id", middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRoleADMIN), productHandler.DeleteProduct)
	v1Products.PATCH("/:id/restore", middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRoleADMIN), productHandler.RestoreProduct)

	// cart
	v1Cart := v1.Group("/cart")
	v1Cart.Use(middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRoleCUSTOMER))
	v1Cart.GET("", cartHandler.GetCart)
	v1Cart.POST("/items", cartHandler.AddItem)
	v1Cart.DELETE("/items", cartHandler.ClearItems)
	v1Cart.PATCH("/items/:item_id", cartHandler.UpdateItem)
	v1Cart.DELETE("/items/:item_id", cartHandler.DeleteItem)

	// order
	v1Orders := v1.Group("/orders")
	v1Orders.Use(middleware.AuthRequired(jwtVerifier, repo))
	v1Orders.POST("/checkout", middleware.RequireRoles(repository.UserRoleCUSTOMER), orderHandler.Checkout)
	v1Orders.GET("", middleware.RequireRoles(repository.UserRoleCUSTOMER, repository.UserRolePEGAWAI, repository.UserRoleADMIN), orderHandler.ListOrders)
	v1Orders.PATCH("/:order_id/cancel", middleware.RequireRoles(repository.UserRoleCUSTOMER, repository.UserRoleADMIN), orderHandler.CancelOrder)
	v1Orders.PATCH("/:order_id/status", middleware.RequireRoles(repository.UserRolePEGAWAI, repository.UserRoleADMIN), orderHandler.UpdateStatus)
	v1Orders.GET("/:order_id", middleware.RequireRoles(repository.UserRoleCUSTOMER, repository.UserRolePEGAWAI, repository.UserRoleADMIN), orderHandler.GetOrder)

	// payment
	v1Payments := v1.Group("/payments")
	v1Payments.POST("/webhook", paymentHandler.Webhook)
	v1Payments.POST("/initiate", middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRoleCUSTOMER), paymentHandler.Initiate)
	v1Payments.GET("/order/:order_id", middleware.AuthRequired(jwtVerifier, repo), middleware.RequireRoles(repository.UserRoleCUSTOMER, repository.UserRoleADMIN), paymentHandler.GetByOrder)

	// internal
	v1Internal := v1.Group("/internal")
	v1Internal.DELETE("/cart/items", cartHandler.ClearInternalItems)
	v1Internal.PATCH("/orders/:order_id/status", orderHandler.InternalUpdateStatus)

	return r
}
