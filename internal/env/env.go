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
