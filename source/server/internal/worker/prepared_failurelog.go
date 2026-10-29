package worker

import (
	"fmt"
	"os"
	"path/filepath"
)

// WithFailureLogPath supplies constructor-pinned authority, never turn metadata
// or a HOME/environment default. Prepared workers require this option.
func WithFailureLogPath(path string) Option {
	return func(w *WorkerServer) { w.failureLogPath = path }
}

// ValidatePreparedFailureLog checks the private host namespace before any log
// open. Its parent must already exist, be canonical and owner-only; an existing
// destination must be an owner-only regular file, never a symlink.
func ValidatePreparedFailureLog(path string) error {
	return validatePreparedLog(path, "failure-log")
}

func validatePreparedLog(path, label string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("prepared worker: absolute clean %s path required", label)
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return fmt.Errorf("prepared worker: %s parent must exist without aliases", label)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return fmt.Errorf("prepared worker: %s parent must be a private 0700 directory", label)
	}
	info, err = os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("prepared worker: %s destination: %w", label, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("prepared worker: %s destination must be a private regular file", label)
	}
	return nil
}

func (p SpawnPolicy) failureLogPath() string {
	if !p.pinned {
		return ""
	}
	return filepath.Join(p.dir, "failures.jsonl")
}
