package goservice

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/releaser"
	goservicetemplate "github.com/ansonallard/deployment-service/internal/templates/go_service"
	"github.com/rs/zerolog/log"
)

const versionFileName = "version.txt"
const dockerfileName = "Dockerfile"

type GoServiceProcessor interface {
	SetVersionFile(service *model.Service, version *semver.Version) error
	BuildGoService(
		ctx context.Context, service *model.Service, nextVersion *semver.Version,
	) error
}

type GoServiceProcessorConfig struct {
	DockerReleaser releaser.DockerReleaser
	GoUser         string
	GoPAT          string
}

type goServiceProcessor struct {
	dockerReleaser releaser.DockerReleaser
	goUser         string
	goPAT          string
}

func NewGoServiceProcessor(config GoServiceProcessorConfig) (GoServiceProcessor, error) {
	if config.DockerReleaser == nil {
		return nil, fmt.Errorf("dockerReleaser not provided")
	}
	if config.GoUser == "" {
		return nil, fmt.Errorf("goUser not provided")
	}
	if config.GoPAT == "" {
		return nil, fmt.Errorf("goPAT not provided")
	}
	return &goServiceProcessor{
		dockerReleaser: config.DockerReleaser,
		goUser:         config.GoUser,
		goPAT:          config.GoPAT,
	}, nil
}

func (gsp *goServiceProcessor) SetVersionFile(service *model.Service, version *semver.Version) error {
	versionFilePath := path.Join(service.GitRepoFilePath, versionFileName)
	if err := os.WriteFile(versionFilePath, []byte(version.String()), 0644); err != nil {
		return fmt.Errorf("failed to write version file: %w", err)
	}
	return nil
}

func (gsp *goServiceProcessor) writeDockerfile(service *model.Service) error {
	dockerfilePath := path.Join(service.GitRepoFilePath, dockerfileName)
	if err := os.WriteFile(dockerfilePath, []byte(goservicetemplate.Dockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	return nil
}

func (gsp *goServiceProcessor) BuildGoService(
	ctx context.Context, service *model.Service, nextVersion *semver.Version,
) error {
	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Building Go service image")

	if err := gsp.writeDockerfile(service); err != nil {
		return err
	}

	tags := []string{
		gsp.dockerReleaser.CreateArtifactTag(service.Name.Name, nextVersion),
		gsp.dockerReleaser.CreateLatestArtifactTag(service.Name.Name),
	}

	if err := gsp.dockerReleaser.BuildImageWithSecrets(
		ctx,
		service.GitRepoFilePath,
		"Dockerfile",
		tags,
		map[string][]byte{
			releaser.GoUserKey: []byte(gsp.goUser),
			releaser.GoPATKey:  []byte(gsp.goPAT),
		},
	); err != nil {
		return err
	}

	for _, tag := range tags {
		if err := gsp.dockerReleaser.PushImage(ctx, service.Name.Name, tag); err != nil {
			return err
		}
	}

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Successfully built and pushed Go service image")

	return nil
}
