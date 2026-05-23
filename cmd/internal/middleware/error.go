package middleware

import (
	"context"
	"net/http"

	"github.com/ansonallard/go_utils/openapi/ierr"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// ErrorHandlerMiddleware converts ierr types to HTTP status codes
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next() // Process request

		// Check if there were errors
		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err
			ctx := c.Request.Context()

			switch err.(type) {
			case *ierr.UnAuthorizedError:
				abortWithStatusResponse(ctx, http.StatusUnauthorized, err, c)
			case *ierr.NotFoundError:
				abortWithStatusResponse(ctx, http.StatusNotFound, err, c)
			case *ierr.ConflictError:
				abortWithStatusResponse(ctx, http.StatusConflict, err, c)
			default:
				abortWithStatusResponse(ctx, http.StatusInternalServerError, err, c)
			}
		}
	}
}

func abortWithStatusResponse(ctx context.Context, code int, err error, c *gin.Context) {
	log := zerolog.Ctx(ctx)
	log.Warn().Err(err).Int("status", code).Interface("request", c.Request).Msg("API Response Error")
	c.AbortWithStatusJSON(code, map[string]string{"message": err.Error()})
}
