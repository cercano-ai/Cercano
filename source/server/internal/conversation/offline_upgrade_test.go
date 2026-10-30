package conversation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func offlinePath(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "history.db")
}
func TestOfflineMarkedOpenRefuses(t *testing.T) {
	p := offlinePath(t)
	if err := PrepareDevelopmentStore(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	s, err := Open(p)
	if err == nil {
		s.Close()
		t.Fatal("ordinary Open bypassed marked-store ownership")
	}
}
func TestOfflineUpgradePreservesAndRepeats(t *testing.T) {
	ctx := context.Background()
	p := offlinePath(t)
	raw, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(*sqliteStore)
	if err = s.InitTasks(ctx); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO conversations(id,started_at,last_turn_at) VALUES('original',1,2)`,
		`INSERT INTO turns(id,conversation_id,role,content,created_at) VALUES('turn','original','user','original history',3)`,
		`INSERT INTO delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at) VALUES('task','original','running','preserve','',4,5)`,
	} {
		if _, err = s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	if err = PreflightCoexistence(ctx, p, DevelopmentCoexistenceDeclaration()); err == nil || !strings.Contains(err.Error(), "cercano upgrade-store --database") {
		t.Fatalf("missing actionable refusal: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err = UpgradeOffline(ctx, p); err != nil {
			t.Fatal(err)
		}
		raw, err = OpenCoexistence(ctx, p, DevelopmentCoexistenceDeclaration())
		if err != nil {
			t.Fatal(err)
		}
		s = raw.(*sqliteStore)
		if i == 0 {
			if _, err = s.db.Exec(`INSERT INTO conversation_ownership(conversation_id,owner,epoch,state,destination,handover) VALUES('original','A','epoch-A','owned','A','')`); err != nil {
				t.Fatal(err)
			}
		} else {
			var owner, epoch string
			if err = s.db.QueryRow(`SELECT owner,epoch FROM conversation_ownership WHERE conversation_id='original'`).Scan(&owner, &epoch); err != nil || owner != "A" || epoch != "epoch-A" {
				t.Fatalf("repeat reset ownership: %s %s %v", owner, epoch, err)
			}
		}
		var history, status string
		var updated int
		if err = s.db.QueryRow("SELECT content FROM turns WHERE id='turn'").Scan(&history); err != nil {
			t.Fatal(err)
		}
		if err = s.db.QueryRow("SELECT status,updated_at FROM delegated_tasks WHERE id='task'").Scan(&status, &updated); err != nil {
			t.Fatal(err)
		}
		if history != "original history" || status != "running" || updated != 5 {
			t.Fatalf("mutated history/task: %s %s %d", history, status, updated)
		}
		if _, err = s.execOwned(ctx, "DELETE FROM turns"); err == nil {
			t.Fatal("unowned marked write accepted")
		}
		s.Close()
	}
}
func TestOfflineUpgradeRollsBack(t *testing.T) {
	p := offlinePath(t)
	raw, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(*sqliteStore)
	// Force marker creation to fail after the additive initialization steps.
	if _, err = s.db.Exec("CREATE VIEW coexistence_metadata AS SELECT 1"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err = UpgradeOffline(context.Background(), p); err == nil {
		t.Fatal("expected failure")
	}
	db, err := openCoexistenceDB(p, "ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"delegated_tasks", "conversation_ownership"} {
		exists, err := tableExists(db, name)
		if err != nil || exists {
			t.Fatalf("partial upgrade %s %v %v", name, exists, err)
		}
	}
	var uv int
	db.QueryRow("PRAGMA user_version").Scan(&uv)
	if uv != 0 {
		t.Fatal(uv)
	}
}

func TestOfflineUpgradeInterruptsWithoutReplay(t *testing.T) {
	ctx := context.Background()
	p := offlinePath(t)
	raw, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(*sqliteStore)
	if err := s.InitTasks(ctx); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"accepted", "running", "completed", "failed"} {
		if _, err := s.db.Exec(`INSERT INTO delegated_tasks(id,parent_id,status,task,work_dir,created_at,updated_at,result_json) VALUES(?, 'parent', ?, 'saved task', '', 1, 2, '{"partial":true}')`, status, status); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	if err := UpgradeInterruptedOffline(ctx, p); err != nil {
		t.Fatal(err)
	}
	raw, err = OpenCoexistence(ctx, p, DevelopmentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	s = raw.(*sqliteStore)
	for _, original := range []string{"accepted", "running", "completed", "failed"} {
		var status, task, result string
		if err := s.db.QueryRow(`SELECT status,task,result_json FROM delegated_tasks WHERE id=?`, original).Scan(&status, &task, &result); err != nil {
			t.Fatal(err)
		}
		want := original
		if original == "accepted" || original == "running" {
			want = "interrupted"
		}
		if status != want || task != "saved task" || result != `{"partial":true}` {
			t.Fatalf("lost task %s: %s %s %s", original, status, task, result)
		}
	}
}
