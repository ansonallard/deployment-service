package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func InjectLogger(log *zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqCtx := log.WithContext(c.Request.Context())
		c.Request = c.Request.WithContext(reqCtx)
		c.Next()
	}
}

func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := zerolog.Ctx(c.Request.Context())
		startTime := time.Now().UTC()

		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Interface("requestBody", c.Request.Body).
			Msg("API Request")

		c.Next()

		log.Info().
			Int("status", c.Writer.Status()).
			TimeDiff("latency", time.Now().UTC(), startTime).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Msg("API Response")
	}
}
