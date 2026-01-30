package env

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func GetPort() (uint16, error) {
	strPort := getOptionalEnvVar("PORT", "5000")
	port, err := strconv.ParseUint(strPort, 10, 16)
	return uint16(port), err
}

func IsDevMode() bool {
	return strings.ToLower(getOptionalEnvVar("IS_DEV", "false")) == "true"
}

func GetOpenAPIPath(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "OPENAPI_PATH")
}

func GetAPIKey(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "API_KEY")
}

func GetSerivceFilePath(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "SERVICE_FILE_PATH")
}

func GetSSHKeyPath(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "SSH_KEY_PATH")
}

func GetGitRepoOirign(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "GIT_REPO_ORIGIN")
}

func GetCICommitAuthorName(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "CI_COMMIT_AUTHOR_NAME")
}

func GetCICommitAuthorEmail(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "CI_COMMIT_AUTHOR_EMAIL")
}

func GetDockerHome() string {
	return getOptionalEnvVarDefault("DOCKER_HOME")
}

func GetBackgroundProcessingInterval(ctx context.Context) (time.Duration, error) {
	return time.ParseDuration(getRequiredEnvVar(ctx, "BACKGROUND_PROCESSING_INTERVAL"))
}

func GetArtifactPrefix(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "ARTIFACT_PREFIX")
}

func GetDockerServer(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "DOCKER_SERVER")
}

func GetDockerUserName(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "DOCKER_USERNAME")
}

func GetDockerPAT(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "DOCKER_PAT")
}

func GetLoggingDir(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "LOGGING_DIR")
}

func GetArtifactRegistryURL(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "ARTIFACT_REGISTRY_URL")
}

func GetArtifactRegistryPAT(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "ARTIFACT_PAT")
}

func GetNPMPackageScope(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "NPM_PACKAGE_SCOPE")
}

func GetNPMRCPath(ctx context.Context) string {
	return getRequiredEnvVar(ctx, "NPMRC_PATH")
}

func GetPathToDockerCLI() string {
	return getOptionalEnvVar("PATH_TO_DOCKER_CLI", "/usr/bin/docker")
}

func getRequiredEnvVar(ctx context.Context, incomingEnvVar string) string {
	log := zerolog.Ctx(ctx)
	envVar := os.Getenv(incomingEnvVar)
	if envVar == "" {
		errMsg := fmt.Sprintf("Env var %s required by not set", incomingEnvVar)
		switch {
		case log == nil:
			panic(errMsg)
		default:
			log.Fatal().Msg(errMsg)
		}
	}
	return envVar
}

func getOptionalEnvVarDefault(incomingEnvVar string) string {
	return getOptionalEnvVar(incomingEnvVar, "")
}

func getOptionalEnvVar(incomingEnvVar, defaultArg string) string {
	envVar := os.Getenv(incomingEnvVar)
	if envVar == "" {
		return defaultArg
	}
	return envVar
}
