package conversation

import (
	"context"
	"strings"
	"testing"
)

func TestHistoryIsolationBoundsLiteralSearch(t *testing.T) {
	ctx := context.Background()
	s, e := Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for _, id := range []string{"active", "other"} {
		if e := s.EnsureConversation(ctx, id, "/p", "m"); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []Turn{{ID: "one", ConversationID: "active", Role: "user", Content: "needle ' OR 1=1 --"}, {ID: "two", ConversationID: "active", Role: "assistant", Content: strings.Repeat("needle", 1000)}, {ID: "hidden", ConversationID: "active", Role: "system", Content: "needle"}, {ID: "foreign", ConversationID: "other", Role: "user", Content: "needle"}} {
		if e := s.Append(ctx, v); e != nil {
			t.Fatal(e)
		}
	}
	q := HistoryQuery{Query: "needle", Limit: 1}
	a, e := s.ReadHistory(ctx, "active", q)
	if e != nil || len(a) != 1 || a[0].TurnID != "one" {
		t.Fatalf("%+v %v", a, e)
	}
	q.After = a[0].Cursor
	a, e = s.ReadHistory(ctx, "active", q)
	if e != nil || len(a) != 1 || a[0].TurnID != "two" || len(a[0].Content) != 2048 || !a[0].Truncated {
		t.Fatalf("%+v %v", a, e)
	}
	q.After = a[0].Cursor
	a, e = s.ReadHistory(ctx, "active", q)
	if e != nil || len(a) != 0 {
		t.Fatalf("%+v %v", a, e)
	}
	for _, q := range []HistoryQuery{{TurnID: "foreign", Limit: 1}, {TurnID: "hidden", Limit: 1}, {Query: "%", Limit: 50}, {Query: "' OR 1=1 --", Limit: 50}} {
		a, e = s.ReadHistory(ctx, "active", q)
		want := 0
		if q.Query == "' OR 1=1 --" {
			want = 1
		}
		if e != nil || len(a) != want {
			t.Fatalf("%+v: %+v %v", q, a, e)
		}
	}
	for _, q := range []HistoryQuery{{Query: "x", Limit: 51}, {Query: strings.Repeat("x", 513), Limit: 1}, {Query: "x", TurnID: "one", Limit: 1}, {TurnID: "one", Limit: 1, After: -1}} {
		if _, e := s.ReadHistory(ctx, "active", q); e == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
}
