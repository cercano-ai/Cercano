//go:build !windows

package conversation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func backupOfflineWithSync(ctx context.Context, path string, syncFile func(*os.File) error) error {
	// Save a coherent SQLite snapshot (including committed WAL), not a raw main
	// file copy. The reserved destination is private and never overwritten. The
	// second connection reads committed data while this connection holds writer
	// admission. Keep the backup even on migration failure for operator inspection.
	backup := path + ".pre-upgrade-backup"
	f, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("reserve offline upgrade backup: %w", err)
	}
	if err = f.Close(); err != nil {
		return err
	}
	reader, err := openCoexistenceDB(path, "ro")
	if err != nil {
		return err
	}
	_, err = reader.ExecContext(ctx, "VACUUM INTO ?", backup)
	reader.Close()
	if err != nil {
		return fmt.Errorf("save offline upgrade backup %q: %w", backup, err)
	}
	// VACUUM INTO produces a consistent file; explicitly flush it before any
	// source changes rather than depending on SQLite version-specific flushing.
	saved, err := os.OpenFile(backup, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = syncFile(saved)
	closeErr := saved.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	// Publishing the new backup name requires a separate directory barrier.
	// Do not treat unsupported directory flushing as successful durability.
	dir, err := os.Open(filepath.Dir(backup))
	if err != nil {
		return fmt.Errorf("open offline backup directory: %w", err)
	}
	err = syncFile(dir)
	closeErr = dir.Close()
	if err != nil {
		return fmt.Errorf("sync offline backup directory: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close offline backup directory: %w", closeErr)
	}
	return nil
}
