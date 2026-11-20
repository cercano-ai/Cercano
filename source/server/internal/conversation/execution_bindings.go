package conversation

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// executionBindingsSchema persists explicit selection only. No row means legacy
// host behavior; paths never identify an executor. These structural constraints
// confer no execution authority, consent, or ownership of host orchestration.
// Roots are opaque executor-local paths (including non-host OS path formats).
const executionBindingsSchema = `CREATE TABLE execution_bindings (
 conversation_id TEXT PRIMARY KEY NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
 location TEXT NOT NULL CHECK(location IN ('host', 'client')),
 executor_id TEXT NOT NULL CHECK(typeof(executor_id) = 'text' AND length(trim(executor_id)) > 0),
 workspace_id TEXT NOT NULL CHECK(typeof(workspace_id) = 'text' AND length(trim(workspace_id)) > 0),
 workspace_root TEXT NOT NULL CHECK(typeof(workspace_root) = 'text' AND length(trim(workspace_root)) > 0),
 ownership_generation INTEGER NOT NULL CHECK(typeof(ownership_generation) = 'integer' AND ownership_generation > 0)
);`

// validateExistingExecutionBindings deliberately accepts only our emitted DDL.
// PRAGMA table_info alone cannot prove CHECK constraints or cascading ownership
// references. Unknown layouts require an explicit migration, not silent adoption.
func validateExistingExecutionBindings(db *sql.DB) error {
	var ddl string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='execution_bindings'`).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect execution_bindings: %w", err)
	}
	normalize := func(s string) string {
		s = strings.TrimSuffix(strings.TrimSpace(s), ";")
		s = strings.Replace(s, "CREATE TABLE IF NOT EXISTS", "CREATE TABLE", 1)
		return strings.Join(strings.Fields(s), " ")
	}
	if normalize(ddl) != normalize(executionBindingsSchema) {
		return fmt.Errorf("incompatible execution_bindings schema; explicit migration required")
	}
	return nil
}
