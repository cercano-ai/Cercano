package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compaction"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/contextmeter"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	providersvc "cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/recap"
	"cercano/source/server/internal/runner"
	"cercano/source/server/pkg/config"
)

// Probe delegates every operation; it neither injects errors nor changes contexts.
type ancillaryReadProbe struct {
	conversation.Store
	t *testing.T
}

func (p ancillaryReadProbe) Get(ctx context.Context, id string) (conversation.Info, error) {
	v, err := p.Store.Get(ctx, id)
	if err != nil {
		p.t.Logf("recap store.Get id=%s ctx=%v err=%v writeFailure=%v", id, ctx.Err(), err, persistsvc.WriteFailure(ctx))
	}
	return v, err
}
func (p ancillaryReadProbe) GetTurns(ctx context.Context, id string) ([]conversation.Turn, error) {
	v, err := p.Store.GetTurns(ctx, id)
	if err != nil {
		p.t.Logf("recap store.GetTurns id=%s ctx=%v err=%v writeFailure=%v", id, ctx.Err(), err, persistsvc.WriteFailure(ctx))
	}
	return v, err
}

type ancillaryProvider struct{ *scriptedProvider }

func (ancillaryProvider) Capabilities() inference.Capabilities {
	return inference.Capabilities{SupportsTools: true}
}

type ancillaryResolver struct {
	providersvc.Resolver
	provider inference.Provider
}

func (r ancillaryResolver) Main() (inference.Provider, bool, bool, error) {
	return r.provider, false, false, nil
}

type ancillaryBarrier struct {
	entered, canceled, release, returned chan struct{}
	once                                 sync.Once
	enterOnce, cancelOnce, returnOnce    sync.Once
	calls                                atomic.Int32
	completed, writeFailures             atomic.Int32
	expected                             int32
	ctx                                  context.Context // published by entered
}

func newAncillaryBarrier(expected int32) *ancillaryBarrier {
	return &ancillaryBarrier{expected: expected, entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{})}
}
func (b *ancillaryBarrier) unblock() { b.once.Do(func() { close(b.release) }) }
func (b *ancillaryBarrier) call(ctx context.Context) error {
	b.enterOnce.Do(func() { b.ctx = ctx })
	if b.calls.Add(1) == b.expected {
		close(b.entered)
	}
	<-ctx.Done()
	b.cancelOnce.Do(func() { close(b.canceled) })
	<-b.release
	defer func() {
		if persistsvc.WriteFailure(ctx) != nil {
			b.writeFailures.Add(1)
		}
		b.completed.Add(1)
		b.returnOnce.Do(func() { close(b.returned) })
	}()
	return ctx.Err()
}
func ancillaryWait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("ancillary barrier timed out")
	}
}

