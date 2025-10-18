package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"

	"github.com/ansonallard/deployment-service/internal/compose"
	"github.com/ansonallard/deployment-service/internal/controllers"
	"github.com/ansonallard/deployment-service/internal/env"
	"github.com/ansonallard/deployment-service/internal/ierr"
	"github.com/ansonallard/deployment-service/internal/middleware/authz"
	"github.com/ansonallard/deployment-service/internal/middleware/openapi"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/releaser"
	"github.com/ansonallard/deployment-service/internal/repo"
	irequest "github.com/ansonallard/deployment-service/internal/request"
	"github.com/ansonallard/deployment-service/internal/service"
	"github.com/ansonallard/deployment-service/internal/version"
	"github.com/docker/docker/client"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config=types.cfg.yaml public/deployment-service.openapi.yaml

func main() {
	ctx := zeroLogConfiguration()
	log := zerolog.Ctx(ctx)

	if err := godotenv.Load(); err != nil {
		log.Fatal().Msg("could not load .env file")
	}

	log.Info().Msg("Loaded .env file")

	loader := openapi3.NewLoader()
	openAPISpec, err := loader.LoadFromFile(env.GetOpenAPIPath())
	if err != nil {
		log.Fatal().Err(err).Msg("Error loading OpenAPI spec")
	}

	log.Info().Str("OpenAPIPath", env.GetOpenAPIPath()).Msg("Loaded OpenAPI spec")

	// Validate the OpenAPI spec itself
	err = openAPISpec.Validate(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Error validating swagger spec")
	}

	log.Info().Str("OpenAPIPath", env.GetOpenAPIPath()).Msg("Validated OpenAPISpec")

	// Create router from OpenAPI spec
	router, err := gorillamux.NewRouter(openAPISpec)
	if err != nil {
		log.Fatal().Err(err).Msg("Error creating router")
	}

	authZMiddleware := authz.NewAuthZ(env.GetAPIKey())

	// Create Gin router
	ginMode := gin.DebugMode
	if !env.IsDevMode() {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)
	ginRouter := gin.New()
	ginRouter.Use(gin.Recovery())
	ginRouter.Use(openapi.ValidationMiddleware(router, authZMiddleware.AuthorizeCaller))

	deploymentServiceRepo, err := repo.NewDeploymentService(repo.DeploymentServieConfig{
		ServiceFilPath: env.GetSerivceFilePath(),
		SSHKeyPath:     env.GetSSHKeyPath(),
		GitClient:      repo.NewGitClient(),
		GitRepoOrigin:  env.GetGitRepoOirign(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate deployment service")
	}

	serviceChannel := make(service.ServiceChannel, 100)

	deploymentService, err := service.NewDeploymentService(service.DeploymentServiceConfig{
		Repo:                 deploymentServiceRepo,
		BackgroundJobChannel: serviceChannel,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate deployment service")
	}
	deploymentServiceController, err := controllers.NewDeploymentServiceController(controllers.DeploymentServiceControllerConfig{
		Service: deploymentService,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate deployment service controller")
	}

	dockerClientOpts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	if env.GetDockerHome() != "" {
		dockerClientOpts = append(dockerClientOpts, client.WithHost(env.GetDockerHome()))
	}

	dockerClient, err := client.NewClientWithOpts(
		dockerClientOpts...,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("could not instantiate docker client")
	}
	defer dockerClient.Close()

	dockerReleaser := releaser.NewDockerReleaser(releaser.DockerReleaserConfig{
		DockerClient: dockerClient,
	})

	dockerCompose := compose.New(compose.Config{
		CLI: compose.V1,
	})

	envWriter := service.NewEnvFileWriter()

	versioner := version.NewVersioner()
	backgroundProcessor, err := service.NewBackgroundProcessor(service.BackgroundProcessorConfig{
		Versioner:     versioner,
		SSHKeyPath:    env.GetSSHKeyPath(),
		GitRepoOrigin: env.GetGitRepoOirign(),
		CiCommitAuthor: &service.CiCommitAuthor{
			Name:  env.GetCICommitAuthorName(),
			Email: env.GetCICommitAuthorEmail(),
		},
		DockerReleaser: dockerReleaser,
		Compose:        dockerCompose,
		EnvWriter:      envWriter,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate background processor")
	}

	// Validate that top level struct contains all required OpenAPI operation IDs
	if err = openapi.ValidateStructAndOpenAPI(openAPISpec, deploymentServiceController); err != nil {
		log.Fatal().Err(err).Msg("Failed to ValidateStructAndOpenAPI")
	}

	go func() {
		log.Info().Msg("Waiting on messages from serviceChannel to start the background processing")
		for channelEntry := range serviceChannel {
			go func(service *model.Service) {
				log.Info().Interface("service", service).Str("serviceName", service.Name).
					Msg("New Service created, starting background processing")
				if err := backgroundProcessor.ProcessService(ctx, service); err != nil {
					log.Error().Err(err).Interface("service", service).Str("serviceName", service.Name).Msg("error when processing service")
				}
			}(channelEntry)
		}
	}()

	if err := deploymentService.CollectExistingServicesForBackgroundProcessing(ctx); err != nil {
		log.Fatal().Err(err).Msg("Errored collecting existing services")
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

		topLevelStructReflected := reflect.ValueOf(deploymentServiceController)
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
			err, ok := result[0].Interface().(error)
			if ok {
				errorHandler(err, c)
				return
			}
		case 2:
			methodResult = result[0].Interface()
			err, ok := result[1].Interface().(error)
			if ok {
				errorHandler(err, c)
				return
			}
		}

		c.JSON(firstSuccessfulResponseCode, methodResult)
	})

	port := env.GetPort()
	log.Info().Str("port", port).Msgf("Server starting on :%s", port)
	ginRouter.Run(fmt.Sprintf(":%s", port))
}

func errorHandler(err error, c *gin.Context) {
	switch err.(type) {
	case *ierr.UnAuthorizedError:
		abortWithStatusResponse(http.StatusUnauthorized, err, c)
	case *ierr.NotFoundError:
		abortWithStatusResponse(http.StatusNotFound, err, c)
	case *ierr.ConflictError:
		abortWithStatusResponse(http.StatusConflict, err, c)
	default:
		abortWithStatusResponse(http.StatusInternalServerError, err, c)
	}
}

func abortWithStatusResponse(code int, err error, c *gin.Context) {
	c.AbortWithStatusJSON(code, map[string]string{"message": err.Error()})
}

func zeroLogConfiguration() context.Context {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	logger := zerolog.New(os.Stdout)
	ctx := context.Background()

	// Attach the Logger to the context.Context
	ctx = logger.WithContext(ctx)
	return ctx
}
