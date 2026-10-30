package worker

import (
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/proto"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// HistorySetter wires the only database owner into the stream host.
type HistorySetter interface {
	SetHistory(conversation.HistoryReader)
}

func (w *workerRunner) SetHistory(h conversation.HistoryReader) { w.history = h }

func handleHistory(ctx context.Context, h conversation.HistoryReader, activeID string, r *proto.ConversationHistoryRequest) *proto.ConversationHistoryResponse {
	out := &proto.ConversationHistoryResponse{Id: r.GetId()}
	q := conversation.HistoryQuery{Query: r.GetQuery(), TurnID: r.GetTurnId(), After: r.GetAfter(), Limit: int(r.GetLimit())}
	if err := q.Validate(); err != nil {
		out.Error = err.Error()
		return out
	}
	if h == nil || activeID == "" {
		out.Error = "conversation history unavailable"
		return out
	}
	entries, err := h.ReadHistory(ctx, activeID, q)
	if err == nil {
		out.EntriesJson, err = json.Marshal(entries)
	}
	if err != nil {
		out.Error = err.Error()
	}
	return out
}

type streamHistory struct {
	sndr           *sender
	conversationID string
	next           atomic.Uint64
	mu             sync.Mutex
	pending        map[string]chan *proto.ConversationHistoryResponse
	sendFn         func(*proto.WorkerToHost)
}

func newStreamHistory(s *sender, id string) *streamHistory {
	return &streamHistory{sndr: s, conversationID: id, pending: map[string]chan *proto.ConversationHistoryResponse{}}
}
func (p *streamHistory) ReadHistory(ctx context.Context, id string, q conversation.HistoryQuery) ([]conversation.HistoryEntry, error) {
	// Subagent calls cannot silently read their parent's conversation.
	if id == "" || id != p.conversationID {
		return nil, fmt.Errorf("history is scoped to the active conversation")
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := fmt.Sprint(p.next.Add(1))
	ch := make(chan *proto.ConversationHistoryResponse, 1)
	p.mu.Lock()
	p.pending[key] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, key); p.mu.Unlock() }()
	m := &proto.WorkerToHost{Msg: &proto.WorkerToHost_HistoryRequest{HistoryRequest: &proto.ConversationHistoryRequest{Id: key, Query: q.Query, TurnId: q.TurnID, After: q.After, Limit: int32(q.Limit)}}}
	if p.sendFn != nil {
		p.sendFn(m)
	} else {
		p.sndr.send(m)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.GetError() != "" {
			return nil, errors.New(r.GetError())
		}
		var entries []conversation.HistoryEntry
		err := json.Unmarshal(r.GetEntriesJson(), &entries)
		return entries, err
	}
}
func (p *streamHistory) deliver(r *proto.ConversationHistoryResponse) {
	p.mu.Lock()
	ch := p.pending[r.GetId()]
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- r:
		default:
		}
	}
}
