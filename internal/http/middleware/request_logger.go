package middleware

import (
	"time"

	"cafeTelkom/internal/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		log.Info("http_request",
			logger.String("method", c.Request.Method),
			logger.String("path", c.FullPath()),
			logger.Int("status", c.Writer.Status()),
			logger.Duration("latency", time.Since(start)),
		)
	}
}

