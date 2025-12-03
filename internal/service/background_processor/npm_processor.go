package backgroundprocessor

import (
	"context"
	"os"
	"path"

	"github.com/Masterminds/semver/v3"
	"github.com/ansonallard/deployment-service/internal/model"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/sjson"
)

func (bp *backgroundProcessor) setPackageJsonVersion(service *model.Service, version *semver.Version) error {
	if _, err := os.Stat(service.GitRepoFilePath); err != nil {
		return err
	}

	packageJsonFilePath := bp.getPackageJsonPath(service.GitRepoFilePath)

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

func (bp *backgroundProcessor) getPackageJsonPath(gitRepoFilePath string) string {
	return path.Join(gitRepoFilePath, packageJSONFilePath)
}

func (bp *backgroundProcessor) buildAndDeployNpmService(
	ctx context.Context, service *model.Service, nextVersion *semver.Version,
) error {
	log.Info().Str("service", service.Name).Str("nextVersion", nextVersion.String()).Msg("Building image")
	if err := bp.dockerReleaser.BuildImage(
		ctx,
		service.Name,
		service.GitRepoFilePath,
		service.Configuration.Npm.Service.DockerfilePath,
		nextVersion,
	); err != nil {
		return err
	}

	if err := bp.dockerReleaser.PushImage(ctx, service.Name, nextVersion); err != nil {
		return err
	}

	log.Info().Str("service", service.Name).Str("nextVersion", nextVersion.String()).Msg("Writing env vars")
	if err := bp.envFileWriter.WriteEnvFile(
		ctx,
		service.GitRepoFilePath,
		service.Configuration.Npm.Service.EnvPath,
		service.Configuration.Npm.Service.EnvVars,
	); err != nil {
		return err
	}

	log.Info().Str("service", service.Name).Str("nextVersion", nextVersion.String()).Msg("Starting service")
	if _, err := bp.compose.Up(ctx, service.GitRepoFilePath, nextVersion); err != nil {
		return err
	}

	return nil
}
