package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCoexistencePreparedRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fixture ?#.db")
	mustCoexist(t, PrepareCoexistence(ctx, path))
	d := CurrentCoexistenceDeclaration()
	mustCoexist(t, PreflightCoexistence(ctx, path, d))
	a, err := OpenCoexistence(ctx, path, d)
	mustCoexist(t, err)
	defer a.Close()
	owner, err := AcquireOwnership(ctx, a, "conversation", "A")
	mustCoexist(t, err)
	defer owner.Close()
	ctx = WithOwnership(ctx, owner)
	mustCoexist(t, a.EnsureConversation(ctx, "conversation", "/synthetic", "fixture-model"))
	blocks := `[{"type":"tool_use","id":"call","name":"fixture","input":{"x":1}},{"type":"text","text":"result"}]`
	mustCoexist(t, a.Append(ctx, Turn{ID: "turn", ConversationID: "conversation", Role: "assistant", Content: "result", BlocksJSON: blocks}))
	ta := a.(TaskStore)
	now := time.Now()
	for _, id := range []string{"done", "live"} {
		mustCoexist(t, ta.InsertTask(ctx, TaskRecord{ID: id, ParentID: "conversation", Status: "accepted", Task: "synthetic", CreatedAt: now, UpdatedAt: now}))
		mustCoexist(t, ta.StartTask(ctx, "conversation", id))
	}
	result := json.RawMessage(`{"output":"fixture","partial":false}`)
	ok, err := ta.FinishTask(ctx, TaskRecord{ID: "done", ParentID: "conversation", Status: "completed", ResultJSON: result})
	mustCoexist(t, err)
	if !ok {
		t.Fatal("outcome not committed")
	}
	ok, err = ta.ClaimTaskWake(ctx, "conversation", "done")
	mustCoexist(t, err)
	if !ok {
		t.Fatal("wake not claimed")
	}
	b, err := OpenCoexistence(ctx, path, d)
	mustCoexist(t, err)
	defer b.Close()
	turns, err := b.GetTurns(ctx, "conversation")
	mustCoexist(t, err)
	if len(turns) != 1 || turns[0].BlocksJSON != blocks {
		t.Fatalf("blocks: %+v", turns)
	}
	tb := b.(TaskStore)
	task, err := tb.GetTask(ctx, "conversation", "done")
	mustCoexist(t, err)
	if task.Status != "completed" || string(task.ResultJSON) != string(result) || !task.NotificationPending || !task.WakeClaimed {
		t.Fatalf("outcome: %+v", task)
	}
	live, err := tb.GetTask(ctx, "conversation", "live")
	mustCoexist(t, err)
	if live.Status != "running" {
		t.Fatalf("startup reconciled task: %+v", live)
	}
	mustCoexist(t, ta.HandleTaskNotification(ctx, "conversation", "done"))
	task, err = ta.GetTask(ctx, "conversation", "done")
	mustCoexist(t, err)
	if task.NotificationPending {
		t.Fatal("notification update did not roundtrip")
	}
}

func TestCoexistenceMissingPathNotCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	ctx := context.Background()
	if err := PreflightCoexistence(ctx, path, CurrentCoexistenceDeclaration()); err == nil {
		t.Fatal("preflight accepted missing path")
	}
	if s, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration()); err == nil {
		s.Close()
		t.Fatal("open accepted missing path")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	mustCoexist(t, err)
	if len(entries) != 0 {
		t.Fatalf("created files: %v", entries)
	}
}

func TestPrepareCoexistenceRefusesExisting(t *testing.T) {
	for _, kind := range []string{"empty", "database", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "existing.db")
			switch kind {
			case "empty":
				mustCoexist(t, os.WriteFile(path, nil, 0600))
			case "database":
				s, err := Open(path)
				mustCoexist(t, err)
				mustCoexist(t, s.Close())
			case "symlink":
				mustCoexist(t, os.Symlink("absent-target", path))
			}
			before, err := os.Lstat(path)
			mustCoexist(t, err)
			if err := PrepareCoexistence(context.Background(), path); err == nil {
				t.Fatal("adopted existing destination")
			}
			after, err := os.Lstat(path)
			mustCoexist(t, err)
			if before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("changed existing destination")
			}
		})
	}
}

