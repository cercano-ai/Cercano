package conversation

import (
	"context"
	"testing"
	"time"
)

func TestStore_ListExcludesZeroTurns(t *testing.T) {
	ctx := context.Background()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.EnsureConversation(ctx, "empty", "/p", "m"); err != nil {
		t.Fatal(err)
	}
	infos, err := s.List(ctx, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatalf("zero-turn conversation appeared in history: %+v", infos)
	}
	info, err := s.Get(ctx, "empty")
	if err != nil || info.ID != "empty" || info.TurnCount != 0 {
		t.Fatalf("empty conversation must remain retrievable: info=%+v err=%v", info, err)
	}
	turns, err := s.GetTurns(ctx, "empty")
	if err != nil || len(turns) != 0 {
		t.Fatalf("empty conversation turns: turns=%+v err=%v", turns, err)
	}
}

func TestStore_ListNonemptyFiltersBeforeLimit(t *testing.T) {
	ctx := context.Background()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Empty conversations have current timestamps, newer than all these turns.
	for _, id := range []string{"empty-a", "empty-b"} {
		if err := s.EnsureConversation(ctx, id, "/p", "m"); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		id, project, role, content string
		at                         int64
	}{
		{"populated", "/p", "user", "hello", 100},
		{"blank", "/p", "assistant", "", 200},
		{"whitespace", "/p", "user", "   \n\t", 300},
		{"other-project", "/other", "user", "elsewhere", 400},
	} {
		if err := s.EnsureConversation(ctx, tc.id, tc.project, "m"); err != nil {
			t.Fatal(err)
		}
		if err := s.Append(ctx, Turn{ConversationID: tc.id, Role: tc.role, Content: tc.content, CreatedAt: time.Unix(tc.at, 0)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.EnsureSubagentConversation(ctx, "child", "populated", "/p", "m", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(ctx, Turn{ConversationID: "child", Role: "assistant", Content: "child result", CreatedAt: time.Unix(500, 0)}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, project string
		limit         int
		want          []string
	}{
		{"project unlimited", "/p", 0, []string{"whitespace", "blank", "populated"}},
		{"project negative limit", "/p", -1, []string{"whitespace", "blank", "populated"}},
		{"project limited", "/p", 2, []string{"whitespace", "blank"}},
		{"project excess limit", "/p", 10, []string{"whitespace", "blank", "populated"}},
		{"all unlimited", "", 0, []string{"other-project", "whitespace", "blank", "populated"}},
		{"all limited", "", 2, []string{"other-project", "whitespace"}},
		{"no matching project", "/missing", 2, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			infos, err := s.List(ctx, tc.project, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(infos) != len(tc.want) {
				t.Fatalf("want IDs %v, got %+v", tc.want, infos)
			}
			for i, want := range tc.want {
				if infos[i].ID != want || infos[i].TurnCount != 1 {
					t.Errorf("entry %d: want %s with one turn, got %+v", i, want, infos[i])
				}
			}
		})
	}
}
