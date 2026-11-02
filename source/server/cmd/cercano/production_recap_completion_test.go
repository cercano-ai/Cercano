package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/broker"
	projectctx "cercano/source/server/internal/context"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/engine"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/loopcompact"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/toolstack"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
)

type recapCompletionProvider struct {
	name string
	call func(context.Context, inference.Call) (inference.Result, error)
}

func (p *recapCompletionProvider) Name() string { return p.name }
func (p *recapCompletionProvider) Capabilities() inference.Capabilities {
	return inference.Capabilities{SupportsTools: true}
}
func (p *recapCompletionProvider) Chat(ctx context.Context, c inference.Call) (inference.Result, error) {
	return p.call(ctx, c)
}
func (p *recapCompletionProvider) StreamChat(ctx context.Context, c inference.Call) (inference.Stream, error) {
	r, err := p.call(ctx, c)
	if err != nil {
		return nil, err
	}
	return &recapCompletionStream{events: []llm.StreamEvent{
		{Type: llm.EventMessageStart},
		{Type: llm.EventTextDelta, TextDelta: r.Blocks[0].Text},
		{Type: llm.EventMessageStop, StopReason: "end_turn"},
	}}, nil
}

type recapCompletionStream struct{ events []llm.StreamEvent }

func (s *recapCompletionStream) Close() error { return nil }
func (s *recapCompletionStream) Next() (llm.StreamEvent, bool, error) {
	if len(s.events) == 0 {
		return llm.StreamEvent{}, false, nil
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, true, nil
}

// Uses the shared ordinary-production service composition and the server-owned
// prepared task manager, dispatch scope and scheduler. No synthetic executor or
// manually constructed ScheduledWork substitutes for dispatch's completion closure.
func TestProductionRecapTaskCompletion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	for _, mode := range []string{"inference", "write", "uncertain", "executor"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			dir, resolveErr := filepath.EvalSymlinks(t.TempDir())
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "tasks.db")
			if err := conversation.PrepareDevelopmentStore(ctx, path); err != nil {
				t.Fatal(err)
			}
			cfg := config.Defaults()
			cfg.OpenRuntime = "synthetic"
			cfg.LocusMode = "cloud_primary"
			cfg.Compaction.Enabled = true
			cfg.CloudProfiles = []config.CloudProfile{{Name: "mock-account", Provider: "mock-vendor"}}
			cfg.ActiveCloudProfile = "mock-account"
			cfg.ModelProfiles.Cloud.Providers = map[string]config.VendorCostTiers{"mock-vendor": {Economy: config.CostTierModel{Model: "mock-primary-model"}}}
			// Recap shares the configured Compaction route: this must reach the
			// local mock while Dispatch reaches cloud primary through the same live graph.
			cfg.TaskAssignments = map[config.Task]config.TaskAssignment{config.TaskCompaction: {Destination: config.DestinationLocal, Quality: config.CostEconomy}, config.TaskDispatch: {Destination: config.DestinationPrimary, Quality: config.CostEconomy}}
			routing := composeProductionRouting(productionRoutingDependencies{open: agent.AbsentCloud("fixture"), cloud: agent.AbsentCloud("fixture"), classificationDisabled: true})
			var a *agent.Agent
			var loader *projectctx.Loader
			var s *server.Server
			var err error
			s, err = newProductionPreparedServer(ctx, server.PreparedServerOptions{
				Config: cfg, ConfigPath: filepath.Join(dir, "config.yaml"), Secrets: secrets.NewMemory(), RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
				Coexistence: server.CoexistenceOptions{StorePath: path, Host: "fixture", Build: "test", Contract: conversation.DevelopmentCoexistenceDeclaration(), CloseParticipation: func(context.Context) error { return nil }},
				Agent: func(st conversation.Store) (*agent.Agent, error) {
					a, loader, err = composeProductionAgent(routing, st, cfg, productionScheduling{})
					return a, err
				},
			}, routing, nil, engine.NewEngineRegistry(), productionSchedulingDependencies{loop: loopcompact.WiringDeps{Cfg: cfg, Candidates: func() inference.Tiers { return s.DispatchCandidates() }}}, productionSchedulingOptions{recapDebounce: time.Millisecond, recapMaxTurns: 12})
			if err != nil {
				t.Fatal(err)
			}
			defer s.CloseCoexistenceStore()
			defer s.Shutdown()
			entered, resume := make(chan struct{}), make(chan struct{})
			var once, release sync.Once
			unblock := func() { release.Do(func() { close(resume) }) }
			defer unblock()
			primary := &recapCompletionProvider{name: "mock-primary", call: func(context.Context, inference.Call) (inference.Result, error) {
				if mode == "executor" {
					return inference.Result{}, errors.New("genuine primary executor failure")
				}
				return inference.Result{Blocks: []llm.Block{{Type: llm.BlockText, Text: "successful cloud primary result"}}}, nil
			}}
			secondary := &recapCompletionProvider{name: "mock-configured-recap", call: func(c context.Context, _ inference.Call) (inference.Result, error) {
				once.Do(func() { close(entered) })
				select {
				case <-resume:
				case <-c.Done():
					return inference.Result{}, c.Err()
				}
				if mode == "uncertain" {
					persistsvc.ReportWriteFailure(c, errors.New("uncertain recap persistence"))
				}
				if mode != "write" {
					return inference.Result{}, errors.New("recap inference unavailable")
				}
				return inference.Result{Blocks: []llm.Block{{Type: llm.BlockText, Text: "generated recap"}}}, nil
			}}
			s.SetCloudLLMProvider(primary)
			s.SetOpenLLMProvider(secondary)
			bindProductionAgent(s, a, loader, agent.NewStaticPermissionStore(agent.ModeBypass), agent.NewPendingDecisions())
			o, _ := developmentProductionFixture(t)
			spawn, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: o.Generation, ManifestPath: filepath.Join(o.Generation, "manifest.json"), HostInstance: o.Host, ResourceRoot: o.RuntimeRoot})
			if err != nil {
				t.Fatal(err)
			}
			policy := productionExecutionPolicy{prepared: &spawn}
			if err := composeProductionServices(s, a, loader, toolstack.EngineDeps{}, policy); err != nil {
				t.Fatal(err)
			}
			if err := policy.install(s); err != nil {
				t.Fatal(err)
			} // Installs production dispatch scoping; no executable is invoked below.
			owner, err := s.Retain(ctx, "parent")
			if err != nil {
				t.Fatal(err)
			}
			owned, ticket, err := s.BeginRetainedWork(ctx, owner, broker.ForegroundWork)
			if err != nil {
				t.Fatal(err)
			}
			defer ticket.Complete()
			// Completion-turn delivery is outside this child-dispatch test. Deny its
			// separate admission so the pinned worker executable is never launched.
			s.SetBackgroundWorkAdmission(func(context.Context) (context.Context, func(), error) {
				return nil, nil, broker.ErrAdmissionClosed
			})

			if mode == "write" {
				db, e := sql.Open("sqlite", path)
				if e != nil {
					t.Fatal(e)
				}
				defer db.Close()
				if _, e = db.ExecContext(ctx, `CREATE TRIGGER fail_recap BEFORE UPDATE OF recap ON conversations BEGIN SELECT RAISE(FAIL, 'actual recap write failure'); END`); e != nil {
					t.Fatal(e)
				}
			}
			tool, ok := s.ToolRegistry().Get("dispatch")
			if !ok {
				t.Fatal("dispatch missing")
			}
			launched, err := tool.Execute(agenttools.WithConversationID(owned, "parent"), json.RawMessage(`{"task":"inspect this fixture","tools":["Read"]}`))
			if err != nil {
				t.Fatal(err)
			}
			var ids struct {
				ID    string `json:"task_id"`
				Child string `json:"child_id"`
			}
			if err = json.Unmarshal([]byte(launched.Text), &ids); err != nil {
				t.Fatal(err)
			}
			tasks := a.PersistentStore().(conversation.TaskStore)
			if mode != "executor" {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("configured recap provider not reached")
				}
				rec, e := tasks.GetTask(ctx, "parent", ids.ID)
				if e != nil {
					t.Fatal(e)
				}
				if rec.Status == "completed" || rec.Status == "failed" {
					t.Fatalf("task became terminal before recap joined: %s", rec.Status)
				}
				state, e := conversation.ReadOwnership(ctx, a.PersistentStore(), ids.Child)
				if e != nil || state.State == "released" {
					t.Fatalf("early child release: %+v %v", state, e)
				}
				unblock()
			}
			var rec conversation.TaskRecord
			for {
				rec, err = tasks.GetTask(ctx, "parent", ids.ID)
				if err != nil {
					t.Fatal(err)
				}
				if rec.Status == "completed" || rec.Status == "failed" {
					break
				}
				select {
				case <-time.After(time.Millisecond):
				case <-ctx.Done():
					t.Fatalf("task never terminal: %+v", rec)
				}
			}
			want := "failed"
			if mode == "inference" {
				want = "completed"
			}
			if rec.Status != want {
				t.Fatalf("status=%s want=%s error=%s result=%s", rec.Status, want, rec.Error, rec.ResultJSON)
			}
			if mode == "write" && !strings.Contains(rec.Error, "actual recap write failure") {
				t.Fatalf("missing SQL failure: %s", rec.Error)
			}
			if mode == "uncertain" && !strings.Contains(rec.Error, "uncertain recap persistence") {
				t.Fatalf("missing uncertain persistence failure: %s", rec.Error)
			}
			if mode != "executor" {
				var result dispatch.Result
				if err = json.Unmarshal(rec.ResultJSON, &result); err != nil {
					t.Fatal(err)
				}
				if result.Text != "successful cloud primary result" || !result.IsCloud {
					t.Fatalf("primary result lost: %+v", result)
				}
			} else if !strings.Contains(rec.Error, "genuine primary executor failure") {
				t.Fatalf("executor error swallowed: %+v", rec)
			}
			info, err := a.PersistentStore().Get(ctx, ids.Child)
			if err != nil {
				t.Fatal(err)
			}
			if info.Recap != "" {
				t.Fatalf("failed recap persisted: %q", info.Recap)
			}
			state, err := conversation.ReadOwnership(ctx, a.PersistentStore(), ids.Child)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "inference" && state.State != "released" {
				t.Fatalf("child accounting did not drain: %+v", state)
			}
			if (mode == "write" || mode == "uncertain") && state.State == "released" {
				t.Fatal("unsafe persistence release")
			}
		})
	}
}
