package recap

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"cercano/source/server/internal/conversation"
)

func TestPersistenceOutcomeCompletionBoundaryAndLegacy(t *testing.T) {
	for _, strict := range []bool{false, true} {
		for _, failAt := range []int{1, 2} {
			t.Run(fmt.Sprintf("strict=%t/completion=%d", strict, failAt), func(t *testing.T) {
				store := &fakeStore{turns: []conversation.Turn{{Role: "user", Content: "hello"}}, updated: make(chan string, 2)}
				failure := errors.New("void completion callback persistence failed")
				var outcome error
				calls := 0
				g := New(store, func(context.Context, string) (string, error) {
					calls++
					if calls == failAt {
						outcome = failure
					}
					return "Completed Work", nil
				}, 0, 10)
				defer g.Close(context.Background())
				ctx := WithPersistenceOutcome(context.Background(), func() error { return outcome })
				err := g.runRecap(ctx, "c", strict)
				if strict {
					if !errors.Is(err, failure) || calls != failAt || len(store.titles) != 0 || len(store.recaps) != failAt-1 {
						t.Fatalf("calls=%d recap=%v title=%v err=%v", calls, store.recaps, store.titles, err)
					}
				} else if err != nil || calls != 2 || len(store.recaps) != 1 || len(store.titles) != 1 {
					t.Fatalf("legacy changed: calls=%d recap=%v title=%v err=%v", calls, store.recaps, store.titles, err)
				}
			})
		}
	}
}
