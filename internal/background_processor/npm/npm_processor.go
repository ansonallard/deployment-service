package npm

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/internal/compose"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/ansonallard/deployment-service/internal/releaser"
	"github.com/ansonallard/deployment-service/internal/service"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/sjson"
)

const (
	packageJSONFilePath   = "package.json"
	packageJSONVersionKey = "version"
)

type NPMServiceProcessor interface {
	SetPackageJsonVersion(service *model.Service, version *semver.Version) error
	BuildAndDeployNpmService(
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

func (nsp *npmServiceProcessor) BuildAndDeployNpmService(
	ctx context.Context, service *model.Service, nextVersion *semver.Version,
) error {
	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Building image")
	if err := nsp.dockerReleaser.BuildImageWithSecrets(
		ctx,
		service.GitRepoFilePath,
		service.Configuration.Npm.Service.DockerfilePath,
		[]string{
			nsp.dockerReleaser.FullyQualifiedImageTag(service.Name.Name, nextVersion),
			nsp.dockerReleaser.CreateArtifactTag(service.Name.Name, nextVersion),
		},
		map[string][]byte{
			releaser.NpmrcSecretKey: nsp.npmrcData,
		},
	); err != nil {
		return err
	}

	if err := nsp.dockerReleaser.PushImage(ctx, service.Name.Name, nextVersion); err != nil {
		return err
	}

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Writing env vars")
	if err := nsp.envFileWriter.WriteEnvFile(
		ctx,
		service.GitRepoFilePath,
		service.Configuration.Npm.Service.EnvPath,
		service.Configuration.Npm.Service.EnvVars,
	); err != nil {
		return err
	}

	log.Info().Str("service", service.Name.Name).Str("nextVersion", nextVersion.String()).Msg("Starting service")
	if _, err := nsp.compose.Up(ctx, service.GitRepoFilePath, nextVersion); err != nil {
		return err
	}

	return nil
}
