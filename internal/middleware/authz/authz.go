package authz

import (
	"context"
	"fmt"

	"github.com/ansonallard/go_utils/openapi/ierr"
	rauthz "github.com/ansonallard/go_utils/openapi/middleware/authz"

	"github.com/getkin/kin-openapi/openapi3filter"
)

func NewAuthZ(apiKey string) rauthz.AuthZ {
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
