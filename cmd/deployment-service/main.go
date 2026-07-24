package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime/debug"
	"time"

	"github.com/ansonallard/deployment-service/cmd/internal/api"
	backgroundprocessor "github.com/ansonallard/deployment-service/cmd/internal/background_processor"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/dockerbuild"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/dockercompose"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/goservice"
	"github.com/ansonallard/deployment-service/cmd/internal/background_processor/npm"
	openapiBp "github.com/ansonallard/deployment-service/cmd/internal/background_processor/openapi"
	"github.com/ansonallard/deployment-service/cmd/internal/compose"
	"github.com/ansonallard/deployment-service/cmd/internal/controllers"
	"github.com/ansonallard/deployment-service/cmd/internal/env"
	"github.com/ansonallard/deployment-service/cmd/internal/middleware"
	"github.com/ansonallard/deployment-service/cmd/internal/middleware/authz"
	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/releaser"
	"github.com/ansonallard/deployment-service/cmd/internal/repo"
	"github.com/ansonallard/deployment-service/cmd/internal/service"
	"github.com/ansonallard/deployment-service/cmd/internal/version"
	"github.com/ansonallard/deployment-service/cmd/service_version"
	"github.com/ansonallard/go_utils/logging"
	"github.com/ansonallard/go_utils/openapi/ierr"
	"github.com/ansonallard/go_utils/tracing"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/moby/moby/client"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultLogFileName = "combined.log"
	serviceName        = "deployment-service"
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
			// Note: Chicken and egg problem - zerolog isn't initialized yet,
			// but we need to pass in ctx to report a log.Fatal() if the
			// required env var isn't present.
			// This will panic instead of log.Fatal() if `LOGGING_DIR` isn't set.
			path.Join(env.GetLoggingDir(context.Background()), defaultLogFileName),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY,
			0644,
		)
		if err != nil {
			panic(err) // or handle error appropriately
		}
		defer logFile.Close()
	}

	logLevel := env.GetLogLevel()
	ctx := logging.ZeroLogConfiguration(logFile, &logLevel, serviceName, service_version.ServiceVersion)
	log := zerolog.Ctx(ctx)
	log.Info().Msg("Loaded .env file and initialized logging")

	shutdown, err := tracing.InitTracer(ctx, serviceName, service_version.ServiceVersion, env.GetTempoURI(ctx))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize tracer")
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Error shutting down tracer")
		}
	}()

	authZMiddleware := authz.NewAuthZ(env.GetAPIKey(ctx))

	deploymentServiceRepo, err := repo.NewDeploymentService(repo.DeploymentServieConfig{
		ServiceFilPath: env.GetSerivceFilePath(ctx),
		SSHKeyPath:     env.GetSSHKeyPath(ctx),
		GitClient:      repo.NewGitClient(),
		GitRepoOrigin:  env.GetGitRepoOirign(ctx),
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

	dockerClient, err := client.New(
		client.FromEnv,
		client.WithHost(env.GetDockerBuildHost(ctx)),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("could not instantiate docker client")
	}
	defer dockerClient.Close()

	dockerReleaser, err := releaser.NewDockerReleaser(releaser.DockerReleaserConfig{
		DockerClient:   dockerClient,
		ArtifactPrefix: env.GetArtifactPrefix(ctx),
		RegistryAuth: &releaser.DockerAuth{
			Username:            env.GetDockerUserName(ctx),
			PersonalAccessToken: env.GetDockerPAT(ctx),
			ServerAddress:       env.GetDockerServer(ctx),
		},
		PathToDockerCLI: env.GetPathToDockerCLI(),
		DockerHost:      env.GetDockerBuildHost(ctx),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate docker releaser")
	}

	dockerCompose := compose.New(compose.Config{
		CLI: compose.V2,
	})

	envWriter := service.NewEnvFileWriter()

	versioner := version.NewVersioner()

	npmrcPath := env.GetNPMRCPath(ctx)
	npmrcFileBytes, err := os.ReadFile(npmrcPath)
	if err != nil {
		log.Fatal().Err(err).Str("npmrcPath", npmrcPath).Msg("Failed to read npmrc")
	}

	npmServiceProcessor, err := npm.NewNPMServiceProcessor(npm.NPMServiceProcessorConfig{
		DockerReleaser: dockerReleaser,
		Compose:        dockerCompose,
		EnvWriter:      envWriter,
		NpmrcData:      npmrcFileBytes,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate npm service processor")
	}
	openAPIProcessor, err := openapiBp.NewOpenAPIProcessor(openapiBp.OpenAPIProcessorConfig{
		DockerReleaser: dockerReleaser,
		RegistryUrl:    env.GetArtifactRegistryURL(ctx),
		TypescriptClientConfig: &openapiBp.TypescriptClientConfig{
			NpmrcData:    npmrcFileBytes,
			PackageScope: env.GetNPMPackageScope(ctx),
		},
		GoClientConfig: &openapiBp.GoClientConfig{
			ModuleBasePath: env.GetArtifactPrefix(ctx),
			Token:          env.GetArtifactRegistryPAT(ctx),
		},
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate openapi processor")
	}

	goServiceProcessor, err := goservice.NewGoServiceProcessor(goservice.GoServiceProcessorConfig{
		DockerReleaser: dockerReleaser,
		GoUser:         env.GetGoUser(ctx),
		GoPAT:          env.GetGoPAT(ctx),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate go service processor")
	}

	dockerComposeProcessor, err := dockercompose.NewDockerComposeProcessor(dockercompose.DockerComposeProcessorConfig{
		Compose:   dockerCompose,
		EnvWriter: envWriter,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate docker compose processor")
	}

	dockerBuildProcessor, err := dockerbuild.NewDockerBuildProcessor(dockerbuild.DockerBuildProcessorConfig{
		DockerReleaser: dockerReleaser,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate docker build processor")
	}

	backgroundProcessor, err := backgroundprocessor.NewBackgroundProcessor(backgroundprocessor.BackgroundProcessorConfig{
		Versioner:     versioner,
		SSHKeyPath:    env.GetSSHKeyPath(ctx),
		GitRepoOrigin: env.GetGitRepoOirign(ctx),
		CiCommitAuthor: &backgroundprocessor.CiCommitAuthor{
			Name:  env.GetCICommitAuthorName(ctx),
			Email: env.GetCICommitAuthorEmail(ctx),
		},
		NpmServiceProcessor:    npmServiceProcessor,
		OpenAPIProcessor:       openAPIProcessor,
		GoServiceProcessor:     goServiceProcessor,
		DockerComposeProcessor: dockerComposeProcessor,
		DockerBuildProcessor:   dockerBuildProcessor,
		IsDev:                  env.IsDevMode(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate background processor")
	}

	interval, err := env.GetBackgroundProcessingInterval(ctx)
	if err != nil {
	}

	go func() {
		log.Info().Msg("Waiting on messages from serviceChannel to start background processing")
		for serviceName := range serviceChannel {
			go processBackgroundJob(ctx, interval, backgroundProcessor, deploymentService.Get, serviceName)
		}
	}()

	if err := deploymentService.CollectExistingServicesForBackgroundProcessing(ctx); err != nil {
		log.Fatal().Err(err).Msg("Errored collecting existing services")
	}

	port, err := env.GetPort()
	if err != nil {
		log.Fatal().Err(err).Msg("Could not parse port")
	}

	// Load embedded OpenAPI spec for request validation
	swagger, err := api.GetSwagger()
	if err != nil {
		log.Fatal().Err(err).Msg("Could not load OpenAPI spec.")
	}
	// Strip servers so kin-openapi doesn't reject requests based on host mismatch
	swagger.Servers = nil

	// Setup Gin router
	ginMode := gin.ReleaseMode
	if env.IsDevMode() {
		ginMode = gin.DebugMode
	}
	gin.SetMode(ginMode)

	router := gin.New()

	router.Use(otelgin.Middleware(serviceName))
	router.Use(logging.InjectLogger(log))
	router.Use(tracing.ZerologTraceMiddleware())
	router.Use(logging.RecoveryMiddleware(log))
	router.Use(logging.LoggingMiddleware())
	router.Use(middleware.ErrorHandlerMiddleware())
	router.Use(authZMiddleware.AuthMiddleware())

	ginmiddleware.OapiRequestValidatorWithOptions(swagger, &ginmiddleware.Options{
		ErrorHandler: func(c *gin.Context, message string, statusCode int) {
			_ = c.Error(ierr.NewBadRequestError(message))
			c.Abort()
		},
	})

	strictHandler := api.NewStrictHandler(deploymentServiceController, nil)

	// Register OpenAPI handlers (generated by oapi-codegen)
	api.RegisterHandlersWithOptions(router, strictHandler, api.GinServerOptions{
		BaseURL: "/v1",
	})

	// Start server
	log.Info().Uint16("port", port).Msgf("Server starting on :%d", port)
	if err := router.Run(fmt.Sprintf(":%d", port)); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

func processBackgroundJob(
	ctx context.Context,
	interval time.Duration,
	backgroundProcessor backgroundprocessor.BackgroundProcesseror,
	getService func(context.Context, string) (*model.Service, error),
	serviceName string,
) {
	tracer := otel.Tracer(fmt.Sprintf("%s/background", serviceName))

	log := zerolog.Ctx(ctx)
	log.Info().
		Str("service", serviceName).
		Msg("New service created, starting background processing")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Str("service", serviceName).
				Msg("Stopping background processing due to context cancel")
			return

		case <-ticker.C:
			if shouldStop := runTick(ctx, tracer, backgroundProcessor, getService, serviceName); shouldStop {
				return
			}
		}
	}
}

// runTick runs a single processing tick as its own root trace.
// Returns true if the background job should stop entirely.
func runTick(
	parentCtx context.Context,
	tracer trace.Tracer,
	backgroundProcessor backgroundprocessor.BackgroundProcesseror,
	getService func(context.Context, string) (*model.Service, error),
	serviceName string,
) (stop bool) {
	tickCtx, tickSpan := tracer.Start(parentCtx, "background.tick",
		trace.WithAttributes(attribute.String("service", serviceName)),
	)
	defer tickSpan.End()

	sc := tickSpan.SpanContext()
	enrichedLog := zerolog.Ctx(tickCtx).With().
		Str("traceID", sc.TraceID().String()).
		Str("spanID", sc.SpanID().String()).
		Logger()
	tickCtx = enrichedLog.WithContext(tickCtx)
	log := zerolog.Ctx(tickCtx)

	service, err := getService(tickCtx, serviceName)
	if err != nil {
		if _, ok := err.(*ierr.NotFoundError); ok {
			log.Info().Str("service", serviceName).
				Msg("Service deleted, stopping background processing")
			return true
		}

		log.Error().Err(err).Str("service", serviceName).
			Msg("Failed to get service for background processing")
		tickSpan.RecordError(err)
		tickSpan.SetStatus(codes.Error, err.Error())
		return false
	}

	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Str("service", serviceName).
				Interface("panic", r).
				Bytes("stack", debug.Stack()).
				Msg("Panic recovered in background processor")
			tickSpan.SetStatus(codes.Error, "panic recovered")
		}
	}()

	if err := backgroundProcessor.ProcessService(tickCtx, service); err != nil {
		log.Error().Err(err).Str("service", serviceName).
			Msg("Error when processing service")
		tickSpan.RecordError(err)
		tickSpan.SetStatus(codes.Error, err.Error())
	}

	return false
}
