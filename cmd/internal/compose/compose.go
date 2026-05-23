package compose

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog"
)

// CLIType is an enum for Docker Compose CLI version.
type CLIType int

const (
	V1                      CLIType = iota // docker-compose
	V2                                     // docker compose
	dockerComposeVersionKey = "VERSION"
)

// ComposeRunner defines the interface for running docker-compose commands.
type ComposeRunner interface {
	Up(ctx context.Context, composeDir string, version *semver.Version) (string, error)
	Down(ctx context.Context, composeDir string) (string, error)
}

// Config holds configuration options for the ComposeRunner.
type Config struct {
	// Optional: Docker host socket (e.g., for Colima)
	// DockerHost string

	// CLI version to use: V1 (docker-compose) or V2 (docker compose)
	CLI CLIType
}

// runner implements ComposeRunner
type runner struct {
	config Config
}

// New creates a new ComposeRunner instance with the given configuration.
func New(config Config) ComposeRunner {
	// Default to V1 if not specified
	if config.CLI != V1 && config.CLI != V2 {
		config.CLI = V1
	}
	return &runner{
		config: config,
	}
}

// Up runs `docker-compose up -d` or `docker compose up -d`.
func (r *runner) Up(ctx context.Context, composeDir string, version *semver.Version) (string, error) {
	return r.runComposeCommand(ctx, version, composeDir, "up", "-d")
}

// Down runs `docker-compose down` or `docker compose down`.
func (r *runner) Down(ctx context.Context, composeDir string) (string, error) {
	return r.runComposeCommand(ctx, nil, composeDir, "down")
}

func (r *runner) runComposeCommand(ctx context.Context, version *semver.Version, composeDir string, args ...string) (string, error) {
	info, err := os.Stat(composeDir)
	if err != nil {
		return "", fmt.Errorf("composeDir invalid: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("composeDir is not a directory: %s", composeDir)
	}

	var cmd *exec.Cmd

	switch r.config.CLI {
	case V1:
		cmd = exec.CommandContext(ctx, "/opt/homebrew/Cellar/docker-compose/2.33.1/bin/docker-compose", args...)
	case V2:
		dockerPath, err := exec.LookPath("docker")
		if err != nil {
			return "", fmt.Errorf("docker not found in PATH: %w", err)
		}
		cmd = exec.CommandContext(ctx, dockerPath, append([]string{"compose"}, args...)...)
	default:
		return "", fmt.Errorf("unsupported compose CLI version: %v", r.config.CLI)
	}

	cmd.Dir = composeDir
	// TODO: Need to standardize and input compose values
	cmd.Env = append(os.Environ(), r.constructVersionEnvVar(version))

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	log := zerolog.Ctx(ctx)
	log.Info().Str("command path", cmd.Path).Interface("args", cmd.Args).Msg(fmt.Sprintf("Running command: %s, %+v", cmd.Path, cmd.Args))
	log.Info().Str("Working directory:", cmd.Dir).Msg("Working dir")

	if err := cmd.Run(); err != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return output.String(), fmt.Errorf("command %v failed (exit code %d): %w", cmd.Args, exitCode, err)
	}

	log.Info().Str("output", output.String()).Msg("Up output")

	return output.String(), nil
}

func (r *runner) constructVersionEnvVar(version *semver.Version) string {
	return fmt.Sprintf("%s=%s", dockerComposeVersionKey, version.String())
}
