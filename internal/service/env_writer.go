package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/rs/zerolog"
)

// EnvFileWriter defines behavior for writing environment variable files.
type EnvFileWriter interface {
	WriteEnvFile(ctx context.Context, path, envFile string, envVars map[string]any) error
}

// envFileWriter is the concrete implementation.
type envFileWriter struct {
	defaultPermissions os.FileMode
}

// NewEnvFileWriter creates a new EnvFileWriter instance.
// Default permissions are 0644.
func NewEnvFileWriter() EnvFileWriter {
	return &envFileWriter{
		defaultPermissions: 0o644,
	}
}

// WriteEnvFile writes a map of environment variables to a .env file
// at the specified directory path. It validates that the directory exists
// and overwrites the file if it already exists.
func (w *envFileWriter) WriteEnvFile(ctx context.Context, path, envFile string, envVars map[string]any) error {
	log := zerolog.Ctx(ctx)
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if envFile == "" {
		return fmt.Errorf("envFile cannot be empty")
	}
	if len(envVars) == 0 {
		log.Warn().Msg("Env vars is empty, not commit env var files to host.")
		return nil
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("context canceled before writing env file: %w", ctx.Err())
	default:
	}

	// Ensure directory exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", path)
		}
		return fmt.Errorf("failed to access path %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	targetFile := filepath.Join(path, envFile)

	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	content := ""
	for _, k := range keys {
		content += fmt.Sprintf("%s=%v\n", k, envVars[k])
	}

	if err := os.WriteFile(targetFile, []byte(content), w.defaultPermissions); err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}

	return nil
}
