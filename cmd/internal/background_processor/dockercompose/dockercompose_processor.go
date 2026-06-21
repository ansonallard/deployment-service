package dockercompose

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/cmd/internal/compose"
	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/service"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("deployment-service.processor.dockercompose")

type DockerComposeProcessor interface {
	DeployDockerComposeApplication(
		ctx context.Context, service *model.Service, nextVersion *semver.Version,
	) error
	RefreshDockerComposeApplication(
		ctx context.Context, service *model.Service,
	) error
}

type DockerComposeProcessorConfig struct {
	Compose   compose.ComposeRunner
	EnvWriter service.EnvFileWriter
}

type dockerComposeProcessor struct {
	compose       compose.ComposeRunner
	envFileWriter service.EnvFileWriter
}

func NewDockerComposeProcessor(config DockerComposeProcessorConfig) (DockerComposeProcessor, error) {
	if config.Compose == nil {
		return nil, fmt.Errorf("compose not provided")
	}
	if config.EnvWriter == nil {
		return nil, fmt.Errorf("envWriter not provided")
	}
	return &dockerComposeProcessor{
		compose:       config.Compose,
		envFileWriter: config.EnvWriter,
	}, nil
}

func (dcp *dockerComposeProcessor) writeEnvFiles(ctx context.Context, service *model.Service) error {
	config := service.Configuration.DockerCompose
	var errs []error
	for envFile, envVars := range config.EnvFiles {
		if err := dcp.envFileWriter.WriteEnvFile(ctx, service.GitRepoFilePath, envFile, envVars); err != nil {
			errs = append(errs, fmt.Errorf("failed to write env file %s: %w", envFile, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (dcp *dockerComposeProcessor) DeployDockerComposeApplication(
	ctx context.Context, service *model.Service, nextVersion *semver.Version,
) error {
	ctx, span := tracer.Start(ctx, "dockercompose.deploy",
		trace.WithAttributes(
			attribute.String("service.name", service.Name.Name),
			attribute.String("version", nextVersion.String()),
		),
	)
	defer span.End()

	log := zerolog.Ctx(ctx)
	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Writing env files")

	if err := dcp.writeEnvFiles(ctx, service); err != nil {
		return err
	}

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Starting compose application")
	if err := dcp.compose.Up(ctx, service.GitRepoFilePath, nextVersion); err != nil {
		return err
	}

	return nil
}

func (dcp *dockerComposeProcessor) RefreshDockerComposeApplication(
	ctx context.Context, service *model.Service,
) error {
	ctx, span := tracer.Start(ctx, "dockercompose.refresh",
		trace.WithAttributes(attribute.String("service.name", service.Name.Name)),
	)
	defer span.End()

	log := zerolog.Ctx(ctx)
	log.Debug().Str("service", service.Name.Name).Msg("Pulling latest images")

	if err := dcp.compose.Pull(ctx, service.GitRepoFilePath); err != nil {
		return fmt.Errorf("failed to pull images: %w", err)
	}

	log.Debug().Str("service", service.Name.Name).Msg("Writing env files")

	if err := dcp.writeEnvFiles(ctx, service); err != nil {
		return err
	}

	log.Debug().Str("service", service.Name.Name).Msg("Starting compose application")
	if err := dcp.compose.Up(ctx, service.GitRepoFilePath, nil); err != nil {
		return err
	}

	return nil
}
