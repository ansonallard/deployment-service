package utils

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ArtifactSet tracks files generated into a repo during a build so they can be
// removed afterwards, regardless of which processor created them.
type ArtifactSet struct {
	root  string
	paths []string
}

// WithArtifacts runs fn with a fresh ArtifactSet and always cleans up afterwards.
// A cleanup error is joined with fn's error, so it never masks a build failure.
func WithArtifacts(root string, fn func(a *ArtifactSet) error) (err error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	a := &ArtifactSet{root: absRoot}
	defer func() {
		err = errors.Join(err, a.Cleanup())
	}()
	return fn(a)
}

// Register marks a path for removal. Call it BEFORE writing, so a partially
// written file is still cleaned up. Paths outside the repo root are rejected.
func (a *ArtifactSet) Register(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(a.root, abs)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("artifact %q is outside repo %q", abs, a.root)
	}
	a.paths = append(a.paths, abs)
	return nil
}

// WriteFile registers the path, then writes the file.
func (a *ArtifactSet) WriteFile(p string, data []byte, perm os.FileMode) error {
	if err := a.Register(p); err != nil {
		return err
	}
	if err := os.WriteFile(p, data, perm); err != nil {
		return fmt.Errorf("failed to write %s: %w", p, err)
	}
	return nil
}

// Cleanup removes every registered file in reverse order. Missing files are
// ignored. It is safe to call more than once.
func (a *ArtifactSet) Cleanup() error {
	var errs []error
	for i := len(a.paths) - 1; i >= 0; i-- {
		if err := os.Remove(a.paths[i]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("failed to remove artifact %s: %w", a.paths[i], err))
		}
	}
	a.paths = nil
	return errors.Join(errs...)
}
