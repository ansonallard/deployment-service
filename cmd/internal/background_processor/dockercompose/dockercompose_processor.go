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
)

type DockerComposeProcessor interface {
	DeployDockerComposeApplication(
		ctx context.Context, service *model.Service, nextVersion *semver.Version,
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

func (dcp *dockerComposeProcessor) DeployDockerComposeApplication(
	ctx context.Context, service *model.Service, nextVersion *semver.Version,
) error {
	log := zerolog.Ctx(ctx)
	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Writing env files")

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

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Starting compose application")
	if _, err := dcp.compose.Up(ctx, service.GitRepoFilePath, nextVersion); err != nil {
		return err
	}

	return nil
}
