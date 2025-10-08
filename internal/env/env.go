package env

import (
	"fmt"
	"os"
	"strings"
)

func GetPort() string {
	return getOptionalEnvVar("PORT", "5000")
}

func IsDevMode() bool {
	return strings.ToLower(getOptionalEnvVar("IS_DEV", "false")) == "true"
}

func GetOpenAPIPath() string {
	return getRequiredEnvVar("OPENAPI_PATH")
}

func GetAPIKey() string {
	return getRequiredEnvVar("API_KEY")
}

func GetSerivceFilePath() string {
	return getRequiredEnvVar("SERVICE_FILE_PATH")
}

func GetSSHKeyPath() string {
	return getRequiredEnvVar("SSH_KEY_PATH")
}

func GetGitRepoOirign() string {
	return getRequiredEnvVar("GIT_REPO_ORIGIN")
}

func GetCICommitAuthorName() string {
	return getRequiredEnvVar("CI_COMMIT_AUTHOR_NAME")
}

func GetCICommitAuthorEmail() string {
	return getRequiredEnvVar("CI_COMMIT_AUTHOR_EMAIL")
}

func getRequiredEnvVar(incomingEnvVar string) string {
	envVar := os.Getenv(incomingEnvVar)
	if envVar == "" {
		panic(fmt.Sprintf("Env var %s required by not set", incomingEnvVar))
	}
	return envVar
}

func getOptionalEnvVar(incomingEnvVar, defaultArg string) string {
	envVar := os.Getenv(incomingEnvVar)
	if envVar == "" {
		return defaultArg
	}
	return envVar
}