// Real Core post-turn scheduling, real retained admission, real inference
// callbacks and real store. No workerTurnDone or synthetic PersistenceFailed.
// This CHARACTERIZES conservative producer error accounting; it does not bless it.
// Set CERCANO_ANCILLARY_EXPECT_CLEAN=1 only for the documented red proof.
func TestPreparedAncillaryCancellationEvidence(t *testing.T) {
	if os.Getenv("CERCANO_ANCILLARY_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPreparedAncillaryCancellationEvidence$", "-test.v")
		cmd.Env = append(os.Environ(), "CERCANO_ANCILLARY_CHILD=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("owned subprocess: %v\n%s", err, out)
		}
		t.Logf("%s", out)
		return
	}

	for _, mode := range []string{"recap", "compaction", "both"} {
		t.Run(mode, func(t *testing.T) {
			// Agent and Core each admit recap; wait for both inference calls, not just one.
			rbar, cbar := newAncillaryBarrier(2), newAncillaryBarrier(1)
			defer rbar.unblock()
			defer cbar.unblock()
			s, ctx, owner, tasks := preparedWakeFixtureWithConfig(t, config.Config{OpenRuntime: "synthetic", LocusMode: "local"}, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
				var r *recap.Generator
				var c *compactiongen.Generator
				if mode != "compaction" {
					r = recap.New(ancillaryReadProbe{Store: st, t: t}, func(ctx context.Context, _ string) (string, error) { return "", rbar.call(ctx) }, 0, 10)
				}
				if mode != "recap" {
					c = compactiongen.New(st, func(ctx context.Context, _ []llm.Message) (compaction.StructuredSummary, error) {
						return compaction.StructuredSummary{}, cbar.call(ctx)
					}, compactor.Config{ActivationFloorTokens: 100, SegmentTokens: 1000, VerbatimRecent: 2}, contextmeter.Default(), time.Millisecond)
					c.SetEnabled(true)
				}
				return r, c
			})
			// Fixture task is irrelevant to this normal main turn; acknowledge it so
			// retained release has no unrelated notification blocker.
			if err := tasks.HandleTaskNotification(ctx, "parent", "one"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 12; i++ {
				if err := s.persistSvc.Store().Append(ctx, conversation.Turn{ID: fmt.Sprintf("seed-%d", i), ConversationID: "parent", Role: string(llm.RoleUser), Content: strings.Repeat("context evidence ", 500), CreatedAt: time.Now().Add(time.Duration(i-20) * time.Second)}); err != nil {
					t.Fatal(err)
				}
			}
			s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
			conf := config.Config{LocusMode: "open_only", OpenRuntime: "ollama"}
			conf.Models.SetOverride("ollama", conf.TaskAssignment(config.TaskChat).Quality.CapabilityTier(), "fixture-model")
			s.cfgSvc.Set(conf)
			s.SetOpenLLMProvider(&scriptedProvider{scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "No further action needed."}}}})
			runctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.ForegroundWork)
			if err != nil {
				t.Fatal(err)
			}
			s.SetToolRegistry(agenttools.NewRegistry())
			deps := s.runnerDeps()
			deps.Providers = ancillaryResolver{Resolver: deps.Providers, provider: ancillaryProvider{&scriptedProvider{scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "No further action needed."}}}}}}
			result, err := runner.New(deps).RunTurn(runctx, runner.Request{ConversationID: "parent", Input: "finish normally", WorkDir: t.TempDir(), Gen: 1}, &capSink{}, nil, func(m llm.Message) { s.persistSvc.PersistTurn(runctx, "parent", m) })
			if err != nil || result.FinalText != "No further action needed." {
				t.Fatalf("main result=%+v err=%v", result, err)
			}
			if err := persistsvc.WriteFailure(runctx); err != nil {
				t.Fatal(err)
			}
			if err := ticket.Complete(); err != nil {
				t.Fatal(err)
			}
			bars := []*ancillaryBarrier{}
			if mode != "compaction" {
				bars = append(bars, rbar)
			}
			if mode != "recap" {
				bars = append(bars, cbar)
			}
			for _, b := range bars {
				ancillaryWait(t, b.entered)
			}
			snapshot := func(stage string) broker.AdmissionSnapshot {
				snap := s.coexistence.admission.Snapshot(owner)
				t.Logf("%s owner=%+v accounting=%+v", stage, owner, snap)
				return snap
			}
			before := snapshot("providers-entered/main-completed")
			if before.Work[broker.PersistenceWork] < len(bars) || before.UnresolvedWork != 0 {
				t.Fatalf("before cancellation: %+v", before)
			}
			closed := make(chan struct{})
			go func() { s.preparedScheduling.close(); close(closed) }()
			// close joins recap before closing compaction, so release each in order.
			for _, b := range bars {
				ancillaryWait(t, b.canceled)
				select {
				case <-closed:
					t.Fatal("close abandoned provider")
				default:
				}
				b.unblock()
				ancillaryWait(t, b.returned)
			}
			ancillaryWait(t, closed)
			for _, b := range bars {
				if b.completed.Load() != b.calls.Load() || b.calls.Load() < b.expected || b.writeFailures.Load() != 0 {
					t.Fatalf("provider callbacks: entered=%d completed=%d writeFailures=%d", b.calls.Load(), b.completed.Load(), b.writeFailures.Load())
				}
				if !errors.Is(b.ctx.Err(), context.Canceled) {
					t.Fatal(b.ctx.Err())
				}
				if err := persistsvc.WriteFailure(b.ctx); err != nil {
					t.Fatalf("provider cancellation became a reported DB failure: %v", err)
				}
			}
			after := snapshot("providers-returned/close-joined")
			if os.Getenv("CERCANO_ANCILLARY_EXPECT_CLEAN") == "1" && after.UnresolvedWork != 0 {
				t.Errorf("desired clean inference cancellation: unresolved=%d", after.UnresolvedWork)
			}
			// Multiple real post-turn admissions can overlap. Do not infer origin
			// from the count: preserve the evidence and require every remaining
			// ticket to be explicitly unresolved (not an unjoined producer).
			if (mode != "recap" && after.UnresolvedWork < 1) || (mode == "recap" && after.UnresolvedWork != 0) || after.Work[broker.PersistenceWork] != after.UnresolvedWork || after.UnresolvedWork > before.Work[broker.PersistenceWork]+before.Work[broker.CallbackWork] {
				t.Fatalf("unexpected joined accounting: before=%+v after=%+v", before, after)
			}
			releaseErr := s.ReleaseRetained(ctx, owner, nil)
			if after.UnresolvedWork == 0 {
				if releaseErr != nil {
					t.Fatal(releaseErr)
				}
			} else if !errors.Is(releaseErr, broker.ErrAdmissionBusy) {
				t.Fatalf("uncertainty allowed release: %v", releaseErr)
			}
			state, e := conversation.ReadOwnership(ctx, s.coexistence.store, "parent")
			if e != nil || (state.State == "released") != (after.UnresolvedWork == 0) {
				t.Fatalf("blocked durable state=%+v err=%v", state, e)
			}

			if after.UnresolvedWork == 0 {
				opts := s.coexistence.options
				opts.Host, opts.Build = "B", "B"
				next, err := NewCoexistenceServer(t.Context(), opts)
				if err != nil {
					t.Fatal(err)
				}
				defer next.CloseCoexistenceStore()
				resumed, err := next.Retain(t.Context(), "parent")
				if err != nil {
					t.Fatalf("ordinary new-host retain without recovery: %v", err)
				}
				if resumed.Epoch == owner.Epoch {
					t.Fatal("epoch reused")
				}
				if err := next.ReleaseRetained(t.Context(), resumed, nil); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
