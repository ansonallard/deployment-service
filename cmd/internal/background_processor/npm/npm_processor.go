package npm

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/cmd/internal/compose"
	"github.com/ansonallard/deployment-service/cmd/internal/model"
	"github.com/ansonallard/deployment-service/cmd/internal/releaser"
	"github.com/ansonallard/deployment-service/cmd/internal/service"
	npmservice "github.com/ansonallard/deployment-service/cmd/internal/templates/npm_service"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/sjson"
)

const (
	packageJSONFilePath   = "package.json"
	packageJSONVersionKey = "version"
	dockerfileName        = "Dockerfile"
	nginxConf             = "nginx.conf"
)

type NPMServiceProcessor interface {
	SetPackageJsonVersion(service *model.Service, version *semver.Version) error
	BuildNpmService(
		ctx context.Context, service *model.Service, nextVersion *semver.Version,
	) error
}

type NPMServiceProcessorConfig struct {
	DockerReleaser releaser.DockerReleaser
	Compose        compose.ComposeRunner
	EnvWriter      service.EnvFileWriter
	NpmrcData      []byte
}

type npmServiceProcessor struct {
	dockerReleaser releaser.DockerReleaser
	compose        compose.ComposeRunner
	envFileWriter  service.EnvFileWriter
	npmrcData      []byte
}

func NewNPMServiceProcessor(config NPMServiceProcessorConfig) (NPMServiceProcessor, error) {
	if config.DockerReleaser == nil {
		return nil, fmt.Errorf("dockerReleaser not provided")
	}
	if config.Compose == nil {
		return nil, fmt.Errorf("compose not provided")
	}
	if config.EnvWriter == nil {
		return nil, fmt.Errorf("envwriter not provided")
	}
	if config.NpmrcData == nil {
		return nil, fmt.Errorf("NpmrcData not provided")
	}
	return &npmServiceProcessor{
		dockerReleaser: config.DockerReleaser,
		compose:        config.Compose,
		envFileWriter:  config.EnvWriter,
		npmrcData:      config.NpmrcData,
	}, nil
}

func (nsp *npmServiceProcessor) SetPackageJsonVersion(service *model.Service, version *semver.Version) error {
	if _, err := os.Stat(service.GitRepoFilePath); err != nil {
		return err
	}

	packageJsonFilePath := nsp.getPackageJsonPath(service.GitRepoFilePath)

	fileBytes, err := os.ReadFile(packageJsonFilePath)
	if err != nil {
		return err
	}

	packageJsonBytes, err := sjson.SetBytes(fileBytes, packageJSONVersionKey, version.String())
	if err != nil {
		return err
	}

	if err := os.WriteFile(packageJsonFilePath, packageJsonBytes, 0644); err != nil {
		return nil
	}
	return nil
}

func (nsp *npmServiceProcessor) getPackageJsonPath(gitRepoFilePath string) string {
	return path.Join(gitRepoFilePath, packageJSONFilePath)
}

func (nsp *npmServiceProcessor) BuildNpmService(
	ctx context.Context, service *model.Service, nextVersion *semver.Version,
) error {
	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Building image")

	if err := nsp.writeArtifacts(service); err != nil {
		return err
	}

	tags := []string{
		nsp.dockerReleaser.CreateArtifactTag(service.Name.Name, nextVersion),
		nsp.dockerReleaser.CreateLatestArtifactTag(service.Name.Name),
	}
	if err := nsp.dockerReleaser.BuildImageWithSecrets(
		ctx,
		service.GitRepoFilePath,
		dockerfileName,
		tags,
		map[string][]byte{
			releaser.NpmrcSecretKey: nsp.npmrcData,
		},
	); err != nil {
		return err
	}

	if err := nsp.removeArtifacts(service); err != nil {
		return err
	}

	for _, tag := range tags {
		if err := nsp.dockerReleaser.PushImage(ctx, service.Name.Name, tag); err != nil {
			return err
		}
	}
	return nil
}

func (nsp *npmServiceProcessor) writeDockerfile(service *model.Service) error {
	dockerfilePath := path.Join(service.GitRepoFilePath, dockerfileName)

	var dockerfileContents string
	switch service.Configuration.Npm.Service.ServiceType {
	case model.NpmServiceTypeBackend:
		dockerfileContents = npmservice.BackendDockerfile
	case model.NpmServiceTypeFrontend:
		dockerfileContents = npmservice.FrontendDockerfile
	}
	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContents), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

func (nsp *npmServiceProcessor) writeFrontendNginxConfig(service *model.Service) error {
	filePath := path.Join(service.GitRepoFilePath, nginxConf)
	if err := os.WriteFile(filePath, []byte(npmservice.FrontendNginxConfig), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

func (nsp *npmServiceProcessor) writeArtifacts(service *model.Service) error {
	if err := nsp.writeDockerfile(service); err != nil {
		return err
	}

	if service.Configuration.Npm.Service.ServiceType == model.NpmServiceTypeFrontend {
		if err := nsp.writeFrontendNginxConfig(service); err != nil {
			return err
		}
	}
	return nil
}

func (nsp *npmServiceProcessor) removeArtifacts(service *model.Service) error {
	dockerfilePath := path.Join(service.GitRepoFilePath, dockerfileName)
	if err := os.Remove(dockerfilePath); err != nil {
		return err
	}
	if service.Configuration.Npm.Service.ServiceType == model.NpmServiceTypeFrontend {
		filePath := path.Join(service.GitRepoFilePath, nginxConf)
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}
	return nil
}
