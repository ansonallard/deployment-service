package authz

import (
	"fmt"
	"net/http"

	"github.com/ansonallard/go_utils/openapi/ierr"
	"github.com/gin-gonic/gin"
)

const (
	apiKeyHeaderKey = "x-api-key"
)

type AuthZ interface {
	AuthMiddleware() gin.HandlerFunc
}

func NewAuthZ(apiKey string) AuthZ {
	if apiKey == "" {
		panic("apiKey not set")
	}
	return &authz{
		apiKey: apiKey,
	}
}

type authz struct {
	apiKey string
}

func (az *authz) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := c.Request.Header[http.CanonicalHeaderKey(apiKeyHeaderKey)]
		if !ok || len(apiKey) != 1 {
			c.Error(fmt.Errorf("%s not present", http.CanonicalHeaderKey(apiKeyHeaderKey)))
			c.Abort()
			return
		}
		if apiKey[0] != az.apiKey {
			c.Error(&ierr.UnAuthorizedError{})
			c.Abort()
			return
		}

		c.Next()
	}
}
