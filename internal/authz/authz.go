package authz

import (
	"context"
	"fmt"

	"github.com/ansonallard/deployment-service/internal/ierr"
	"github.com/getkin/kin-openapi/openapi3filter"
)

type AuthZ interface {
	AuthorizeCaller(ctx context.Context, ai *openapi3filter.AuthenticationInput) error
}

type AuthorizationFunction = func(ctx context.Context, ai *openapi3filter.AuthenticationInput) error

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

func (az *authz) AuthorizeCaller(ctx context.Context, ai *openapi3filter.AuthenticationInput) error {
	apiKey, ok := ai.RequestValidationInput.Request.Header[ai.SecurityScheme.Name]
	if !ok || len(apiKey) != 1 {
		return fmt.Errorf("%s not present", ai.SecurityScheme.Name)
	}
	if apiKey[0] != az.apiKey {
		return &ierr.UnAuthorizedError{}
	}
	return nil
}
