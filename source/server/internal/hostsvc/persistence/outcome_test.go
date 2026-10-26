package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/llm"
)

type appendFailureStore struct {
	conversation.Store
	err error
}

func (s appendFailureStore) Append(context.Context, conversation.Turn) error { return s.err }

func TestPersistTurnReportsFailureAcrossBoundedWrite(t *testing.T) {
	failure := errors.New("synthetic append failure")
	x := &svc{convAgent: viewportResumeFakeAgent{store: appendFailureStore{err: failure}}}
	count := 0
	ctx, cancel := context.WithCancel(t.Context())
	ctx = WithWriteFailure(ctx, func(err error) {
		if !errors.Is(err, failure) {
			t.Error(err)
		}
		count++
	})
	cancel()
	bounded, done := BoundedWriteContext(ctx, time.Second)
	defer done()
	if bounded.Err() != nil {
		t.Fatal(bounded.Err())
	}
	x.PersistTurn(bounded, "one", llm.Message{Role: llm.RoleAssistant})
	x.PersistTurn(bounded, "one", llm.Message{Role: llm.RoleAssistant})
	if count != 1 || !errors.Is(WriteFailure(ctx), failure) {
		t.Fatalf("outcome: calls=%d err=%v", count, WriteFailure(ctx))
	}
}
