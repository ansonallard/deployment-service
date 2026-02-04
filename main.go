package main

import (
	"context"
	"os"
	"path"
	"time"

	backgroundprocessor "github.com/ansonallard/deployment-service/internal/background_processor"
	"github.com/ansonallard/deployment-service/internal/background_processor/npm"
	openapiBp "github.com/ansonallard/deployment-service/internal/background_processor/openapi"
	"github.com/ansonallard/deployment-service/internal/compose"
	"github.com/ansonallard/deployment-service/internal/controllers"
	"github.com/ansonallard/deployment-service/internal/env"
	"github.com/ansonallard/deployment-service/internal/middleware/authz"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/releaser"
	"github.com/ansonallard/deployment-service/internal/repo"
	"github.com/ansonallard/deployment-service/internal/service"
	"github.com/ansonallard/deployment-service/internal/version"
	"github.com/ansonallard/go_utils/logging"
	"github.com/ansonallard/go_utils/openapi"
	"github.com/joho/godotenv"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog"
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

	ctx := logging.ZeroLogConfiguration(logFile, env.IsDevMode(), serviceName, serviceVersion)
	log := zerolog.Ctx(ctx)

	log.Info().Msg("Loaded .env file")

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
		ArtifactPrefix: env.GetArtifactPrefix(ctx),
		RegistryAuth: &releaser.DockerAuth{
			Username:            env.GetDockerUserName(ctx),
			PersonalAccessToken: env.GetDockerPAT(ctx),
			ServerAddress:       env.GetDockerServer(ctx),
		},
		PathToDockerCLI: env.GetPathToDockerCLI(),
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

	backgroundProcessor, err := backgroundprocessor.NewBackgroundProcessor(backgroundprocessor.BackgroundProcessorConfig{
		Versioner:     versioner,
		SSHKeyPath:    env.GetSSHKeyPath(ctx),
		GitRepoOrigin: env.GetGitRepoOirign(ctx),
		CiCommitAuthor: &backgroundprocessor.CiCommitAuthor{
			Name:  env.GetCICommitAuthorName(ctx),
			Email: env.GetCICommitAuthorEmail(ctx),
		},
		NpmServiceProcessor: npmServiceProcessor,
		OpenAPIProcessor:    openAPIProcessor,
		IsDev:               env.IsDevMode(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to instantiate background processor")
	}

	interval, err := env.GetBackgroundProcessingInterval(ctx)
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

	port, err := env.GetPort()
	if err != nil {
		log.Fatal().Err(err).Msg("Could not parse port")
	}

	openAPIConfig, err := openapi.NewServeOpenAPIConfig().
		WithAuthZMiddleware(authZMiddleware).
		WithIsDevMode(env.IsDevMode()).
		WithOpenAPISpecFilePath(env.GetOpenAPIPath(ctx)).
		WithPort(port).
		WithServiceController(deploymentServiceController).
		Build()
	if err != nil {
		log.Fatal().Err(err).Msg("Couldn't configure server boilerplate")
	}
	if err := openapi.ServeOpenAPI(ctx, openAPIConfig); err != nil {
		log.Fatal().Err(err).Msg("Could not start server")
	}
}

func processBackgroundJob(
	ctx context.Context,
	interval time.Duration,
	backgroundProcessor backgroundprocessor.BackgroundProcesseror,
	service *model.Service,
) {
	log := zerolog.Ctx(ctx)
	log.Info().
		Str("service", service.Name.Name).
		Msg("New service created, starting background processing")

	// Create ticker for repeating processing
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Str("service", service.Name.Name).
				Msg("Stopping background processing due to context cancel")
			return
		case <-ticker.C:
			if err := backgroundProcessor.ProcessService(ctx, service); err != nil {
				log.Error().Err(err).
					Str("service", service.Name.Name).
					Msg("Error when processing service")
			}
		}
	}
}
