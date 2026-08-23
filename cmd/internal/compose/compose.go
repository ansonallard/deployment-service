package compose

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
	logwriter "github.com/ansonallard/deployment-service/cmd/internal/log_writer"
	"github.com/rs/zerolog"
)

// ComposeRunner defines the interface for running docker-compose commands.
type ComposeRunner interface {
	Up(ctx context.Context, composeDir string, version *semver.Version) error
	Down(ctx context.Context, composeDir string) error
	Pull(ctx context.Context, composeDir string) error
}

// Config holds configuration options for the ComposeRunner.
type Config struct {
	DockerHome      string
	PathToDockerCLI string
}

// runner implements ComposeRunner
type runner struct {
	config Config
}

// New creates a new ComposeRunner instance with the given configuration.
func New(config Config) ComposeRunner {
	return &runner{
		config: config,
	}
}

// Up runs `docker-compose up -d` or `docker compose up -d`.
func (r *runner) Up(ctx context.Context, composeDir string, version *semver.Version) error {
	return r.runComposeCommand(ctx, version, composeDir, "up", "-d")
}

// Down runs `docker-compose down` or `docker compose down`.
func (r *runner) Down(ctx context.Context, composeDir string) error {
	return r.runComposeCommand(ctx, nil, composeDir, "down")
}

// Pull runs `docker-compose pull` or `docker compose pull`.
func (r *runner) Pull(ctx context.Context, composeDir string) error {
	return r.runComposeCommand(ctx, nil, composeDir, "pull")
}

func (r *runner) runComposeCommand(ctx context.Context, version *semver.Version, composeDir string, args ...string) error {
	info, err := os.Stat(composeDir)
	if err != nil {
		return fmt.Errorf("composeDir invalid: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("composeDir is not a directory: %s", composeDir)
	}

	cmd := exec.CommandContext(ctx, r.config.PathToDockerCLI, append([]string{"compose"}, args...)...)
	cmd.Dir = composeDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("DOCKER_HOME=%s", r.config.DockerHome))

	log := zerolog.Ctx(ctx)

	// Debug: log Docker-related env vars
	log.Debug().
		Str("DOCKER_HOST", os.Getenv("DOCKER_HOST")).
		Str("DOCKER_CONTEXT", os.Getenv("DOCKER_CONTEXT")).
		Str("DOCKER_CONFIG", os.Getenv("DOCKER_CONFIG")).
		Str("PATH", os.Getenv("PATH")).
		Msg("Docker environment variables")

	// Pipe stdout and stderr to zerolog
	cmd.Stdout = logwriter.NewZerologWriter(*log, zerolog.DebugLevel)
	cmd.Stderr = logwriter.NewZerologWriter(*log, zerolog.DebugLevel)

	log.Debug().Str("command path", cmd.Path).Interface("args", cmd.Args).Msg(fmt.Sprintf("Running command: %s, %+v", cmd.Path, cmd.Args))
	log.Debug().Str("Working directory:", cmd.Dir).Msg("Working dir")

	if err := cmd.Run(); err != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		// The stderr *should* be in your zerolog output already, but let's be explicit
		log.Error().
			Int("exit_code", exitCode).
			Str("command", strings.Join(cmd.Args, " ")).
			Err(err).
			Msg("Compose command failed")
		return fmt.Errorf("command %v failed (exit code %d): %w", cmd.Args, exitCode, err)
	}

	return nil
}
