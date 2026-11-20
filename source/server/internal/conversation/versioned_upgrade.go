package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// declaredMigration is deliberately private: only reviewed, compiled-in steps
// may change a declared store. SQL must not contain transaction control or modify
// either version marker; the runner owns those. Startup never calls this runner.
type declaredMigration struct {
	from, to   CoexistenceDeclaration
	statements []string
}

var developmentMigrations = []declaredMigration{{
	from:       CoexistenceDeclaration{"cercano-development-owned-v2", 2},
	to:         DevelopmentCoexistenceDeclaration(),
	statements: []string{executionBindingsSchema},
}}

// upgradeDeclared runs under the caller's BEGIN IMMEDIATE. All writers must be
// stopped and remain stopped through backup, commit and compatible restart.
// SQLite admission is not a fence against historical/custom writers. No tasks
// are reconciled, bindings inferred from paths, or ownership epochs reset.
func upgradeDeclared(ctx context.Context, db *sql.DB, path string, target CoexistenceDeclaration, steps []declaredMigration) error {
	return upgradeDeclaredWithSync(ctx, db, path, target, steps, (*os.File).Sync)
}

func upgradeDeclaredWithSync(ctx context.Context, db *sql.DB, path string, target CoexistenceDeclaration, steps []declaredMigration, syncFile func(*os.File) error) error {
	var actual CoexistenceDeclaration
	var count, uv int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM coexistence_metadata").Scan(&count); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, "SELECT contract,schema_version FROM coexistence_metadata WHERE singleton=1").Scan(&actual.Contract, &actual.SchemaVersion); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&uv); err != nil {
		return err
	}
	if count != 1 || uv != actual.SchemaVersion {
		return fmt.Errorf("offline upgrade refuses inconsistent marker")
	}
	var route []declaredMigration
	for next := actual; next != target; {
		var found *declaredMigration
		for i := range steps {
			if steps[i].from == next {
				if found != nil {
					return fmt.Errorf("ambiguous offline migration from %+v", next)
				}
				found = &steps[i]
			}
		}
		if found == nil || found.to.SchemaVersion <= next.SchemaVersion || found.to.SchemaVersion > target.SchemaVersion {
			return fmt.Errorf("offline upgrade refuses unsupported declaration %+v", next)
		}
		route = append(route, *found)
		next = found.to
	}
	if len(route) == 0 {
		return nil
	}
	if err := backupOfflineWithSync(ctx, path, syncFile); err != nil {
		return err
	}
	for _, step := range route {
		for _, statement := range step.statements {
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("offline migration %+v to %+v: %w", step.from, step.to, err)
			}
		}
		if _, err := db.ExecContext(ctx, "UPDATE coexistence_metadata SET contract=?,schema_version=? WHERE singleton=1", step.to.Contract, step.to.SchemaVersion); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", step.to.SchemaVersion)); err != nil {
			return err
		}
	}
	return nil
}

func backupOffline(ctx context.Context, path string) error {
	return backupOfflineWithSync(ctx, path, (*os.File).Sync)
}

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
	// Windows needs a separately validated directory-handle implementation;
	// ordinary os.File.Sync is not a supported substitute there.
	if runtime.GOOS == "windows" {
		return fmt.Errorf("offline upgrade backup directory sync unsupported on Windows")
	}
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
