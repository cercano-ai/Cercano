package telemetry

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/usage"
	"modernc.org/sqlite"
)

// Pause the real driver after the immutable-source read has established a
// SQLite snapshot. The competing connection then attempts a health write
// before WriteAttempts can perform its first mutation.
type attemptReadGate struct {
	dsn          string
	read         chan struct{}
	resume       chan struct{}
	failRollback bool
	once         sync.Once
}

func (g *attemptReadGate) Driver() driver.Driver { return &sqlite.Driver{} }
func (g *attemptReadGate) Connect(context.Context) (driver.Conn, error) {
	underlying, err := g.Driver().Open(g.dsn)
	if err != nil {
		return nil, err
	}
	return &attemptReadConn{Conn: underlying, gate: g}, nil
}

type attemptReadConn struct {
	driver.Conn
	gate *attemptReadGate
}

func (c *attemptReadConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == "ROLLBACK" && c.gate.failRollback {
		return nil, errors.New("injected rollback failure")
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (c *attemptReadConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, fmt.Errorf("SQLite driver does not support QueryContext")
	}
	rows, err := queryer.QueryContext(ctx, query, args)
	if err == nil && strings.HasPrefix(query, "SELECT requested_provider,requested_model,route_id,source_connection_id,report_identity FROM inference_attempts") {
		c.gate.once.Do(func() {
			close(c.gate.read)
			<-c.gate.resume
		})
	}
	return rows, err
}

func TestWriteAttemptsReservesWriterBeforeSourceRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounting.db")
	initial, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.InitializeAccounting(t.Context()); err != nil {
		t.Fatal(err)
	}
	start := accountingFixture("interleaved")
	if err := initial.WriteAttempts(t.Context(), []usage.AttemptObservation{start}); err != nil {
		t.Fatal(err)
	}
	if err := initial.Close(); err != nil {
		t.Fatal(err)
	}
	gate := &attemptReadGate{dsn: path + "?_pragma=busy_timeout(5000)", read: make(chan struct{}), resume: make(chan struct{})}
	db := sql.OpenDB(gate)
	defer db.Close()
	store := &SQLiteStore{db: db}
	other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(25)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	final := start
	final.Revision = 2
	final.Outcome = usage.Completed
	final.EndedAt = final.StartedAt.Add(time.Second)
	result := make(chan error, 1)
	go func() { result <- store.WriteAttempts(t.Context(), []usage.AttemptObservation{final}) }()
	select {
	case <-gate.read:
	case <-time.After(3 * time.Second):
		close(gate.resume)
		t.Fatal("immutable-source read never reached the driver")
	}
	writeCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	_, competingErr := other.ExecContext(writeCtx, `INSERT INTO accounting_health(writer_id,updated_at,snapshot) VALUES('competing',1,'{}')`)
	cancel()
	close(gate.resume)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("attempt write lost its snapshot after health write: %v (competing write: %v)", err, competingErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attempt writer remained blocked")
	}
	if competingErr == nil {
		t.Fatal("competing health write committed between source read and attempt update")
	}
	stored, err := store.AccountingAttempt(t.Context(), final.ID)
	if err != nil || stored.Revision != final.Revision || stored.SourceConnectionID != start.SourceConnectionID {
		t.Fatalf("same-source revision was not committed: %+v %v", stored, err)
	}
	pending, err := store.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
	if err != nil || len(pending) != 2 {
		t.Fatalf("attempt and outbox did not commit together: %+v %v", pending, err)
	}
}

func TestWriteAttemptsCancellationReleasesWriter(t *testing.T) {
	for _, failRollback := range []bool{false, true} {
		name := "rollback"
		if failRollback {
			name = "discard-after-rollback-error"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "accounting.db")
			initial, err := NewSQLiteStore(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := initial.InitializeAccounting(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := initial.Close(); err != nil {
				t.Fatal(err)
			}
			gate := &attemptReadGate{dsn: path + "?_pragma=busy_timeout(5000)", read: make(chan struct{}), resume: make(chan struct{}), failRollback: failRollback}
			db := sql.OpenDB(gate)
			defer db.Close()
			store := &SQLiteStore{db: db}
			attempt := accountingFixture("canceled")
			ctx, cancel := context.WithCancel(t.Context())
			result := make(chan error, 1)
			go func() { result <- store.WriteAttempts(ctx, []usage.AttemptObservation{attempt}) }()
			select {
			case <-gate.read:
			case <-time.After(3 * time.Second):
				cancel()
				close(gate.resume)
				t.Fatal("immutable-source read never reached the driver")
			}
			cancel()
			close(gate.resume)
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("canceled attempt committed")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("canceled attempt did not finish")
			}
			other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(25)")
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			if _, err := other.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
				t.Fatalf("canceled attempt leaked writer lock: %v", err)
			}
			if _, err := other.ExecContext(t.Context(), "ROLLBACK"); err != nil {
				t.Fatal(err)
			}
			if _, err := store.AccountingAttempt(t.Context(), attempt.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("canceled attempt persisted: %v", err)
			}
			pending, err := store.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
			if err != nil || len(pending) != 0 {
				t.Fatalf("canceled attempt created outbox: %+v %v", pending, err)
			}
			if err := store.WriteAttempts(t.Context(), []usage.AttemptObservation{attempt}); err != nil {
				t.Fatalf("clean write after cancellation: %v", err)
			}
			pending, err = store.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
			if err != nil || len(pending) != 1 {
				t.Fatalf("clean write did not persist attempt and outbox: %+v %v", pending, err)
			}
		})
	}
}

func TestWriteAttemptsOutboxFailureRollsBackAttempt(t *testing.T) {
	store := accountingTestStore(t)
	attempt := accountingFixture("outbox-failure")
	if _, err := store.db.ExecContext(t.Context(), `CREATE TRIGGER reject_outbox BEFORE INSERT ON enterprise_usage_outbox BEGIN SELECT RAISE(ABORT, 'injected outbox failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAttempts(t.Context(), []usage.AttemptObservation{attempt}); err == nil {
		t.Fatal("outbox failure acknowledged attempt")
	}
	if _, err := store.AccountingAttempt(t.Context(), attempt.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("outbox failure left an attempt row: %v", err)
	}
	pending, err := store.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
	if err != nil || len(pending) != 0 {
		t.Fatalf("outbox failure left a report row: %+v %v", pending, err)
	}
	if _, err := store.db.ExecContext(t.Context(), `DROP TRIGGER reject_outbox`); err != nil {
		t.Fatalf("failed transaction kept writer lock: %v", err)
	}
	if err := store.WriteAttempts(t.Context(), []usage.AttemptObservation{attempt}); err != nil {
		t.Fatalf("clean retry after rollback: %v", err)
	}
	pending, err = store.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
	if err != nil || len(pending) != 1 {
		t.Fatalf("retry did not atomically persist attempt and outbox: %+v %v", pending, err)
	}
}
