package openapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"strings"

	"github.com/ansonallard/deployment-service/internal/ierr"
	"github.com/ansonallard/deployment-service/internal/middleware/authz"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/gin-gonic/gin"
)

type responseWriter struct {
	gin.ResponseWriter
	body []byte
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return w.ResponseWriter.Write(b)
}

func ValidationMiddleware(router routers.Router, authZFunction authz.AuthorizationFunction) gin.HandlerFunc {
	return func(c *gin.Context) {
		route, pathParams, err := router.FindRoute(c.Request)
		switch err {
		case nil:
			// Do nothing
		case routers.ErrMethodNotAllowed:
			c.JSON(http.StatusMethodNotAllowed, nil)
			c.Abort()
			return
		case routers.ErrPathNotFound:
			c.JSON(http.StatusNotFound, nil)
			c.Abort()
			return
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error finding route: %v", err)})
			return
		}

		ctx := context.Background()

		// Validate Request
		requestValidationInput := &openapi3filter.RequestValidationInput{
			Request:    c.Request,
			PathParams: pathParams,
			Route:      route,
			Options: &openapi3filter.Options{
				MultiError:         true,
				AuthenticationFunc: authZFunction,
			},
		}

		if err := openapi3filter.ValidateRequest(ctx, requestValidationInput); err != nil {

			switch incomingError := err.(type) {
			case openapi3.MultiError:
				for _, e := range incomingError {
					switch incomingError1 := e.(type) {
					case *openapi3filter.SecurityRequirementsError:
						for _, e1 := range incomingError1.Errors {
							if _, ok := e1.(*ierr.UnAuthorizedError); ok {
								c.JSON(http.StatusUnauthorized, gin.H{"error": e1.Error()})
								c.Abort()
								return
							}
						}
					}
				}
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Error validating request: %v", err)})
			c.Abort()
			return
		}

		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:           []byte{},
		}
		c.Writer = writer

		// Process the request
		c.Next()

		// Validate Response
		responseValidationInput := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: requestValidationInput,
			Status:                 writer.Status(),
			Header:                 writer.Header(),
			Body:                   io.NopCloser(bytes.NewReader(writer.body)),
			Options: &openapi3filter.Options{
				MultiError: true,
			},
		}

		if err := openapi3filter.ValidateResponse(ctx, responseValidationInput); err != nil {
			log.Printf("Error validating response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error validating response: %v", err)})
			return
		}
	}
}

func ValidateStructAndOpenAPI[T any](openApiSpec *openapi3.T, topLevelStruct T) error {
	topLevelStructReflected := reflect.ValueOf(topLevelStruct)
	structName := reflect.TypeOf(topLevelStruct).Elem().Name()

	operationIdsMap := collectAllOperationIds(openApiSpec)
	methodErrors := []error{}

	for operationId := range *operationIdsMap {
		callableMethod := topLevelStructReflected.MethodByName(operationId)
		if !callableMethod.IsValid() {
			methodErrors = append(methodErrors, fmt.Errorf("struct of type %s does not contain required method %s", structName, operationId))
		}
	}

	if len(methodErrors) > 0 {
		return errors.Join(methodErrors...)
	}
	return nil
}

func collectAllOperationIds(openApiSpec *openapi3.T) *map[string]struct{} {
	operationIds := map[string]struct{}{}
	for _, path := range openApiSpec.Paths.Map() {
		operationsMap := path.Operations()
		for _, operation := range operationsMap {
			sanitizedOperationId := ConvertOperationIdToPascalCase(operation.OperationID)
			operationIds[sanitizedOperationId] = struct{}{}
		}
	}
	return &operationIds
}

func ConvertOperationIdToPascalCase(operationId string) string {
	if len(operationId) == 0 {
		return operationId
	}
	return strings.ToUpper(operationId[:1]) + operationId[1:]
}
