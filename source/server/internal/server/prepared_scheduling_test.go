package server

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/recap"
	"cercano/source/server/internal/requestassembly"
	"cercano/source/server/pkg/config"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Original gap probe retained for the default path: no implicit generators are
// installed. This passed before integration and must remain true without opt-in.
func TestPreparedSchedulingGap(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	ctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	s.persistSvc.PersistTurn(ctx, owner.Conversation, llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "fixture"}}})
	if err := ticket.Complete(); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatalf("unexpected owned producer: %v", err)
	}
}

// Use a real destination acceptance to exercise the transfer gate, not just
// release. Busy work must stop before any source handover reservation is made.
func preparedSchedulingDestination(t *testing.T, s *Server, owner broker.Owner) *IncomingAcceptance {
	t.Helper()
	options := s.coexistence.options
	sourceBuild := options.Build
	options.Host = "scheduling-destination"
	options.Build = "scheduling-destination-build"
	destination, err := NewCoexistenceServer(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = destination.coexistence.store.Close() })
	accepted, err := destination.AcceptIncoming(owner, "scheduling-transfer", sourceBuild, options.Contract)
	if err != nil {
		t.Fatal(err)
	}
	return accepted
}

func assertPreparedSchedulingTransferBusy(t *testing.T, s *Server, ctx context.Context, owner broker.Owner, accepted *IncomingAcceptance) {
	t.Helper()
	if err := s.HandoverRetained(ctx, owner, nil, accepted); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("transfer while producer busy: %v", err)
	}
	s.coexistence.mu.Lock()
	defer s.coexistence.mu.Unlock()
	if len(s.coexistence.closures) != 0 {
		t.Fatal("busy transfer left source reservation")
	}
}

func TestPreparedSchedulingPendingAndStale(t *testing.T) {
	for _, producer := range []string{"recap", "compaction", "both"} {
		t.Run(producer, func(t *testing.T) {
			s, ctx, owner, _ := preparedWakeFixture(t, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
				var r *recap.Generator
				var c *compactiongen.Generator
				if producer != "compaction" {
					r = recap.New(st, func(context.Context, string) (string, error) { t.Error("pending timer executed"); return "", nil }, time.Hour, 10)
				}
				if producer != "recap" {
					c = compactiongen.New(st, nil, compactor.Config{}, nil, time.Hour)
					c.SetEnabled(true)
				}
				return r, c
			})
			defer s.preparedScheduling.close()
			accepted := preparedSchedulingDestination(t, s, owner)
			// Context contains explicit capability but a forged epoch; no current-owner
			// fallback may turn this into accepted work.
			cap := s.coexistence.owners[owner.Conversation]
			stale := owner
			stale.Epoch += "-stale"
			staleCtx := WithPreparedTaskOwnership(ctx, stale, cap)
			if err := s.agent.ScheduleRecapContext(staleCtx, owner.Conversation); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("stale: %v", err)
			}
			if err := s.agent.ScheduleCompactionContext(ctx, "sibling"); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("cross owner: %v", err)
			}
			if err := s.agent.ScheduleCompactionContext(context.Background(), owner.Conversation); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("ambient: %v", err)
			}
			admitted, ticket, err := s.BeginRetainedWork(ctx, owner, broker.ForegroundWork)
			if err != nil {
				t.Fatal(err)
			}
			s.persistSvc.PersistTurn(admitted, owner.Conversation, llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "owned"}}})
			if err := ticket.Complete(); err != nil {
				t.Fatal(err)
			}
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("timer not counted: %v", err)
			}
			assertPreparedSchedulingTransferBusy(t, s, ctx, owner, accepted)
			s.FenceBackgroundWork()
			if err := s.agent.ScheduleRecapContext(ctx, owner.Conversation); err == nil {
				t.Fatal("fenced accepted")
			}
			s.JoinBackgroundWork()
			if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
				t.Fatalf("join did not drain: %v", err)
			}
		})
	}
}

