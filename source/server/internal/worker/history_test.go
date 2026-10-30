package worker

import (
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/proto"
	"context"
	"testing"
)

func TestHistoryStreamScopeAndBounds(t *testing.T) {
	calls := 0
	h := conversation.HistoryReaderFunc(func(_ context.Context, id string, q conversation.HistoryQuery) ([]conversation.HistoryEntry, error) {
		calls++
		if id != "active" {
			t.Fatalf("scope %q", id)
		}
		return []conversation.HistoryEntry{{TurnID: "one", Content: q.Query}}, nil
	})
	p := newStreamHistory(nil, "active")
	p.sendFn = func(m *proto.WorkerToHost) {
		p.deliver(handleHistory(context.Background(), h, "active", m.GetHistoryRequest()))
	}
	q := conversation.HistoryQuery{Query: "needle", Limit: 1}
	a, e := p.ReadHistory(context.Background(), "active", q)
	if e != nil || len(a) != 1 || a[0].Content != "needle" {
		t.Fatalf("%+v %v", a, e)
	}
	if _, e := p.ReadHistory(context.Background(), "other", q); e == nil {
		t.Fatal("foreign scope accepted")
	}
	for _, r := range []*proto.ConversationHistoryRequest{{Query: "x", Limit: 51}, {Query: "x", Limit: 1, After: -1}, {Query: "x", TurnId: "one", Limit: 1}} {
		if handleHistory(context.Background(), h, "active", r).Error == "" {
			t.Fatal("invalid query accepted")
		}
	}
	if handleHistory(context.Background(), h, "", &proto.ConversationHistoryRequest{Query: "x", Limit: 1}).Error == "" {
		t.Fatal("missing scope accepted")
	}
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := p.ReadHistory(ctx, "active", q); e == nil {
		t.Fatal("cancellation ignored")
	}
}
