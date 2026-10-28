package telemetry

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
)

type schemaConnection interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Only the three historically supported additive upgrades are allowed. No
// incompatible table is rebuilt, reinterpreted or stamped as compatible.
var legacyAdditions = map[string]string{
	"token_saving":           "BOOLEAN NOT NULL DEFAULT 1",
	"session_id":             "TEXT NOT NULL DEFAULT ''",
	"content_tokens_avoided": "INTEGER NOT NULL DEFAULT 0",
}

var legacyColumns = map[string]map[string]string{
	"events": {
		"id": "INTEGER", "timestamp": "DATETIME", "tool_name": "TEXT", "model": "TEXT",
		"input_tokens": "INTEGER", "output_tokens": "INTEGER", "duration_ms": "INTEGER",
		"was_escalated": "BOOLEAN", "cloud_provider": "TEXT", "cloud_model": "TEXT",
		"token_saving": "BOOLEAN", "session_id": "TEXT", "content_tokens_avoided": "INTEGER",
	},
	"cloud_usage": {
		"id": "INTEGER", "timestamp": "DATETIME", "cloud_input_tokens": "INTEGER",
		"cloud_output_tokens": "INTEGER", "cloud_provider": "TEXT", "cloud_model": "TEXT",
	},
	"sessions": {"session_id": "TEXT", "started_at": "DATETIME"},
}

func initializeLegacySchema(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open telemetry schema connection: %w", err)
	}
	defer conn.Close()
	// Serialize inspection and ALTERs across simultaneous openers. Like the
	// accounting initializer, never upgrade a deferred read snapshot to a write.
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("begin telemetry schema initialization: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
	}()
	if err = validateLegacySchema(conn, true); err != nil {
		return err
	}
	if err = createTables(conn); err != nil {
		return err
	}
	if err = migrateSchema(conn); err != nil {
		return err
	}
	if err = validateLegacySchema(conn, false); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit telemetry schema initialization: %w", err)
	}
	committed = true
	return nil
}

func schemaColumns(db schemaConnection, table string) (map[string]string, error) {
	rows, err := db.QueryContext(context.Background(), "PRAGMA table_xinfo("+table+")")
	if err != nil {
		return nil, fmt.Errorf("inspect telemetry table %s: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]string)
	for rows.Next() {
		var cid, notNull, pk, hidden int
		var name, typ string
		var def sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &def, &pk, &hidden); err != nil {
			return nil, err
		}
		want, known := legacyColumns[table][name]
		wantPK := 0
		if name == "id" || (table == "sessions" && name == "session_id") {
			wantPK = 1
		}
		if known {
			if !strings.EqualFold(typ, want) || pk != wantPK || hidden != 0 || (wantPK == 0 && notNull != 1) {
				return nil, fmt.Errorf("incompatible telemetry column %s.%s", table, name)
			}
		} else if pk != 0 || (hidden == 0 && notNull != 0 && !def.Valid) {
			return nil, fmt.Errorf("unsupported required telemetry column %s.%s", table, name)
		}
		columns[name] = typ
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect telemetry columns: %w", err)
	}
	return columns, nil
}

func validateLegacySchema(db schemaConnection, allowUpgrade bool) error {
	for table, expected := range legacyColumns {
		var kind string
		err := db.QueryRowContext(context.Background(), "SELECT type FROM sqlite_schema WHERE name=?", table).Scan(&kind)
		if err == sql.ErrNoRows && allowUpgrade {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect telemetry object %s: %w", table, err)
		}
		if kind != "table" {
			return fmt.Errorf("incompatible telemetry object %s: expected table, got %s", table, kind)
		}
		columns, err := schemaColumns(db, table)
		if err != nil {
			return err
		}
		for name := range expected {
			if _, ok := columns[name]; ok {
				continue
			}
			if _, additive := legacyAdditions[name]; allowUpgrade && table == "events" && additive {
				continue
			}
			return fmt.Errorf("incompatible telemetry table %s: missing column %s", table, name)
		}
	}
	return nil
}

func migrateSchema(db schemaConnection) error {
	columns, err := schemaColumns(db, "events")
	if err != nil {
		return err
	}
	for _, name := range []string{"token_saving", "session_id", "content_tokens_avoided"} {
		if _, exists := columns[name]; exists {
			continue
		}
		if _, err := db.ExecContext(context.Background(), "ALTER TABLE events ADD COLUMN "+name+" "+legacyAdditions[name]); err != nil {
			return fmt.Errorf("migrate telemetry events.%s: %w", name, err)
		}
	}
	return nil
}
