package conversation

import (
	"context"
	"fmt"
)

// UpgradeOffline adds the generation declaration and ownership tables in place.
// ALL legacy agents must already be stopped and remain stopped. This API cannot
// fence historical/custom writers and does not reconcile or replay tasks.
func UpgradeOffline(ctx context.Context, path string) error {
	return upgradeOffline(ctx, path, false)
}

// UpgradeInterruptedOffline recovers accepted/running work only after callers
// have stopped writers and saved a backup. It never replays tasks.
func UpgradeInterruptedOffline(ctx context.Context, path string) error {
	return upgradeOffline(ctx, path, true)
}

func upgradeOffline(ctx context.Context, path string, interrupted bool) error {
	db, err := openCoexistenceDB(path, "rw") // never create a missing destination
	if err != nil {
		return err
	}
	defer db.Close()
	// One pinned connection, one SQLite writer admission, and a marker recheck.
	if _, err = db.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer db.Exec("ROLLBACK")
	marked, err := tableExists(db, "coexistence_metadata")
	if err != nil {
		return err
	}
	d := DevelopmentCoexistenceDeclaration()
	if marked {
		var contract string
		var version, count, uv int
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM coexistence_metadata").Scan(&count); err != nil {
			return err
		}
		if err = db.QueryRowContext(ctx, "SELECT contract,schema_version FROM coexistence_metadata WHERE singleton=1").Scan(&contract, &version); err != nil {
			return err
		}
		if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&uv); err != nil {
			return err
		}
		if count != 1 || contract != d.Contract || version != d.SchemaVersion || uv != d.SchemaVersion {
			return fmt.Errorf("offline upgrade refuses incompatible marker")
		}
		return nil // no ownership reset, including on repeated invocation
	}
	var uv int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&uv); err != nil {
		return err
	}
	if uv != 0 {
		return fmt.Errorf("offline upgrade refuses unknown user_version %d", uv)
	}
	exists, err := tableExists(db, "conversations")
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("offline upgrade requires an existing conversation database")
	}
	if err = initializeConversationSchema(db); err != nil {
		return err
	}
	s := &sqliteStore{db: db}
	if err = s.InitTasks(ctx); err != nil {
		return err
	}
	if interrupted {
		if err = s.ReconcileTasks(ctx); err != nil {
			return err
		}
	}
	if _, err = db.ExecContext(ctx, ownershipSchema); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, `CREATE TABLE coexistence_metadata (singleton INTEGER PRIMARY KEY CHECK(singleton=1), contract TEXT NOT NULL, schema_version INTEGER NOT NULL); INSERT INTO coexistence_metadata VALUES(1,?,?);`, d.Contract, d.SchemaVersion); err != nil {
		return err
	}
	if _, err = db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", d.SchemaVersion)); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "COMMIT")
	return err
}
