package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

func ZerologTraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		traceID := span.SpanContext().TraceID().String()
		spanID := span.SpanContext().SpanID().String()

		// Derive a child logger with trace fields, re-attach to context
		log := zerolog.Ctx(c.Request.Context()).With().
			Str("traceID", traceID).
			Str("spanID", spanID).
			Logger()
		ctx := log.WithContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
