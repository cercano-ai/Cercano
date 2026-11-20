package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionBindingUpgrade(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "old.db")
	old := CoexistenceDeclaration{"cercano-development-owned-v2", 2}
	fixture, err := os.ReadFile("testdata/production-v2.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := openCoexistenceDB(p, "rwc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(fixture)); err != nil {
		t.Fatal(err)
	}
	before := bindingUpgradeSnapshot(t, db)
	db.Close()
	if err := PreflightCoexistence(ctx, p, DevelopmentCoexistenceDeclaration()); err == nil || !strings.Contains(err.Error(), "upgrade-store") {
		t.Fatalf("old startup: %v", err)
	}
	if err := UpgradeOffline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := UpgradeOffline(ctx, p); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if err := PreflightCoexistence(ctx, p, DevelopmentCoexistenceDeclaration()); err != nil {
		t.Fatal(err)
	}
	db, err = openCoexistenceDB(p, "rw")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if after := bindingUpgradeSnapshot(t, db); after != before {
		t.Fatalf("preservation mismatch\nbefore %s\nafter %s", before, after)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM execution_bindings`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("backfill: %d %v", count, err)
	}
	var root string
	if err := db.QueryRow(`SELECT dev_work_dir FROM conversations WHERE id='legacy'`).Scan(&root); err != nil || root != "/development" {
		t.Fatalf("legacy history: %q %v", root, err)
	}
	backup, err := openCoexistenceDB(p+".pre-upgrade-backup", "ro")
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if got := bindingUpgradeSnapshot(t, backup); got != before {
		t.Fatal("backup did not preserve pre-upgrade rows")
	}
	var backupVersion int
	if err := backup.QueryRow("PRAGMA user_version").Scan(&backupVersion); err != nil || backupVersion != 2 {
		t.Fatalf("backup version %d %v", backupVersion, err)
	}
	if exists, err := tableExists(backup, "execution_bindings"); err != nil || exists {
		t.Fatalf("backup schema: %v %v", exists, err)
	}
	var contract string
	if err := backup.QueryRow(`SELECT contract FROM coexistence_metadata`).Scan(&contract); err != nil || contract != old.Contract {
		t.Fatalf("backup marker: %s %v", contract, err)
	}
	for _, values := range []string{
		`('missing','client','e','w','/root',1)`,
		`('legacy','other','e','w','/root',1)`,
		`('legacy','client','','w','/root',1)`,
		`('legacy','client','e',' ','/root',1)`,
		`('legacy','client','e','w','',1)`,
		`('legacy','client','e','w','/root',0)`,
		`('legacy','client','e','w','/root',1.5)`,
	} {
		if _, err := db.Exec(`INSERT INTO execution_bindings VALUES ` + values); err == nil {
			t.Errorf("accepted malformed %s", values)
		}
	}
	if _, err := db.Exec(`INSERT INTO execution_bindings VALUES('legacy','client','e','w','C:\workspace',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM conversations WHERE id='legacy'`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM execution_bindings`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade: %d %v", count, err)
	}
}

func TestBindingSchemaCreationAndLegacyBackup(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"ordinary", "development", "fixture"} {
		t.Run(kind, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(dir, "store.db")
			switch kind {
			case "ordinary":
				s, err := Open(p)
				if err != nil {
					t.Fatal(err)
				}
				s.Close()
				s, err = Open(p)
				if err != nil {
					t.Fatalf("ordinary reopen: %v", err)
				}
				s.Close()
			case "development":
				if err := PrepareDevelopmentStore(ctx, p); err != nil {
					t.Fatal(err)
				}
			case "fixture":
				if err := PrepareCoexistence(ctx, p); err != nil {
					t.Fatal(err)
				}
			}
			db, err := openCoexistenceDB(p, "ro")
			if err != nil {
				t.Fatal(err)
			}
			exists, err := tableExists(db, "execution_bindings")
			db.Close()
			if err != nil || exists != (kind != "fixture") {
				t.Fatalf("schema: %v %v", exists, err)
			}
			if kind == "ordinary" {
				// A reserved backup must fail closed even through the public API.
				if err := os.WriteFile(p+".pre-upgrade-backup", []byte("reserved"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := UpgradeOffline(ctx, p); err == nil {
					t.Fatal("overwrote backup")
				}
				if err := os.Remove(p + ".pre-upgrade-backup"); err != nil {
					t.Fatal(err)
				}
				if err := UpgradeOffline(ctx, p); err != nil {
					t.Fatal(err)
				}
				backup, err := openCoexistenceDB(p+".pre-upgrade-backup", "ro")
				if err != nil {
					t.Fatal(err)
				}
				defer backup.Close()
				if marked, err := tableExists(backup, "coexistence_metadata"); err != nil || marked {
					t.Fatalf("legacy backup mutated: %v %v", marked, err)
				}
			}
		})
	}
}

func TestRejectIncompatibleExistingBindings(t *testing.T) {
	for _, mode := range []string{"open", "adopt"} {
		for _, ddl := range []string{
			"CREATE TABLE execution_bindings(dummy TEXT)",
			strings.Replace(executionBindingsSchema, " REFERENCES conversations(id) ON DELETE CASCADE", "", 1),
			strings.Replace(executionBindingsSchema, " CHECK(location IN ('host', 'client'))", "", 1),
			strings.Replace(executionBindingsSchema, " PRIMARY KEY", "", 1),
			strings.Replace(executionBindingsSchema, " NOT NULL", "", 1),
			strings.Replace(executionBindingsSchema, " CHECK(typeof(ownership_generation) = 'integer' AND ownership_generation > 0)", "", 1),
			strings.Replace(executionBindingsSchema, " CHECK(typeof(workspace_root) = 'text' AND length(trim(workspace_root)) > 0)", "", 1),
		} {
			t.Run(mode+ddl, func(t *testing.T) {
				p := filepath.Join(t.TempDir(), "bad.db")
				store, err := openConversationStore(p, false)
				if err != nil {
					t.Fatal(err)
				}
				store.Close()
				db, err := openCoexistenceDB(p, "rw")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec("INSERT INTO conversations(id, started_at, last_turn_at) VALUES('kept',1,1);" + ddl); err != nil {
					t.Fatal(err)
				}
				db.Close()
				before, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "open" {
					var s Store
					s, err = Open(p)
					if s != nil {
						s.Close()
					}
				} else {
					err = UpgradeOffline(context.Background(), p)
				}
				if err == nil || !strings.Contains(err.Error(), "execution_bindings") {
					t.Fatalf("expected binding schema refusal, got %v", err)
				}
				after, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) {
					t.Fatal("refusal mutated database")
				}
			})
		}
	}
}

// Snapshot every column, not just status: ownership epochs, wake flags, roots,
// turn payloads and timestamps must survive byte-for-byte at the SQL value level.
func bindingUpgradeSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	var all [][]any
	for _, table := range []string{"conversations", "turns", "delegated_tasks", "conversation_ownership", "conversation_handovers"} {
		rows, err := db.Query("SELECT * FROM " + table + " ORDER BY 1")
		if err != nil {
			t.Fatal(err)
		}
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			all = append(all, vals)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	b, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
