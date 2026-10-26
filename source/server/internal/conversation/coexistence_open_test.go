package conversation

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenCoexistenceDoesNotInitializeOrMigrate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "prepared.db")
	mustCoexist(t, PrepareCoexistence(ctx, path))
	db, err := openCoexistenceDB(path, "rw")
	mustCoexist(t, err)
	_, err = db.Exec(`INSERT INTO conversations(id,started_at,last_turn_at) VALUES('c',1,1);
 INSERT INTO conversation_context_usage(conversation_id,source,tokens_used) VALUES('c','turn',123);
 PRAGMA journal_mode=DELETE;`)
	mustCoexist(t, err)
	mustCoexist(t, db.Close())
	before := coexistenceSnapshot(t, path)
	s, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	var foreignKeys int
	mustCoexist(t, s.(*sqliteStore).db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys))
	if foreignKeys != 1 {
		t.Fatal("foreign key enforcement missing")
	}
	mustCoexist(t, s.Close())
	if after := coexistenceSnapshot(t, path); !reflect.DeepEqual(before, after) {
		t.Fatalf("matching open mutated store: before=%v after=%v", before, after)
	}
}
