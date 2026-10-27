package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// CoexistenceDeclaration is an exact-match development declaration, not version
// negotiation. Contract covers persisted blocks, task outcomes/notifications and
// ownership semantics. Ownership changes require a new contract.
type CoexistenceDeclaration struct {
	Contract      string
	SchemaVersion int
}

// CurrentCoexistenceDeclaration describes only the deliberately selected fixture
// pair. Incompatible representation or ownership changes must change Contract;
// schema changes must also change SchemaVersion. Normal Open does not use it.
func CurrentCoexistenceDeclaration() CoexistenceDeclaration {
	return CoexistenceDeclaration{"cercano-dev-fixture-owned-v2", 2}
}

// PrepareCoexistence creates a NEW synthetic fixture store, including task
// tables, and stamps its declaration. It never adopts an existing destination
// (including an empty file or symlink). The parent directory must already exist.
// On failure the exclusively created file is left unprepared for inspection;
// callers must not retry by adopting it. This is not an application startup API.
func PrepareCoexistence(ctx context.Context, path string) error {
	return prepareCoexistence(ctx, path, CurrentCoexistenceDeclaration())
}

// DevelopmentCoexistenceDeclaration is the production development-store
// contract. It shares the ownership schema with fixtures but cannot be mistaken
// for a fixture declaration. It does not authorize adopting legacy databases.
func DevelopmentCoexistenceDeclaration() CoexistenceDeclaration {
	return CoexistenceDeclaration{"cercano-development-owned-v2", 2}
}

// PrepareDevelopmentStore exclusively creates a new development database.
// The caller must explicitly select a separate destination with an existing
// private parent. Existing files, including empty files and symlinks, are refused.
func PrepareDevelopmentStore(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("development store requires a clean absolute new path")
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	if resolved != parent {
		return fmt.Errorf("development store parent must not contain aliases")
	}
	info, err := os.Stat(parent)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("development store parent must be private")
	}
	return prepareCoexistence(ctx, path, DevelopmentCoexistenceDeclaration())
}

func prepareCoexistence(ctx context.Context, path string, d CoexistenceDeclaration) error {
	uri, err := coexistenceURI(path, "rw")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("prepare coexistence destination: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	s, err := Open(uri)
	if err != nil {
		return err
	}
	defer s.Close()
	store := s.(*sqliteStore)
	if err := store.InitTasks(ctx); err != nil {
		return err
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE coexistence_metadata (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 contract TEXT NOT NULL, schema_version INTEGER NOT NULL
 ); INSERT INTO coexistence_metadata VALUES (1, ?, ?);`, d.Contract, d.SchemaVersion); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, ownershipSchema); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", d.SchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

// PreflightCoexistence reads an existing store without creating it, applying
// schema, setting persistent pragmas, initializing tasks, or reconciling work.
// expected must match both this build and the prepared store exactly. Callers
// can use the selected peer's declaration here; unknown labels fail closed.
func PreflightCoexistence(ctx context.Context, path string, expected CoexistenceDeclaration) error {
	db, err := openCoexistenceDB(path, "ro")
	if err != nil {
		return err
	}
	defer db.Close()
	return validateCoexistence(ctx, db, expected)
}

// OpenCoexistence opens only an explicitly prepared declared store, never running Open
// migrations or task initialization/reconciliation. It rechecks metadata on the
// writable connection before setting connection-local options. Mutations require
// explicit capabilities supplied by WithOwnership. This is a storage foundation,
// not application admission, quiescence, or crash recovery.
func OpenCoexistence(ctx context.Context, path string, expected CoexistenceDeclaration) (Store, error) {
	if err := PreflightCoexistence(ctx, path, expected); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	if err := validateOwnershipPath(absolute); err != nil {
		return nil, err
	}
	db, err := openCoexistenceDB(absolute, "rw")
	if err != nil {
		return nil, err
	}
	if err := validateCoexistence(ctx, db, expected); err != nil {
		db.Close()
		return nil, err
	}
	// No journal_mode assignment: preparation alone selects persistent settings.
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000; PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, err
	}
	return &sqliteStore{db: db, coexistencePath: absolute, coexistenceDeclaration: expected}, nil
}

func coexistenceURI(path, mode string) (string, error) {
	if path == "" || path == ":memory:" {
		return "", fmt.Errorf("coexistence requires an explicit fixture file path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Treat the argument as a filesystem path, never as a caller-supplied DSN.
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{"mode": {mode}}
	// Connection-local settings must also apply to replacement connections. These
	// do not migrate schema or modify persistent SQLite settings.
	if mode == "rw" {
		q["_pragma"] = []string{"foreign_keys(1)", "busy_timeout(5000)"}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func openCoexistenceDB(path, mode string) (*sql.DB, error) {
	uri, err := coexistenceURI(path, mode)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func validateCoexistence(ctx context.Context, db *sql.DB, expected CoexistenceDeclaration) error {
	if expected != CurrentCoexistenceDeclaration() && expected != DevelopmentCoexistenceDeclaration() {
		return fmt.Errorf("unsupported coexistence declaration: %+v", expected)
	}
	// A single read transaction gives the marker and SQLite version one snapshot.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("coexistence preflight: %w", err)
	}
	defer tx.Rollback()
	var actual CoexistenceDeclaration
	var count, version int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM coexistence_metadata`).Scan(&count); err != nil {
		return fmt.Errorf("coexistence marker required: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("coexistence requires exactly one marker")
	}
	if err := tx.QueryRowContext(ctx, `SELECT contract, schema_version FROM coexistence_metadata WHERE singleton=1`).Scan(&actual.Contract, &actual.SchemaVersion); err != nil {
		return fmt.Errorf("coexistence marker: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if actual != expected || version != expected.SchemaVersion {
		return fmt.Errorf("coexistence mismatch: store=%+v user_version=%d expected=%+v", actual, version, expected)
	}
	return tx.Commit()
}
