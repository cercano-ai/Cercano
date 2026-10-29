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
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("prepared worker: absolute clean failure-log path required")
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return fmt.Errorf("prepared worker: failure-log parent must exist without aliases")
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return fmt.Errorf("prepared worker: failure-log parent must be a private 0700 directory")
	}
	info, err = os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("prepared worker: failure-log destination: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return fmt.Errorf("prepared worker: failure-log destination must be a private regular file")
	}
	return nil
}

func (p SpawnPolicy) failureLogPath() string {
	if !p.pinned {
		return ""
	}
	return filepath.Join(p.dir, "failures.jsonl")
}
