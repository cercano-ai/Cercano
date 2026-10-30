package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSchemaReviewRejectsNonRowidPrimaryKey(t *testing.T) {
	for _, table := range []string{"events", "cloud_usage"} {
		for _, layout := range []string{"without-rowid", "descending"} {
			t.Run(table+"/"+layout, func(t *testing.T) {
				path, db := legacySchemaFixture(t)
				var ddl string
				if err := db.QueryRow(`SELECT sql FROM sqlite_schema WHERE name=?`, table).Scan(&ddl); err != nil {
					t.Fatal(err)
				}
				ddl = strings.Replace(ddl, "INTEGER PRIMARY KEY AUTOINCREMENT", "INTEGER PRIMARY KEY", 1)
				if layout == "without-rowid" {
					ddl += " WITHOUT ROWID"
				} else {
					ddl = strings.Replace(ddl, "INTEGER PRIMARY KEY", "INTEGER PRIMARY KEY DESC", 1)
				}
				if _, err := db.Exec("DROP TABLE " + table + ";" + ddl); err != nil {
					t.Fatal(err)
				}
				before := schemaSQL(t, db)
				store, err := NewSQLiteStore(path)
				if store != nil {
					defer store.Close()
					if table == "events" {
						err = store.RecordEvent(context.Background(), NewEvent("probe", "fake"))
					} else {
						err = store.RecordCloudUsage(context.Background(), CloudUsageReport{})
					}
					var nullIDs int
					if e := db.QueryRow("SELECT count(*) FROM " + table + " WHERE id IS NULL").Scan(&nullIDs); e != nil {
						t.Fatal(e)
					}
					t.Fatalf("constructor accepted non-rowid PK: omit-id write error=%v null IDs=%d", err, nullIDs)
				}
				if err == nil {
					t.Fatal("expected constructor rejection")
				}
				if schemaSQL(t, db) != before {
					t.Fatal("rejection mutated schema")
				}
			})
		}
	}
}

func TestSchemaReviewRejectsRequiredNullDefault(t *testing.T) {
	for _, def := range []string{"NULL", "(NULL)", "(nullif(1,1))"} {
		t.Run(def, func(t *testing.T) {
			path, db := legacySchemaFixture(t)
			if _, err := db.Exec(`DROP TABLE sessions; CREATE TABLE sessions(session_id TEXT PRIMARY KEY, started_at DATETIME NOT NULL, future TEXT NOT NULL DEFAULT ` + def + `)`); err != nil {
				t.Fatal(err)
			}
			before := schemaSQL(t, db)
			store, err := NewSQLiteStore(path)
			if store != nil {
				defer store.Close()
				writeErr := store.RecordSession(context.Background(), "probe")
				var count int
				if err := db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				t.Fatalf("constructor accepted required NULL default: session write error=%v rows=%d", writeErr, count)
			}
			if err == nil {
				t.Fatal("expected constructor rejection")
			}
			if schemaSQL(t, db) != before {
				t.Fatal("rejection mutated schema")
			}
		})
	}
}

