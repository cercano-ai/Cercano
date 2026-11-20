package conversation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Fixture-only candidate: absence of a binding means legacy host execution.
// Do not infer executor/workspace identity or freeze legacy roots from paths.
func bindingFixtureStep() declaredMigration {
	return declaredMigration{CurrentCoexistenceDeclaration(), CoexistenceDeclaration{"test-only-binding-candidate", 3}, []string{`CREATE TABLE execution_bindings (
 conversation_id TEXT PRIMARY KEY REFERENCES conversations(id),
 location TEXT NOT NULL CHECK(location IN ('host','client')),
 executor_id TEXT NOT NULL, workspace_id TEXT NOT NULL, root TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK(generation > 0))`}}
}

func runFixtureUpgrade(t *testing.T, path string, step declaredMigration) error {
	t.Helper()
	db, err := openCoexistenceDB(path, "rw")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err = db.Exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer db.Exec("ROLLBACK")
	if err = upgradeDeclared(context.Background(), db, path, step.to, []declaredMigration{step}); err != nil {
		return err
	}
	_, err = db.Exec("COMMIT")
	return err
}

func TestDeclaredVersionUpgrade(t *testing.T) {
	for _, scenario := range []string{"success", "rollback", "backup-exists", "unsupported", "inconsistent"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			p := offlinePath(t)
			if err := PrepareCoexistence(ctx, p); err != nil {
				t.Fatal(err)
			}
			db, err := openCoexistenceDB(p, "rw")
			if err != nil {
				t.Fatal(err)
			}
			for _, q := range []string{
				`INSERT INTO conversations(id,started_at,last_turn_at,project_dir) VALUES('old',1,2,'/host/project')`,
				`INSERT INTO turns(id,conversation_id,role,content,created_at) VALUES('turn','old','user','saved chat',3)`,
				`INSERT INTO conversation_ownership(conversation_id,owner,epoch,state,destination,handover) VALUES('old','host','epoch','owned','host','')`,
				`INSERT INTO delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at) VALUES('task','old','running','saved','/host/project',4,5)`,
			} {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			step := bindingFixtureStep()
			switch scenario {
			case "rollback":
				step.statements = append(step.statements, "INSERT INTO missing_table VALUES(1)")
			case "backup-exists":
				if err := os.WriteFile(p+".pre-upgrade-backup", []byte("retain"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unsupported":
				step.from.Contract = "not-the-source"
			case "inconsistent":
				if _, err := db.Exec("PRAGMA user_version=99"); err != nil {
					t.Fatal(err)
				}
			}
			db.Close()
			err = runFixtureUpgrade(t, p, step)
			if (err == nil) != (scenario == "success") {
				t.Fatalf("upgrade: %v", err)
			}
			db, err = openCoexistenceDB(p, "ro")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			exists, err := tableExists(db, "execution_bindings")
			if err != nil || exists != (scenario == "success") {
				t.Fatalf("partial schema: %v %v", exists, err)
			}
			var content, owner, status, contract string
			var uv, version int
			if err := db.QueryRow(`SELECT content,owner,status FROM turns JOIN conversation_ownership ON conversation_ownership.conversation_id=turns.conversation_id JOIN delegated_tasks ON parent_id=turns.conversation_id`).Scan(&content, &owner, &status); err != nil {
				t.Fatal(err)
			}
			if content != "saved chat" || owner != "host" || status != "running" {
				t.Fatal(content, owner, status)
			}
			if err := db.QueryRow("SELECT contract,schema_version FROM coexistence_metadata").Scan(&contract, &version); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
				t.Fatal(err)
			}
			if scenario == "success" {
				if contract != step.to.Contract || version != 3 || uv != 3 {
					t.Fatal(contract, version, uv)
				}
				var count int
				if err := db.QueryRow("SELECT count(*) FROM execution_bindings").Scan(&count); err != nil || count != 0 {
					t.Fatalf("legacy host rebound: %d %v", count, err)
				}
				if err := runFixtureUpgrade(t, p, step); err != nil {
					t.Fatalf("repeat: %v", err)
				}
				if err := PreflightCoexistence(ctx, p, CurrentCoexistenceDeclaration()); err == nil {
					t.Fatal("old startup admitted new schema")
				}
				if err := PreflightCoexistence(ctx, p, step.to); err == nil {
					t.Fatal("startup admitted unregistered fixture target")
				}
				if err := UpgradeOffline(ctx, p); err == nil {
					t.Fatal("production adopted fixture")
				}
			} else if contract != CurrentCoexistenceDeclaration().Contract || version != 2 || (uv != 2 && scenario != "inconsistent") {
				t.Fatal(contract, version, uv)
			}
			if scenario == "success" || scenario == "rollback" {
				if err := PreflightCoexistence(ctx, p+".pre-upgrade-backup", CurrentCoexistenceDeclaration()); err != nil {
					t.Fatalf("backup not usable: %v", err)
				}
				backup, err := openCoexistenceDB(p+".pre-upgrade-backup", "ro")
				if err != nil {
					t.Fatal(err)
				}
				defer backup.Close()
				var history string
				if err := backup.QueryRow("SELECT content FROM turns WHERE id='turn'").Scan(&history); err != nil || history != "saved chat" {
					t.Fatalf("backup lost history: %q %v", history, err)
				}
				if exists, err := tableExists(backup, "execution_bindings"); err != nil || exists {
					t.Fatalf("backup not pre-upgrade: %v %v", exists, err)
				}
			} else if scenario == "backup-exists" {
				b, err := os.ReadFile(p + ".pre-upgrade-backup")
				if err != nil || string(b) != "retain" {
					t.Fatal("overwrote backup", err)
				}
			} else if _, err := os.Stat(p + ".pre-upgrade-backup"); !os.IsNotExist(err) {
				t.Fatal("unsupported source created backup", err)
			}
		})
	}
}

