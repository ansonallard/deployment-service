package compose

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// CLIType is an enum for Docker Compose CLI version.
type CLIType int

const (
	V1 CLIType = iota // docker-compose
	V2                // docker compose
)

// ComposeRunner defines the interface for running docker-compose commands.
type ComposeRunner interface {
	Up(ctx context.Context, composeDir string) (string, error)
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
func (r *runner) Up(ctx context.Context, composeDir string) (string, error) {
	return r.runComposeCommand(ctx, composeDir, "up", "-d")
}

// Down runs `docker-compose down` or `docker compose down`.
func (r *runner) Down(ctx context.Context, composeDir string) (string, error) {
	return r.runComposeCommand(ctx, composeDir, "down")
}

// runComposeCommand executes the configured compose command with arguments.
func (r *runner) runComposeCommand(ctx context.Context, composeDir string, args ...string) (string, error) {
	var cmd *exec.Cmd

	switch r.config.CLI {
	case V1:
		cmd = exec.CommandContext(ctx, "/opt/homebrew/Cellar/docker-compose/2.33.1/bin/docker-compose", args...)
	case V2:
		cmd = exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	default:
		return "", fmt.Errorf("unsupported compose CLI version: %v", r.config.CLI)
	}

	cmd.Dir = composeDir

	cmd.Env = append(os.Environ(), "SERVER_VERSION=2.5.5")

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Run(); err != nil {
		return outBuf.String(), fmt.Errorf("%v failed: %w", cmd.Args, err)
	}

	return outBuf.String(), nil
}
