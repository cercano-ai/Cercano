package conversation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskProgressPageIndexedAccessAndSemantics(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "progress.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	db := s.(*sqliteStore)
	tasks := s.(TaskStore)
	if err := tasks.InitTasks(ctx); err != nil {
		t.Fatal(err)
	}
	for _, record := range []TaskRecord{
		{ID: "top", ParentID: "root", ChildID: "child", Status: "accepted"},
		{ID: "leaf", ParentID: "child", ChildID: "grandchild", Status: "accepted"},
		{ID: "private-task", ParentID: "elsewhere", ChildID: "private", Status: "accepted"},
	} {
		if err := tasks.InsertTask(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"child", "grandchild", "private"} {
		if err := s.EnsureConversation(ctx, id, "", "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	kinds := []string{"dispatch_start", "dispatch_done", "note", "model_response", "tool_call", "tool_result"}
	// 130 eligible even sequences, interleaved with 130 excluded model requests.
	// Paging from zero must yield 2..256, then 258 and 260, regardless of the
	// 512 unrelated events and the event belonging to the ancestor task.
	for i := 1; i <= 2*(TaskSubscriptionPageSize+2); i++ {
		kind := "model_request"
		if i%2 == 0 {
			kind = kinds[(i/2-1)%len(kinds)]
		}
		if _, err := db.db.ExecContext(ctx, `INSERT INTO dispatch_events(conversation_id,seq,kind,iteration,created_at,payload) VALUES('grandchild',?,?,7,100,'{"text":"fixture"}')`, i, kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<512) INSERT INTO dispatch_events(conversation_id,seq,kind,iteration,created_at,payload) SELECT 'private',x,'note',1,100,'{}' FROM n`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `INSERT INTO dispatch_events(conversation_id,seq,kind,iteration,created_at,payload) VALUES('child',1,'note',1,100,'{}')`); err != nil {
		t.Fatal(err)
	}

	t.Run("indexed event range", func(t *testing.T) {
		// Explain the exact production query with the current Go SQLite driver.
		// Do not pin an index name or the rest of the recursive tree's plan.
		rows, err := db.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+taskProgressPageQuery, "root", "leaf", 0, TaskSubscriptionPageSize)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		indexed := false
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
			upper := strings.ToUpper(detail)
			if strings.HasPrefix(upper, "SCAN E") {
				t.Errorf("full event scan: %s", detail)
			}
			if strings.HasPrefix(upper, "SEARCH E USING ") && strings.Contains(upper, "INDEX") && strings.Contains(upper, "CONVERSATION_ID=?") && strings.Contains(upper, "SEQ>?") {
				indexed = true
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !indexed {
			t.Fatalf("missing indexed event range lookup: %v", plan)
		}
		t.Logf("plan: %v", plan)
	})
	sub := s.(TaskSubscriptionStore)
	checkPage := func(parent string, after int64, count int, first int64) []DispatchEvent {
		t.Helper()
		got, err := sub.TaskProgressPage(ctx, parent, "leaf", after)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != count {
			t.Fatalf("parent=%s after=%d: got %d events, want %d", parent, after, len(got), count)
		}
		for i, ev := range got {
			seq := first + int64(2*i)
			if ev.ConversationID != "grandchild" || ev.Seq != seq || ev.Kind != kinds[(seq/2-1)%int64(len(kinds))] || ev.Iteration != 7 || ev.Timestamp.Unix() != 100 || ev.PayloadJSON != `{"text":"fixture"}` {
				t.Fatalf("event %d: %+v", i, ev)
			}
		}
		return got
	}
	first := checkPage("root", 0, TaskSubscriptionPageSize, 2)
	second := checkPage("root", first[len(first)-1].Seq, 2, 258)
	checkPage("root", second[len(second)-1].Seq, 0, 0)
	checkPage("root", 255, 3, 256)  // Exclusive cursor, including non-event cursors.
	checkPage("child", 256, 2, 258) // Immediate parent and ancestor both authorize observation.
	for _, tc := range []struct{ parent, id string }{
		{"elsewhere", "leaf"}, {"root", "private-task"}, {"root", "grandchild"}, {"root", "missing"}, {"grandchild", "leaf"},
	} {
		got, err := sub.TaskProgressPage(ctx, tc.parent, tc.id, 0)
		if err != nil || len(got) != 0 {
			t.Fatalf("unauthorized/missing %+v: got=%v err=%v", tc, got, err)
		}
	}
}