func TestCoexistenceRefusalDoesNotMutate(t *testing.T) {
	cases := []struct {
		name, mutation string
		declaration    CoexistenceDeclaration
	}{
		{"missing_marker", "DROP TABLE coexistence_metadata", CurrentCoexistenceDeclaration()},
		{"representation_same_schema", "UPDATE coexistence_metadata SET contract='different-representation'", CurrentCoexistenceDeclaration()},
		{"ownership_same_schema", "UPDATE coexistence_metadata SET contract='different-ownership'", CurrentCoexistenceDeclaration()},
		{"marker_schema", "UPDATE coexistence_metadata SET schema_version=999", CurrentCoexistenceDeclaration()},
		{"sqlite_schema_version", "PRAGMA user_version=999", CurrentCoexistenceDeclaration()},
		{"missing_marker_row", "DELETE FROM coexistence_metadata", CurrentCoexistenceDeclaration()},
		{"unknown_build", "", CoexistenceDeclaration{"unknown", 1}},
		{"unknown_build_schema", "", CoexistenceDeclaration{CurrentCoexistenceDeclaration().Contract, 999}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "fixture.db")
			mustCoexist(t, PrepareCoexistence(ctx, path))
			db, err := openCoexistenceDB(path, "rw")
			mustCoexist(t, err)
			// Legacy snapshot would be copied by normal Open. An unfinished task would
			// be interrupted by reconciliation. DELETE journal mode exposes pragma writes.
			_, err = db.Exec(`INSERT INTO conversations(id,started_at,last_turn_at) VALUES('c',1,1);
 INSERT INTO conversation_context_usage(conversation_id,source,tokens_used) VALUES('c','turn',123);
 INSERT INTO delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at) VALUES('live','c','running','fixture','',1,1);
 PRAGMA journal_mode=DELETE; PRAGMA application_id=12345;`)
			mustCoexist(t, err)
			if tc.mutation != "" {
				_, err = db.Exec(tc.mutation)
				mustCoexist(t, err)
			}
			mustCoexist(t, db.Close())
			before := coexistenceSnapshot(t, path)
			if err := PreflightCoexistence(ctx, path, tc.declaration); err == nil {
				t.Fatal("preflight accepted mismatch")
			}
			if s, err := OpenCoexistence(ctx, path, tc.declaration); err == nil {
				s.Close()
				t.Fatal("open accepted mismatch")
			}
			after := coexistenceSnapshot(t, path)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("refusal mutated store\nbefore %v\nafter %v", before, after)
			}
		})
	}
}

// Capture all logical tables, schema SQL and persistent SQLite metadata, not
// filesystem bytes whose WAL/checkpoint layout is not a logical mutation.
func coexistenceSnapshot(t *testing.T, path string) map[string]string {
	t.Helper()
	db, err := openCoexistenceDB(path, "ro")
	mustCoexist(t, err)
	defer db.Close()
	out := map[string]string{}
	queries := map[string]string{"schema": "SELECT type,name,tbl_name,rootpage,sql FROM sqlite_master ORDER BY name"}
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	mustCoexist(t, err)
	for rows.Next() {
		var name string
		mustCoexist(t, rows.Scan(&name))
		queries[name] = `SELECT * FROM "` + name + `" ORDER BY rowid`
	}
	mustCoexist(t, rows.Err())
	mustCoexist(t, rows.Close())
	for _, p := range []string{"user_version", "schema_version", "application_id", "journal_mode", "auto_vacuum", "page_size"} {
		queries["pragma_"+p] = "PRAGMA " + p
	}
	for name, q := range queries {
		out[name] = coexistenceQuerySnapshot(t, db, q)
	}
	return out
}
func coexistenceQuerySnapshot(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	rows, err := db.Query(q)
	mustCoexist(t, err)
	defer rows.Close()
	cols, err := rows.Columns()
	mustCoexist(t, err)
	all := [][]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptr := make([]any, len(cols))
		for i := range values {
			ptr[i] = &values[i]
		}
		mustCoexist(t, rows.Scan(ptr...))
		all = append(all, values)
	}
	mustCoexist(t, rows.Err())
	data, err := json.Marshal(all)
	mustCoexist(t, err)
	return string(data)
}
func mustCoexist(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