func TestSchemaReviewAllAdditiveLayouts(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		t.Run(fmt.Sprint(mask), func(t *testing.T) {
			path, db := legacySchemaFixture(t)
			// Historical fixture starts with all three additions absent.
			if _, err := db.Exec(`ALTER TABLE events ADD COLUMN token_saving BOOLEAN NOT NULL DEFAULT 1; ALTER TABLE events ADD COLUMN session_id TEXT NOT NULL DEFAULT ''; ALTER TABLE events ADD COLUMN content_tokens_avoided INTEGER NOT NULL DEFAULT 0`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE events SET token_saving=0, session_id='retained', content_tokens_avoided=29`); err != nil {
				t.Fatal(err)
			}
			for bit, name := range []string{"token_saving", "session_id", "content_tokens_avoided"} {
				if mask&(1<<bit) == 0 {
					if _, err := db.Exec("ALTER TABLE events DROP COLUMN " + name); err != nil {
						t.Fatal(err)
					}
				}
			}
			for reopen := 0; reopen < 2; reopen++ {
				store, err := NewSQLiteStore(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.RecordSession(context.Background(), "new-session"); err != nil {
					t.Fatal(err)
				}
				store.Close()
			}
			var tokens, saving, avoided int
			var session string
			if err := db.QueryRow(`SELECT input_tokens,token_saving,session_id,content_tokens_avoided FROM events`).Scan(&tokens, &saving, &session, &avoided); err != nil {
				t.Fatal(err)
			}
			wantSaving, wantSession, wantAvoided := 1, "", 0
			if mask&1 != 0 {
				wantSaving = 0
			}
			if mask&2 != 0 {
				wantSession = "retained"
			}
			if mask&4 != 0 {
				wantAvoided = 29
			}
			if tokens != 42 || saving != wantSaving || session != wantSession || avoided != wantAvoided {
				t.Fatal("legacy values/defaults changed")
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&count); err != nil || count != 2 {
				t.Fatalf("sessions: %d %v", count, err)
			}
		})
	}
}

func TestSchemaReviewOldestNoSessions(t *testing.T) {
	path, db := legacySchemaFixture(t)
	if _, err := db.Exec(`DROP TABLE sessions`); err != nil {
		t.Fatal(err)
	}
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RecordSession(context.Background(), "first"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordEvent(context.Background(), NewEvent("new", "fake")); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM events`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("events: %d %v", count, err)
	}
}

// Inject only at the transaction's schemaConnection boundary, never globally.
type failSecondAlter struct {
	schemaConnection
	alters  *int
	failure error
}

func (c failSecondAlter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if strings.HasPrefix(query, "ALTER TABLE") {
		*c.alters++
		if *c.alters == 2 {
			return nil, c.failure
		}
	}
	return c.schemaConnection.ExecContext(ctx, query, args...)
}

func TestSchemaReviewRollbackAfterSuccessfulAlter(t *testing.T) {
	path, db := legacySchemaFixture(t)
	if _, err := db.Exec(`DROP TABLE sessions`); err != nil {
		t.Fatal(err)
	}
	before := schemaSQL(t, db)
	failure := errors.New("second ALTER failure")
	alters := 0
	err := initializeLegacySchemaWith(db, func(c schemaConnection) schemaConnection { return failSecondAlter{c, &alters, failure} })
	if !errors.Is(err, failure) || alters != 2 {
		t.Fatalf("fault not reached after successful ALTER: %d %v", alters, err)
	}
	if schemaSQL(t, db) != before {
		t.Fatal("transaction left partial schema upgrades")
	}
	var tokens int
	if err := db.QueryRow(`SELECT input_tokens FROM events`).Scan(&tokens); err != nil || tokens != 42 {
		t.Fatalf("lost legacy data: %d %v", tokens, err)
	}
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
}

func TestSchemaReviewCompatiblePrimaryKeysAndDefaults(t *testing.T) {
	for _, pk := range []string{"INTEGER PRIMARY KEY", "INTEGER PRIMARY KEY ASC", "INTEGER, PRIMARY KEY(id DESC)"} {
		t.Run(pk, func(t *testing.T) {
			path, db := legacySchemaFixture(t)
			var ddl string
			if err := db.QueryRow(`SELECT sql FROM sqlite_schema WHERE name='cloud_usage'`).Scan(&ddl); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(pk, ",") {
				ddl = strings.Replace(ddl, "INTEGER PRIMARY KEY AUTOINCREMENT", "INTEGER", 1)
				end := strings.LastIndex(ddl, ")")
				ddl = ddl[:end] + ", PRIMARY KEY(id DESC)" + ddl[end:]
			} else {
				ddl = strings.Replace(ddl, "INTEGER PRIMARY KEY AUTOINCREMENT", pk, 1)
			}
			if _, err := db.Exec("DROP TABLE cloud_usage;" + ddl + `; ALTER TABLE sessions ADD COLUMN future TEXT NOT NULL DEFAULT ('ok'); INSERT INTO sessions(session_id,started_at) VALUES(NULL,'2026-10-27')`); err != nil {
				t.Fatal(err)
			}
			store, err := NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.RecordCloudUsage(context.Background(), CloudUsageReport{}); err != nil {
				t.Fatal(err)
			}
			if err := store.RecordSession(context.Background(), "new"); err != nil {
				t.Fatal(err)
			}
			var id, nullSessions int
			if err := db.QueryRow(`SELECT id FROM cloud_usage`).Scan(&id); err != nil || id != 1 {
				t.Fatalf("rowid not allocated: %d %v", id, err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM sessions WHERE session_id IS NULL`).Scan(&nullSessions); err != nil || nullSessions != 1 {
				t.Fatalf("historical nullable TEXT PK lost: %d %v", nullSessions, err)
			}
		})
	}
}

func TestSchemaReviewNonNullDefaultClassification(t *testing.T) {
	for _, value := range []string{"''", "'can''t'", "('ok')", "((1))", "-1.2e+3", "0xFF", "X'00ff'", "TRUE", "CURRENT_TIMESTAMP"} {
		if !nonNullDefault(value) {
			t.Errorf("rejected safe default %s", value)
		}
	}
	for _, value := range []string{"NULL", "((null))", "(nullif(1,1))", "(random())", "('x' || NULL)", "('a') || ('b')"} {
		if nonNullDefault(value) {
			t.Errorf("accepted unproven default %s", value)
		}
	}
}
