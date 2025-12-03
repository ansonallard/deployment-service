package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"reflect"
	"time"

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
	backgroundprocessor "github.com/ansonallard/deployment-service/internal/service/background_processor"
	"github.com/ansonallard/deployment-service/internal/version"
	"github.com/docker/docker/client"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config=types.cfg.yaml public/deployment-service.openapi.yaml

const (
	defaultIPv4OpenAddress = "0.0.0.0"
	defaultLogFileName     = "combined.log"
	serviceName            = "deployment-service"
)

func main() {
	var logFile *os.File

	if err := godotenv.Load(); err != nil {
		panic("could not load .env file")
	}

	if !env.IsDevMode() {
		var err error
		// Open or create the log file
		logFile, err = os.OpenFile(
			path.Join(env.GetLoggingDir(), defaultLogFileName),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY,
			0644,
		)
		if err != nil {
			panic(err) // or handle error appropriately
		}
		defer logFile.Close()
	}

	ctx := zeroLogConfiguration(logFile)
	log := zerolog.Ctx(ctx)

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

	dockerReleaser, err := releaser.NewDockerReleaser(releaser.DockerReleaserConfig{
		DockerClient:   dockerClient,
		ArtifactPrefix: env.GetArtifactPrefix(),
		RegistryAuth: &releaser.DockerAuth{
			Username:            env.GetDockerUserName(),
			PersonalAccessToken: env.GetDockerPAT(),
			ServerAddress:       env.GetDockerServer(),
		},
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate docker releaser")
	}

	dockerCompose := compose.New(compose.Config{
		CLI: compose.V2,
	})

	envWriter := service.NewEnvFileWriter()

	versioner := version.NewVersioner()
	backgroundProcessor, err := backgroundprocessor.NewBackgroundProcessor(backgroundprocessor.BackgroundProcessorConfig{
		Versioner:     versioner,
		SSHKeyPath:    env.GetSSHKeyPath(),
		GitRepoOrigin: env.GetGitRepoOirign(),
		CiCommitAuthor: &backgroundprocessor.CiCommitAuthor{
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

	interval, err := env.GetBackgroundProcessingInterval()
	if err != nil {
	}

	go func() {
		log.Info().Msg("Waiting on messages from serviceChannel to start background processing")
		for service := range serviceChannel {
			go processBackgroundJob(ctx, interval, backgroundProcessor, service)
		}
	}()

	if err := deploymentService.CollectExistingServicesForBackgroundProcessing(ctx); err != nil {
		log.Fatal().Err(err).Msg("Errored collecting existing services")
	}

	ginRouter.Any("/*path", func(c *gin.Context) {
		log.Info().Interface("request", c.Request).Msg("API Request")
		startTime := time.Now().UTC()

		route, pathParams, err := router.FindRoute(c.Request)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Error finding route: %v", err)})
			return
		}
		log.Info().Interface("route", route).Interface("pathParams", pathParams).Msg("Route and path params")

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
				errorHandler(ctx, err, c)
				return
			}
		case 2:
			methodResult = result[0].Interface()
			err, ok := result[1].Interface().(error)
			if ok {
				errorHandler(ctx, err, c)
				return
			}
		}

		log.Info().Int("status", firstSuccessfulResponseCode).
			Interface("response", methodResult).TimeDiff("latency", time.Now().UTC(), startTime).
			Str("httpMethod", route.Method).Str("path", route.Path).
			Msg("API Response")
		c.JSON(firstSuccessfulResponseCode, methodResult)
	})

	port := env.GetPort()
	log.Info().Str("port", port).Msgf("Server starting on :%s", port)
	if err := ginRouter.Run(fmt.Sprintf("%s:%s", defaultIPv4OpenAddress, port)); err != nil {
		log.Fatal().Err(err).Msg("Failed to run gin router")
	}
}

func errorHandler(ctx context.Context, err error, c *gin.Context) {
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

func abortWithStatusResponse(ctx context.Context, code int, err error, c *gin.Context) {
	log := zerolog.Ctx(ctx)
	log.Warn().Err(err).Int("status", code).Interface("request", c.Request).Msg("API Response Error")

	c.AbortWithStatusJSON(code, map[string]string{"message": err.Error()})
}

func zeroLogConfiguration(logFile *os.File) context.Context {
	if env.IsDevMode() {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	var writer io.Writer
	if logFile != nil {
		writer = io.MultiWriter(os.Stdout, logFile)
	} else {
		writer = os.Stdout
	}

	zerolog.TimeFieldFormat = time.RFC3339
	logger := zerolog.New(writer).With().
		Timestamp().
		Str("serviceName", serviceName).
		Str("serviceVersion", serviceVersion).
		Logger()

	ctx := context.Background()

	// Attach the Logger to the context.Context
	ctx = logger.WithContext(ctx)
	return ctx
}

func processBackgroundJob(
	ctx context.Context,
	interval time.Duration,
	backgroundProcessor backgroundprocessor.BackgroundProcesseror,
	service *model.Service,
) {
	log := zerolog.Ctx(ctx)
	log.Info().
		Str("service", service.Name).
		Msg("New service created, starting background processing")

	// Create ticker for repeating processing
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Str("service", service.Name).
				Msg("Stopping background processing due to context cancel")
			return
		case <-ticker.C:
			if err := backgroundProcessor.ProcessService(ctx, service); err != nil {
				log.Error().Err(err).
					Str("service", service.Name).
					Msg("Error when processing service")
			}
		}
	}
}