func TestPreparedSchedulingFinalCallbackShutdownJoin(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	s, ctx, owner, _ := preparedWakeFixture(t, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
		c := compactiongen.New(st, nil, compactor.Config{}, nil, 0)
		c.SetEnabled(true)
		c.SetToolElisionOnly(true)
		c.SetElideOnlyFn(func(ctx context.Context, id string) (int, int, int, bool, error) {
			if id != "parent" {
				t.Errorf("cross-owner work: %s", id)
			}
			if ctx.Value(preparedTaskAuthorityKey{}) == nil {
				t.Error("missing capability")
			}
			return 100, 10, 1, true, nil
		})
		c.SetContextUsageFn(func(context.Context, string, int, int) { close(entered); <-finish })
		return nil, c
	})
	accepted := preparedSchedulingDestination(t, s, owner)
	released := false
	defer func() {
		if !released {
			close(finish)
		}
		s.preparedScheduling.close()
	}()
	admitted, ticket, err := s.BeginRetainedWork(ctx, owner, broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	s.persistSvc.PersistTurn(admitted, owner.Conversation, llm.Message{Role: llm.RoleAssistant})
	if err := ticket.Complete(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("callback not reached")
	}
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("final callback uncounted: %v", err)
	}
	assertPreparedSchedulingTransferBusy(t, s, ctx, owner, accepted)
	joined := make(chan struct{})
	go func() { s.Shutdown(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("shutdown did not join final callback")
	case <-time.After(30 * time.Millisecond):
	}
	close(finish)
	released = true
	select {
	case <-joined:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}

// Exercise the other context-bearing persistence seam without an assistant
// append, so only assembly can have admitted the pending compaction ticket.
func TestPreparedSchedulingHistoryAssembly(t *testing.T) {
	cfg := config.Config{OpenRuntime: "synthetic"}
	cfg.Compaction.Enabled = true
	cfg.Compaction.HardOverridePct = 0.8
	s, ctx, owner, _ := preparedWakeFixtureWithConfig(t, cfg, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
		c := compactiongen.New(st, nil, compactor.Config{}, nil, time.Hour)
		c.SetEnabled(true)
		return nil, c
	})
	defer s.preparedScheduling.close()
	admitted, ticket, err := s.BeginRetainedWork(ctx, owner, broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	s.persistSvc.PersistTurn(admitted, owner.Conversation, llm.Message{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: strings.Repeat("history ", 1000)}}})
	result := s.persistSvc.AssembleHistoryForTarget(admitted, owner.Conversation, requestassembly.Target{ContextWindow: 128, ContextWindowKnown: true})
	if !result.Accounting.Scheduled {
		t.Fatal("fixture did not reach scheduling threshold")
	}
	if !s.cfgSvc.Frozen() {
		t.Fatal("shared config thawed")
	}
	if err := ticket.Complete(); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("assembly timer not counted: %v", err)
	}
	s.FenceBackgroundWork()
	s.JoinBackgroundWork()
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
}

// Callback write failures belong to the producer ticket and remain conservative
// blockers even once the generator has been joined.
func TestPreparedSchedulingScopedPersistenceFailure(t *testing.T) {
	failure := errors.New("injected final usage write failure")
	callback := make(chan struct{})
	s, ctx, owner, _ := preparedWakeFixture(t, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
		c := compactiongen.New(st, nil, compactor.Config{}, nil, 0)
		c.SetEnabled(true)
		c.SetToolElisionOnly(true)
		c.SetElideOnlyFn(func(context.Context, string) (int, int, int, bool, error) { return 100, 10, 1, true, nil })
		c.SetContextUsageFn(func(ctx context.Context, _ string, _, _ int) {
			persistsvc.ReportWriteFailure(ctx, failure)
			close(callback)
		})
		return nil, c
	})
	if err := s.agent.ScheduleCompactionContext(ctx, owner.Conversation); err != nil {
		t.Fatal(err)
	}
	select {
	case <-callback:
	case <-time.After(3 * time.Second):
		t.Fatal("callback not reached")
	}
	s.JoinBackgroundWork()
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("failed write lost producer blocker: %v", err)
	}
}
