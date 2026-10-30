package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/recap"
	"cercano/source/server/pkg/proto"
)

type regenerationTestTokenizer struct{}

func (regenerationTestTokenizer) Count(s string) int { return len(s) / 4 }

type ordinaryRegenStream struct {
	proto.Agent_RegenerateContextServer
	ctx  context.Context
	last *proto.RegenerateContextProgress
	send func(*proto.RegenerateContextProgress) error
}

func (s *ordinaryRegenStream) Context() context.Context { return s.ctx }
func (s *ordinaryRegenStream) Send(p *proto.RegenerateContextProgress) error {
	s.last = p
	if s.send != nil {
		return s.send(p)
	}
	return nil
}

func ordinaryRegenFixture(t *testing.T, debounce time.Duration) (*Server, context.Context, broker.Owner, *compactiongen.Generator) {
	t.Helper()
	s, ctx, owner, _ := ordinaryPolicyFixture(t)
	var g *compactiongen.Generator
	s.installPreparedScheduling(s.agent, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
		// Small synthetic turns never reach the summarizer.
		g = compactiongen.New(st, nil, compactor.Config{ActivationFloorTokens: 1000}, regenerationTestTokenizer{}, debounce)
		g.SetEnabled(true)
		return nil, g
	})
	t.Cleanup(s.preparedScheduling.close)
	writeCtx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	s.persistSvc.PersistTurn(writeCtx, owner.Conversation, llm.Message{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: "synthetic test turn"}}})
	if err := persistsvc.WriteFailure(writeCtx); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Complete(); err != nil {
		t.Fatal(err)
	}
	return s, ctx, owner, g
}

func TestOrdinaryRegenerationModes(t *testing.T) {
	for _, mode := range []string{"rebuild", "clear", "incremental"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, owner, _ := ordinaryRegenFixture(t, time.Hour)
			stream := &ordinaryRegenStream{ctx: ctx}
			req := &proto.RegenerateContextRequest{ConversationId: owner.Conversation, ClearOnly: mode == "clear", Incremental: mode == "incremental"}
			if err := s.RegenerateContext(req, stream); err != nil {
				t.Fatalf("ordinary regeneration refused: %v", err)
			}
			if stream.last == nil || !stream.last.GetDone() || !stream.last.GetOk() {
				t.Fatalf("result: %v", stream.last)
			}
			err := s.ReleaseRetained(ctx, owner, nil)
			if mode == "incremental" {
				if !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("pending producer not retained: %v", err)
				}
				s.preparedScheduling.close()
				if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
					t.Fatalf("cancelled pending producer leaked work: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOrdinaryRegenerationAuthority(t *testing.T) {
	for _, mode := range []string{"rebuild", "clear", "incremental"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, owner, _ := ordinaryRegenFixture(t, time.Hour)
			stale := owner
			stale.Epoch = "stale"
			for name, badCtx := range map[string]context.Context{"unbound": t.Context(), "stale": foregroundBinding(ctx, stale)} {
				stream := &ordinaryRegenStream{ctx: badCtx}
				if err := s.RegenerateContext(&proto.RegenerateContextRequest{ConversationId: owner.Conversation, ClearOnly: mode == "clear", Incremental: mode == "incremental"}, stream); err == nil {
					t.Fatalf("%s admitted", name)
				}
				if stream.last != nil {
					t.Fatalf("%s published before admission", name)
				}
			}
			if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
				t.Fatalf("rejected requests left work: %v", err)
			}
		})
	}
}

func TestOrdinaryRegenerationCancellationAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			s, ctx, owner, g := ordinaryRegenFixture(t, time.Millisecond)
			entered, finish := make(chan struct{}), make(chan struct{})
			g.SetToolElisionOnly(true)
			g.SetElideOnlyFn(func(writeCtx context.Context, id string) (int, int, int, bool, error) {
				close(entered)
				<-finish // A cancelled RPC must not release this write's ownership.
				if fail {
					err := errors.New("injected persistence failure")
					persistsvc.ReportWriteFailure(writeCtx, err)
					return 0, 0, 0, false, err
				}
				return 0, 0, 0, false, nil
			})
			rpcCtx, cancel := context.WithCancel(ctx)
			stream := &ordinaryRegenStream{ctx: rpcCtx}
			if err := s.RegenerateContext(&proto.RegenerateContextRequest{ConversationId: owner.Conversation, Incremental: true}, stream); err != nil || !stream.last.GetOk() {
				t.Fatalf("admission: %v %v", stream.last, err)
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("producer never started")
			}
			cancel()
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("running write released: %v", err)
			}
			closed := make(chan struct{})
			go func() { s.preparedScheduling.close(); close(closed) }()
			select {
			case <-closed:
				t.Fatal("cancellation treated as write completion")
			case <-time.After(20 * time.Millisecond):
			}
			close(finish)
			select {
			case <-closed:
			case <-time.After(5 * time.Second):
				t.Fatal("producer failed to finish")
			}
			err := s.ReleaseRetained(ctx, owner, nil)
			if fail && !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("failed write lost blocker: %v", err)
			}
			if !fail && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOrdinaryRegenerationSynchronousFailure(t *testing.T) {
	s, ctx, owner, g := ordinaryRegenFixture(t, time.Hour)
	g.SetContextUsageFn(func(ctx context.Context, _ string, _, _ int) {
		persistsvc.ReportWriteFailure(ctx, errors.New("injected usage write failure"))
	})
	stream := &ordinaryRegenStream{ctx: ctx}
	if err := s.RegenerateContext(&proto.RegenerateContextRequest{ConversationId: owner.Conversation, ClearOnly: true}, stream); err != nil {
		t.Fatal(err)
	}
	if stream.last.GetOk() {
		t.Fatal("failed write published success")
	}
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("failed synchronous write released: %v", err)
	}
}

// Keep a synchronous claim occupied across several producer debounce intervals.
// The admitted incremental request must retry under its retained ticket, then
// execute with the same capability after the claim is released.
func TestOrdinaryRegenerationBusyReschedule(t *testing.T) {
	s, ctx, owner, g := ordinaryRegenFixture(t, time.Millisecond)
	claimed, release := make(chan struct{}), make(chan struct{})
	clearDone := make(chan error, 1)
	stream := &ordinaryRegenStream{ctx: ctx, send: func(p *proto.RegenerateContextProgress) error {
		if !p.GetDone() {
			close(claimed)
			<-release
		}
		return nil
	}}
	go func() {
		clearDone <- s.RegenerateContext(&proto.RegenerateContextRequest{ConversationId: owner.Conversation, ClearOnly: true}, stream)
	}()
	select {
	case <-claimed:
	case <-time.After(5 * time.Second):
		t.Fatal("clear never acquired claim")
	}
	entered, finish := make(chan struct{}), make(chan struct{})
	g.SetToolElisionOnly(true)
	g.SetElideOnlyFn(func(writeCtx context.Context, id string) (int, int, int, bool, error) {
		cap, err := s.resolvePreparedTaskOwner(writeCtx, id)
		if err != nil || ownerOf(cap) != owner {
			t.Errorf("retry lost authority: %v", err)
		}
		close(entered)
		<-finish
		return 0, 0, 0, false, nil
	})
	ack := &ordinaryRegenStream{ctx: ctx}
	if err := s.RegenerateContext(&proto.RegenerateContextRequest{ConversationId: owner.Conversation, Incremental: true}, ack); err != nil || !ack.last.GetOk() {
		t.Fatalf("incremental admission: %v %v", ack.last, err)
	}
	select {
	case <-entered:
		t.Fatal("producer ignored clear claim")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-clearDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("busy producer never retried")
	}
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("retry lost ticket: %v", err)
	}
	close(finish)
	s.preparedScheduling.close()
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatalf("retry leaked work: %v", err)
	}
}

func TestOrdinaryRegenerationScheduleFailure(t *testing.T) {
	s, ctx, owner, g := ordinaryRegenFixture(t, time.Hour)
	g.SetEnabled(false)
	stream := &ordinaryRegenStream{ctx: ctx}
	if err := s.RegenerateContext(&proto.RegenerateContextRequest{ConversationId: owner.Conversation, Incremental: true}, stream); err != nil {
		t.Fatal(err)
	}
	if stream.last.GetOk() {
		t.Fatal("failed producer admission published success")
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatalf("failed producer admission leaked ticket: %v", err)
	}
}
