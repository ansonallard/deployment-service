package backgroundprocessor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type savedFile struct {
	rel  string
	data []byte
	perm os.FileMode
}

// snapshotFiles reads the given repo-relative files into memory. Files that
// don't exist are skipped (nothing to preserve).
func snapshotFiles(root string, rels []string) ([]savedFile, error) {
	var out []savedFile
	for _, rel := range rels {
		if !filepath.IsLocal(rel) {
			return nil, fmt.Errorf("preserved path %q must be relative and inside the repo", rel)
		}
		p := filepath.Join(root, rel)
		info, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		out = append(out, savedFile{rel: rel, data: data, perm: info.Mode().Perm()})
	}
	return out, nil
}

// restoreFiles writes snapshotted files back, atomically, with their original
// permissions.
func restoreFiles(root string, files []savedFile) error {
	var errs []error
	for _, f := range files {
		p := filepath.Join(root, f.rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			errs = append(errs, fmt.Errorf("mkdir for %s: %w", f.rel, err))
			continue
		}
		if err := writeFileAtomic(p, f.data, f.perm); err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", f.rel, err))
		}
	}
	return errors.Join(errs...)
}

// writeFileAtomic writes to a temp file in the same directory and renames it
// into place, so readers (docker compose, the backup script) never see a
// half-written file. Worth using in WriteEnvFile too.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
