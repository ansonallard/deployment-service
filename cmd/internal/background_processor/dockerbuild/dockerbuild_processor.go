package dockerbuild

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/releaser"
	"github.com/rs/zerolog"
)

type DockerBuildProcessor interface {
	BuildAndPushDockerImage(
		ctx context.Context, service *model.Service, nextVersion *semver.Version,
	) error
}

type DockerBuildProcessorConfig struct {
	DockerReleaser releaser.DockerReleaser
}

type dockerBuildProcessor struct {
	dockerReleaser releaser.DockerReleaser
}

func NewDockerBuildProcessor(config DockerBuildProcessorConfig) (DockerBuildProcessor, error) {
	if config.DockerReleaser == nil {
		return nil, fmt.Errorf("dockerReleaser not provided")
	}
	return &dockerBuildProcessor{
		dockerReleaser: config.DockerReleaser,
	}, nil
}

func (dbp *dockerBuildProcessor) BuildAndPushDockerImage(
	ctx context.Context, service *model.Service, nextVersion *semver.Version,
) error {
	log := zerolog.Ctx(ctx)
	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Building docker image")

	dockerfilePath := service.Configuration.DockerBuild.DockerfilePath
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}

	tags := []string{
		dbp.dockerReleaser.CreateArtifactTag(service.Name.Name, nextVersion),
		dbp.dockerReleaser.CreateLatestArtifactTag(service.Name.Name),
	}

	if err := dbp.dockerReleaser.BuildImage(
		ctx,
		service.GitRepoFilePath,
		dockerfilePath,
		tags,
	); err != nil {
		return err
	}

	for _, tag := range tags {
		if err := dbp.dockerReleaser.PushImage(ctx, service.Name.Name, tag); err != nil {
			return err
		}
	}

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Successfully built and pushed docker image")

	return nil
}
