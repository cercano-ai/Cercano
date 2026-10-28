package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSchemaOpenRejectsView(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE VIEW events AS SELECT 1 AS id`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	store, err := NewSQLiteStore(path)
	if store != nil {
		store.Close()
	}
	if err == nil {
		t.Fatal("constructor accepted a view: ALTER failures masked and required event columns absent")
	}
}

func legacySchemaFixture(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := createTables(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE events DROP COLUMN token_saving;
 INSERT INTO events(timestamp,tool_name,model,input_tokens) VALUES('2026-10-27','fixture','fake',42);
 INSERT INTO cloud_usage(timestamp,cloud_input_tokens) VALUES('2026-10-27',17);
 INSERT INTO sessions VALUES('fixture','2026-10-27')`); err != nil {
		t.Fatal(err)
	}
	return path, db
}

func TestSchemaOpenConcurrentLegacyPreservation(t *testing.T) {
	path, db := legacySchemaFixture(t)
	const n = 8
	start := make(chan struct{})
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			store, err := NewSQLiteStore(path)
			if err == nil {
				err = store.InitializeAccounting(context.Background())
				store.Close()
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id, tokens, saving, avoided int
	var session string
	if err := db.QueryRow(`SELECT id,input_tokens,token_saving,session_id,content_tokens_avoided FROM events`).Scan(&id, &tokens, &saving, &session, &avoided); err != nil {
		t.Fatal(err)
	}
	if id != 1 || tokens != 42 || saving != 1 || session != "" || avoided != 0 {
		t.Fatalf("legacy row changed: %d %d %d %q %d", id, tokens, saving, session, avoided)
	}
	var cloud, sessions int
	if err := db.QueryRow(`SELECT cloud_input_tokens FROM cloud_usage`).Scan(&cloud); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sessions WHERE session_id='fixture'`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if cloud != 17 || sessions != 1 {
		t.Fatal("legacy cloud/session data changed")
	}
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RecordEvent(context.Background(), NewEvent("new", "fake")); err != nil {
		t.Fatal(err)
	}
}

// WAL admission precedes validation; only schema SQL and stored data, not
// journal mode or filesystem bytes, are promised unchanged on rejection.
func TestSchemaOpenRejectsMalformedPreservesSchemaAndData(t *testing.T) {
	for _, ddl := range []string{
		`ALTER TABLE events DROP COLUMN model`,
		`ALTER TABLE events ADD COLUMN session_id INTEGER NOT NULL DEFAULT 0`,
		`DROP TABLE sessions; CREATE TABLE sessions(session_id TEXT, started_at DATETIME NOT NULL)`,
		`ALTER TABLE cloud_usage ADD COLUMN required TEXT NOT NULL`,
	} {
		t.Run(ddl, func(t *testing.T) {
			path, db := legacySchemaFixture(t)
			// A required unknown column must be installed before rows exist.
			if strings.Contains(ddl, "required") {
				ddl = `DROP TABLE cloud_usage; CREATE TABLE cloud_usage(id INTEGER PRIMARY KEY, timestamp DATETIME NOT NULL, required TEXT NOT NULL)`
			}
			if _, err := db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			before := schemaSQL(t, db)
			store, err := NewSQLiteStore(path)
			if store != nil {
				store.Close()
			}
			if err == nil {
				t.Fatal("accepted incompatible schema")
			}
			if after := schemaSQL(t, db); after != before {
				t.Fatal("failed open mutated schema")
			}
			var tokens int
			if err := db.QueryRow(`SELECT input_tokens FROM events`).Scan(&tokens); err != nil || tokens != 42 {
				t.Fatalf("lost event: %d %v", tokens, err)
			}
		})
	}
}

func schemaSQL(t *testing.T, db *sql.DB) string {
	t.Helper()
	var result string
	if err := db.QueryRow(`SELECT group_concat(sql, ';') FROM (SELECT sql FROM sqlite_schema ORDER BY name)`).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

type failingSchemaConnection struct {
	schemaConnection
	failure error
}

func (c failingSchemaConnection) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if strings.HasPrefix(query, "ALTER TABLE") {
		return nil, c.failure
	}
	return c.schemaConnection.ExecContext(ctx, query, args...)
}

func TestSchemaMigrationPropagatesFailure(t *testing.T) {
	_, db := legacySchemaFixture(t)
	failure := errors.New("injected ALTER failure")
	err := migrateSchema(failingSchemaConnection{db, failure})
	if !errors.Is(err, failure) || !strings.Contains(err.Error(), "events.token_saving") {
		t.Fatalf("lost migration error: %v", err)
	}
	// A real SQLite read-only rejection must also be returned, not ignored.
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "PRAGMA query_only=OFF")
	if err := migrateSchema(conn); err == nil {
		t.Fatal("ignored read-only migration failure")
	}
}

func TestSchemaOpenConcurrentFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.db")
	start := make(chan struct{})
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			<-start
			store, err := NewSQLiteStore(path)
			if store != nil {
				store.Close()
			}
			errs <- err
		}()
	}
	close(start)
	for i := 0; i < 8; i++ {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestSchemaOpenCurrentCompatibleExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`ALTER TABLE events ADD COLUMN future_note TEXT; INSERT INTO events(timestamp,tool_name,model,future_note) VALUES('2026-10-27','fixture','fake','preserve')`); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	store, err = NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var note string
	if err := store.db.QueryRow(`SELECT future_note FROM events`).Scan(&note); err != nil || note != "preserve" {
		t.Fatalf("extension lost: %q %v", note, err)
	}
	if err := store.RecordEvent(context.Background(), NewEvent("new", "fake")); err != nil {
		t.Fatal(err)
	}
}

func TestWALAdmissionRetriesOnlyBusyAndIsBounded(t *testing.T) {
	path, db := legacySchemaFixture(t)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	_, busy := other.Exec("PRAGMA journal_mode=WAL")
	if busy == nil {
		t.Fatal("expected competing writer to block WAL admission")
	}
	attempts := 0
	if err := admitWAL(context.Background(), func(context.Context) error {
		attempts++
		if attempts == 1 {
			return busy
		}
		return nil
	}); err != nil || attempts != 2 {
		t.Fatalf("busy was not retried: attempts=%d err=%v", attempts, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := admitWAL(ctx, func(context.Context) error { return busy }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retry did not expire: %v", err)
	}
	_, nonBusy := other.Exec("SELECT * FROM nonexistent_table")
	if nonBusy == nil {
		t.Fatal("expected SQL error")
	}
	for _, failure := range []error{nonBusy, errors.New("connection failed")} {
		attempts = 0
		err := admitWAL(context.Background(), func(context.Context) error { attempts++; return failure })
		if !errors.Is(err, failure) || attempts != 1 {
			t.Fatalf("masked/retried non-BUSY: attempts=%d err=%v", attempts, err)
		}
	}
}

func TestSchemaOpenConstructorIOErrorVisible(t *testing.T) {
	// A directory cannot be opened as a SQLite database.
	store, err := NewSQLiteStore(t.TempDir())
	if store != nil {
		store.Close()
		t.Fatal("returned store on failed open")
	}
	if err == nil || !strings.Contains(err.Error(), "failed to enable WAL mode") {
		t.Fatalf("lost constructor error: %v", err)
	}
}
