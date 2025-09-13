package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"reflect"

	"github.com/ansonallard/deployment-service/internal/controllers"
	"github.com/ansonallard/deployment-service/internal/env"
	"github.com/ansonallard/deployment-service/internal/openapi"
	irequest "github.com/ansonallard/deployment-service/internal/request"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config=types.cfg.yaml public/deployment-service.openapi.yaml

func main() {
	ctx := context.Background()

	if err := godotenv.Load(); err != nil {
		panic("could not load .env file")
	}

	loader := openapi3.NewLoader()
	openAPISpec, err := loader.LoadFromFile(env.GetOpenAPIPath())
	if err != nil {
		log.Fatalf("Error loading swagger spec: %v", err)
	}

	// Validate the OpenAPI spec itself
	err = openAPISpec.Validate(ctx)
	if err != nil {
		log.Fatalf("Error validating swagger spec: %v", err)
	}

	// Create router from OpenAPI spec
	router, err := gorillamux.NewRouter(openAPISpec)
	if err != nil {
		log.Fatalf("Error creating router: %v", err)
	}

	// Create Gin router
	ginMode := gin.DebugMode
	if !env.IsDevMode() {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)
	ginRouter := gin.New()
	ginRouter.Use(gin.Recovery())
	ginRouter.Use(openapi.ValidationMiddleware(router))

	topLevelStruct := controllers.NewDeploymentControllers()

	// Validate that top level struct contains all required OpenAPI operation IDs
	if err = openapi.ValidateStructAndOpenAPI(openAPISpec, topLevelStruct); err != nil {
		log.Panic(err)
	}

	ginRouter.Any("/*path", func(c *gin.Context) {
		route, pathParams, err := router.FindRoute(c.Request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Error finding route: %v", err)})
			return
		}

		firstSuccessfulResponseCode, err := openapi.GetFirstSuccessfulStatusCode(route.Operation.Responses)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		topLevelStructReflected := reflect.ValueOf(topLevelStruct)
		method := topLevelStructReflected.MethodByName(openapi.ConvertOperationIdToPascalCase(route.Operation.OperationID))

		iRequest := irequest.NewRequest(&irequest.RequestConfig{
			QueryParams: c.Request.URL.Query(),
			Headers:     c.Request.Header,
			PathParams:  pathParams,
			RequestBody: c.Request.Body,
		})

		values := []reflect.Value{reflect.ValueOf(context.Background()), reflect.ValueOf(iRequest)}
		result := method.Call(values)

		// All top level methods must either return an error
		// or a successful response and error
		var methodResult any
		switch len(result) {
		case 1:
			_, ok := result[0].Interface().(error)
			if ok {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
		case 2:
			methodResult = result[0].Interface()
			_, ok := result[1].Interface().(error)
			if ok {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
		}

		c.JSON(firstSuccessfulResponseCode, methodResult)
	})

	port := env.GetPort()
	log.Printf("Server starting on :%s", port)
	ginRouter.Run(fmt.Sprintf(":%s", port))
}
