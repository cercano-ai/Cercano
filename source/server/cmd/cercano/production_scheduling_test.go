package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/loopcompact"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/usage"
	"cercano/source/server/pkg/config"
)

type schedulingRunner func(context.Context, *agent.Request) (*agent.Response, error)

func (s schedulingRunner) Name() string { return "memory-scheduling" }
func (s schedulingRunner) Process(ctx context.Context, r *agent.Request) (*agent.Response, error) {
	return s(ctx, r)
}

type schedulingProvider struct {
	developmentSyntheticProvider
	call func(context.Context) error
}

func (s *schedulingProvider) Chat(ctx context.Context, _ inference.Call) (inference.Result, error) {
	if err := s.call(ctx); err != nil {
		return inference.Result{}, err
	}
	return inference.Result{Blocks: []llm.Block{{Type: llm.BlockText, Text: "GOAL: fixture summary\nSTATE: completed fixture"}}}, nil
}

// These are constructor/bridge tests, not a second production application.
// Existing server prepared-scheduling tests exercise the agent installation.
func TestProductionSchedulingCommonFactory(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		for _, producer := range []string{"recap", "compaction"} {
			for _, shutdown := range []bool{false, true} {
				t.Run(fmt.Sprintf("prepared=%v/%s/shutdown=%v", prepared, producer, shutdown), func(t *testing.T) {
					t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					path := filepath.Join(t.TempDir(), "history.db")
					var st, second conversation.Store
					var err error
					if prepared {
						if err = conversation.PrepareCoexistence(ctx, path); err != nil {
							t.Fatal(err)
						}
						st, err = conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
					} else {
						st, err = conversation.Open(path)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer st.Close()
					if prepared {
						second, err = conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
					} else {
						second, err = conversation.Open(path)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer second.Close()
					// A separate store has the same ID: any captured/wrong-store writes are visible.
					wrong, err := conversation.Open(filepath.Join(t.TempDir(), "wrong.db"))
					if err != nil {
						t.Fatal(err)
					}
					defer wrong.Close()
					if err := wrong.EnsureConversation(ctx, "c", "", ""); err != nil {
						t.Fatal(err)
					}
					owned, other := ctx, ctx
					owner := broker.Owner{Conversation: "c", Host: "host", Epoch: "legacy"}
					if prepared {
						cap, err := conversation.AcquireOwnership(ctx, st, "c", "host")
						if err != nil {
							t.Fatal(err)
						}
						defer cap.Close()
						sibling, err := conversation.AcquireOwnership(ctx, second, "sibling", "other")
						if err != nil {
							t.Fatal(err)
						}
						defer sibling.Close()
						owned, other = conversation.WithOwnership(ctx, cap), conversation.WithOwnership(ctx, sibling)
						owner.Epoch = cap.Epoch()
					}
					for _, seed := range []struct {
						s   conversation.Store
						ctx context.Context
						id  string
					}{{st, owned, "c"}, {second, other, "sibling"}} {
						if err := seed.s.EnsureConversation(seed.ctx, seed.id, "", ""); err != nil {
							t.Fatal(err)
						}
					}
					for i := 0; i < 12; i++ {
						if err := st.Append(owned, conversation.Turn{ID: fmt.Sprint(i), ConversationID: "c", Role: "user", Content: strings.Repeat("fixture text ", 1000), CreatedAt: time.Now().Add(time.Duration(i-12) * time.Second)}); err != nil {
							t.Fatal(err)
						}
					}
					type key struct{}
					owned = context.WithValue(owned, key{}, "exact incoming context")
					entered, resume, callback, finish := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
					var once, callbackOnce, resumeOnce, finishOnce sync.Once
					unblock := func() { resumeOnce.Do(func() { close(resume) }); finishOnce.Do(func() { close(finish) }) }
					defer unblock()
					call := func(c context.Context) error {
						if prepared && c.Value(key{}) != "exact incoming context" {
							return errors.New("lost incoming authority context")
						}
						if !prepared && c.Value(key{}) != nil {
							return errors.New("legacy unexpectedly inherited request context")
						}
						attempt := usage.StartAttempt(c, "memory", "fixture")
						once.Do(func() {
							close(entered)
							select {
							case <-resume:
							case <-c.Done():
							}
						})
						attempt.Finish(usage.Completed)
						return c.Err()
					}
					cfg := config.Defaults()
					cfg.LocusMode = "open_only"
					cfg.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskCompaction: {Destination: config.DestinationLocal, Quality: config.CostEconomy}}
					cfg.Compaction.Enabled = true
					cfg.Compaction.ActivationFloorTokens = 1000
					cfg.Compaction.SegmentTokens = 4000
					cfg.Compaction.VerbatimRecent = 2
					deps := productionSchedulingDependencies{
						recapModel: "recap-fixture",
						recapRunner: schedulingRunner(func(c context.Context, r *agent.Request) (*agent.Response, error) {
							if r.ModelOverride != "recap-fixture" {
								return nil, errors.New("recap model lost")
							}
							if err := call(c); err != nil {
								return nil, err
							}
							return &agent.Response{Output: "fixture recap"}, nil
						}),
						loop: loopcompact.WiringDeps{Cfg: cfg, ChatModel: "fixture", Candidates: func() inference.Tiers { return inference.Tiers{Open: &schedulingProvider{call: call}} },
							BeginWork: func(c context.Context) (context.Context, func(), error) { return c, func() {}, nil },
						},
					}
					opts := ordinarySchedulingOptions()
					opts.recapDebounce = time.Millisecond
					opts.compactionDebounce = time.Millisecond
					opts.attempts = func(o usage.AttemptObservation) bool {
						if o.Outcome == usage.Completed {
							callbackOnce.Do(func() { close(callback); <-finish })
						}
						return true
					}
					var p productionScheduling
					if prepared {
						// Compile/use the exact opt-in seam; it must consume the supplied store.
						options := server.PreparedServerOptions{Scheduling: preparedProductionScheduling(deps, opts)}
						p.recap, p.compaction = options.Scheduling(st)
					} else {
						p = composeProductionScheduling(st, deps, opts)
					}
					defer func() {
						unblock()
						_ = p.recap.Close(context.Background())
						_ = p.compaction.Close(context.Background())
					}()
					admission := broker.NewAdmission("host")
					if err := admission.RegisterOwner(owner); err != nil {
						t.Fatal(err)
					}
					begin := func(c context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
						if o != owner || c.Value(key{}) != "exact incoming context" {
							return nil, nil, broker.ErrAdmissionStale
						}
						ticket, err := admission.AdmitWork(o, k)
						return c, ticket, err
					}
					if prepared {
						stale := owner
						stale.Epoch = "stale"
						if _, err := persistsvc.ScheduleOwnedRecap(owned, p.recap, stale, begin); !errors.Is(err, broker.ErrAdmissionStale) {
							t.Fatalf("stale: %v", err)
						}
						if producer == "recap" {
							_, err = persistsvc.ScheduleOwnedRecap(owned, p.recap, owner, begin)
						} else {
							_, err = persistsvc.ScheduleOwnedCompaction(owned, p.compaction, owner, begin)
						}
						if err != nil {
							t.Fatal(err)
						}
					} else if producer == "recap" {
						p.recap.Schedule("c")
					} else {
						p.compaction.Schedule("c")
					}
					select {
					case <-entered:
					case <-ctx.Done():
						t.Fatal("inference not reached", ctx.Err())
					}
					if prepared {
						if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
							t.Fatalf("inference blocker: %v", err)
						}
					}
					// Independent SQLite writer commits while inference is blocked.
					writeCtx, writeCancel := context.WithTimeout(other, time.Second)
					err = second.UpdateRecap(writeCtx, "sibling", "independent writer")
					writeCancel()
					if err != nil {
						t.Fatal(err)
					}
					resumeOnce.Do(func() { close(resume) })
					select {
					case <-callback:
					case <-ctx.Done():
						t.Fatal("final callback not reached", ctx.Err())
					}
					p.recap.Fence()
					p.compaction.Fence()
					if !shutdown {
						finishOnce.Do(func() { close(finish) })
						// Fence preserves active callbacks. Wait for the actual
						// durable result before cancellation/join.
						for {
							ready := false
							if producer == "recap" {
								info, e := st.Get(owned, "c")
								ready = e == nil && info.Title == "fixture recap"
							} else {
								compact, e := st.GetCompaction(owned, "c")
								ready = e == nil && compact.SegmentSummariesJSON != ""
							}
							if ready {
								break
							}
							select {
							case <-ctx.Done():
								t.Fatal("durable result missing", ctx.Err())
							case <-time.After(time.Millisecond):
							}
						}
					}
					joined := make(chan struct{})
					go func() {
						_ = p.recap.Close(context.Background())
						_ = p.compaction.Close(context.Background())
						close(joined)
					}()
					if shutdown {
						select {
						case <-joined:
							t.Fatal("joined before callback returned")
						case <-time.After(20 * time.Millisecond):
						}
					}
					if prepared && shutdown {
						if _, err := admission.CloseOwner(owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
							t.Fatalf("callback blocker: %v", err)
						}
					}
					finishOnce.Do(func() { close(finish) })
					select {
					case <-joined:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					if prepared {
						_, err := admission.CloseOwner(owner, nil)
						if shutdown && !errors.Is(err, broker.ErrAdmissionBusy) {
							t.Fatalf("cancelled write lost blocker: %v", err)
						}
						if !shutdown && err != nil {
							t.Fatal(err)
						}
					}
					if !shutdown && producer == "recap" {
						info, err := st.Get(owned, "c")
						if err != nil || info.Recap != "fixture recap" {
							t.Fatalf("recap: %+v %v", info, err)
						}
					} else if !shutdown {
						compact, err := st.GetCompaction(owned, "c")
						if err != nil || compact.SegmentSummariesJSON == "" {
							t.Fatalf("compaction: %+v %v", compact, err)
						}
					}
					info, err := wrong.Get(ctx, "c")
					if err != nil || info.Recap != "" {
						t.Fatalf("wrong store modified: %+v %v", info, err)
					}
					compact, err := wrong.GetCompaction(ctx, "c")
					if err != nil || compact.SegmentSummariesJSON != "" {
						t.Fatalf("wrong compaction store: %+v %v", compact, err)
					}
				})
			}
		}
	}
}