func TestOfflineBackupDurabilityBoundary(t *testing.T) {
	p := offlinePath(t)
	if err := PrepareCoexistence(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	var kinds []bool
	err := backupOfflineWithSync(context.Background(), p, func(f *os.File) error {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		kinds = append(kinds, info.IsDir())
		return f.Sync()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 2 || kinds[0] || !kinds[1] {
		t.Fatalf("durability boundary: synced directory flags %v; want [false true] before source writes", kinds)
	}
}

// Keep the writer alive: closing the final source connection would checkpoint
// the WAL and make this regression indistinguishable from a main-file copy.
func TestOfflineUpgradeCommittedWALBackup(t *testing.T) {
	ctx := context.Background()
	p := offlinePath(t)
	if err := prepareCoexistence(ctx, p, developmentMigrations[0].from); err != nil {
		t.Fatal(err)
	}
	writer, err := openCoexistenceDB(p, "rw")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	conn, err := writer.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, q := range []string{
		`PRAGMA journal_mode=WAL`, `PRAGMA wal_autocheckpoint=0`, `BEGIN`,
		`INSERT INTO conversations(id,started_at,last_turn_at,project_dir) VALUES('wal-conversation',1,2,'/fixture')`,
		`INSERT INTO turns(id,conversation_id,role,content,created_at) VALUES('wal-turn','wal-conversation','user','committed WAL history',3)`,
		`INSERT INTO conversation_ownership(conversation_id,owner,epoch,state,destination,handover) VALUES('wal-conversation','host','wal-epoch','owned','host','')`,
		`INSERT INTO delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at) VALUES('wal-task','wal-conversation','running','committed WAL task','/fixture',4,5)`,
		`COMMIT`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(p + "-wal")
	if err != nil || info.Size() <= 32 {
		t.Fatalf("expected nonempty committed WAL: %v %v", info, err)
	}
	if err := UpgradeOffline(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Copy only the backup main file to an isolated directory: no WAL or SHM
	// sidecar from either database can participate in the verification.
	contents, err := os.ReadFile(p + ".pre-upgrade-backup")
	if err != nil {
		t.Fatal(err)
	}
	standalone := filepath.Join(t.TempDir(), "standalone.db")
	if err := os.WriteFile(standalone, contents, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := openCoexistenceDB(standalone, "ro")
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var contract string
	var version, uv int
	if err := backup.QueryRow("SELECT contract,schema_version FROM coexistence_metadata").Scan(&contract, &version); err != nil {
		t.Fatal(err)
	}
	if err := backup.QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
		t.Fatal(err)
	}
	if contract != developmentMigrations[0].from.Contract || version != 2 || uv != 2 {
		t.Fatalf("backup markers: %s %d %d", contract, version, uv)
	}
	var history, epoch, task string
	if err := backup.QueryRow(`SELECT content,epoch,task FROM turns JOIN conversation_ownership ON conversation_ownership.conversation_id=turns.conversation_id JOIN delegated_tasks ON parent_id=turns.conversation_id WHERE turns.id='wal-turn'`).Scan(&history, &epoch, &task); err != nil {
		t.Fatal(err)
	}
	if history != "committed WAL history" || epoch != "wal-epoch" || task != "committed WAL task" {
		t.Fatal(history, epoch, task)
	}
	if exists, err := tableExists(backup, "execution_bindings"); err != nil || exists {
		t.Fatalf("backup not pre-upgrade: %v %v", exists, err)
	}
	if err := PreflightCoexistence(ctx, p, DevelopmentCoexistenceDeclaration()); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineUpgradeSyncFailureBeforeMutation(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			ctx := context.Background()
			p := offlinePath(t)
			if err := prepareCoexistence(ctx, p, developmentMigrations[0].from); err != nil {
				t.Fatal(err)
			}
			db, err := openCoexistenceDB(p, "rw")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec("BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			defer db.Exec("ROLLBACK")
			sentinel := errors.New("injected backup sync failure")
			var calls []bool
			err = upgradeDeclaredWithSync(ctx, db, p, DevelopmentCoexistenceDeclaration(), developmentMigrations, func(f *os.File) error {
				info, err := f.Stat()
				if err != nil {
					return err
				}
				calls = append(calls, info.IsDir())
				if info.IsDir() == directory {
					return sentinel
				}
				return f.Sync()
			})
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected sync error, got %v", err)
			}
			if len(calls) != map[bool]int{false: 1, true: 2}[directory] {
				t.Fatalf("sync order: %v", calls)
			}
			// Inspect inside the still-open transaction, BEFORE rollback: this catches
			// attempted source mutation even if rollback would hide it afterwards.
			if exists, err := tableExists(db, "execution_bindings"); err != nil || exists {
				t.Fatalf("source schema mutated: %v %v", exists, err)
			}
			var contract string
			var version, uv int
			if err := db.QueryRow("SELECT contract,schema_version FROM coexistence_metadata").Scan(&contract, &version); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("PRAGMA user_version").Scan(&uv); err != nil {
				t.Fatal(err)
			}
			if contract != developmentMigrations[0].from.Contract || version != 2 || uv != 2 {
				t.Fatalf("source markers mutated: %s %d %d", contract, version, uv)
			}
		})
	}
}
