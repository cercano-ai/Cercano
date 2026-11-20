package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
