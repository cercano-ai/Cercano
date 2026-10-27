package server

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/taskrun"
	"cercano/source/server/pkg/config"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/worker"
)

func preparedWorkerPolicy(t *testing.T) worker.SpawnPolicy {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "pws-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gen, resources := filepath.Join(root, "gen"), filepath.Join(root, "res")
	for _, dir := range []string{gen, resources} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := "cercano"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	for name, text := range map[string]string{binary: "never executed", "manifest.json": `{"fixture":true}`} {
		if err := os.WriteFile(filepath.Join(gen, name), []byte(text), 0700); err != nil {
			t.Fatal(err)
		}
	}
	p, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: gen, ManifestPath: filepath.Join(gen, "manifest.json"), ResourceRoot: resources, HostInstance: "A"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPreparedWorkerConfiguration(t *testing.T) {
	if err := (&Server{}).ConfigurePreparedWorker(worker.SpawnPolicy{}); err == nil {
		t.Fatal("ordinary server accepted")
	}
	s, _, _, _ := preparedWakeFixture(t)
	s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
	if err := s.ConfigurePreparedWorker(worker.SpawnPolicy{}); err == nil || s.workerRunner != nil {
		t.Fatal("legacy policy installed")
	}
	policy := preparedWorkerPolicy(t)
	if err := s.ConfigurePreparedWorker(policy); err != nil {
		t.Fatal(err)
	}
	if s.workerRunner == nil {
		t.Fatal("runner not installed")
	}
	if err := s.ConfigurePreparedWorker(policy); err == nil {
		t.Fatal("runner replaced")
	}
}

func TestPreparedWorkerAuthorityConservativeCompletion(t *testing.T) {
	for _, mode := range []string{"success", "turn-error", "write-error"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, owner, _ := preparedWakeFixture(t)
			workctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			a := &preparedWorkerAuthority{server: s, ctx: workctx, id: owner.Conversation, ticket: ticket}
			if _, err := a.Context(context.Background(), owner.Conversation); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("ambient context authorized: %v", err)
			}
			if _, err := a.Context(ctx, "child"); err == nil {
				t.Fatal("arbitrary child authorized")
			}
			if _, err := a.Context(ctx, owner.Conversation); err != nil {
				t.Fatal(err)
			}
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("active ticket released: %v", err)
			}
			var turnErr error
			if mode == "turn-error" {
				turnErr = errors.New("worker failed")
			}
			if mode == "write-error" {
				persistsvc.ReportWriteFailure(workctx, errors.New("durable write failed"))
			}
			a.Close(turnErr)
			err = s.ReleaseRetained(ctx, owner, nil)
			if mode == "success" && err != nil {
				t.Fatalf("successful ticket not completed: %v", err)
			}
			if mode != "success" && err == nil {
				t.Fatal("failed turn advertised closure readiness")
			}
		})
	}
}

func TestPreparedScopedDispatch(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
	if err := s.ConfigurePreparedWorker(preparedWorkerPolicy(t)); err != nil {
		t.Fatal(err)
	}
	reg := agenttools.NewRegistry()
	reg.MustRegister(stubDispatchTool{name: "r_read", perm: agenttools.PermR, retText: "ok"})
	s.SetToolRegistry(reg)
	p := &scriptedProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "finished"}}}}
	spec := dispatch.Spec{Task: "inspect", ConversationID: owner.Conversation, SubConversationID: "independent", Tools: []string{"r_read"}, MaxIterations: 2,
		Emit: func(ev agenttools.ProgressEvent) {
			if ev.Kind == "done" {
				state, err := conversation.ReadOwnership(ctx, s.coexistence.store, "independent")
				if err != nil || state.State != "released" {
					t.Errorf("done before release: %v %v", state, err)
				}
				return
			}
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Errorf("parent escaped callback barrier: %v", err)
			}
		}}
	if _, err := s.runAgenticDispatch(context.Background(), spec, inference.Selection{Provider: p}, "fixture"); err == nil || p.calls != 0 {
		t.Fatal("ambient dispatch not refused before provider")
	}
	result, err := s.runAgenticDispatch(ctx, spec, inference.Selection{Provider: p}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if result.SubConversationID != "independent" || p.calls != 1 {
		t.Fatalf("result=%+v calls=%d", result, p.calls)
	}
	state, err := conversation.ReadOwnership(ctx, s.coexistence.store, "independent")
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "released" {
		t.Fatalf("child not durably released: %+v", state)
	}
	info, err := s.coexistence.store.Get(ctx, "independent")
	if err != nil || info.Kind != "subagent" || info.ParentID != owner.Conversation {
		t.Fatalf("child row=%+v err=%v", info, err)
	}
	turns, err := s.coexistence.store.GetTurns(ctx, "independent")
	if err != nil || len(turns) < 2 {
		t.Fatalf("missing child transcript: %v %v", turns, err)
	}
	if s.coexistence.owners["independent"] != nil {
		t.Fatal("child lock retained on success")
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedDispatchFailureRetainsScope(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	scoped, complete, err := s.beginPreparedDispatch(ctx, owner.Conversation, "failed-child")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.beginPreparedDispatch(scoped, owner.Conversation, "nested"); err == nil {
		t.Fatal("nested dispatch authorized")
	}
	if _, _, err := s.beginPreparedDispatch(ctx, owner.Conversation, "failed-child"); err == nil {
		t.Fatal("adopted active child")
	}
	persistsvc.ReportWriteFailure(scoped, errors.New("write failed"))
	if complete(nil) == nil {
		t.Fatal("failure lost")
	}
	if s.coexistence.owners["failed-child"] == nil {
		t.Fatal("capability lost")
	}
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("parent not blocked: %v", err)
	}
	_ = s.coexistence.owners["failed-child"].Close()
}

// Exercise routing through the real dispatch engine, not the server runner shim.
func TestPreparedDispatchEngineFinalization(t *testing.T) {
	for _, mode := range []string{"append", "release", "panic"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, owner, _ := preparedWakeFixture(t)
			s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
			if err := s.ConfigurePreparedWorker(preparedWorkerPolicy(t)); err != nil {
				t.Fatal(err)
			}
			reg := agenttools.NewRegistry()
			reg.MustRegister(stubDispatchTool{name: "r_read", perm: agenttools.PermR, retText: "ok"})
			s.SetToolRegistry(reg)
			p := &scriptedProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "finished"}}}}
			engine := dispatch.NewEngine(func() inference.Tiers {
				return inference.Tiers{Open: p, Destinations: map[config.Destination]inference.Candidate{config.Destination("secondary"): {Provider: p}}}
			}, func() locus.Mode { return locus.Mode("local") }, nil)
			engine.SetDestinationModelFor(func(inference.Selection, config.Tier) string { return "fixture" })
			s.toolSvc.SetEngine(engine)
			db, err := sql.Open("sqlite", s.coexistence.options.StorePath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			query := ""
			if mode == "append" {
				query = `CREATE TRIGGER fail_evidence BEFORE INSERT ON dispatch_events BEGIN SELECT RAISE(FAIL, 'injected append failure'); END`
			}
			if mode == "release" {
				query = `CREATE TRIGGER fail_release BEFORE UPDATE ON conversation_ownership WHEN NEW.conversation_id='probe-child' AND NEW.state='released' BEGIN SELECT RAISE(FAIL, 'injected release failure'); END`
			}
			if query != "" {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			done, errorEvent := false, false
			spec := dispatch.Spec{Mode: dispatch.Agentic, Task: "inspect", ConversationID: owner.Conversation, SubConversationID: "probe-child", MaxIterations: 2, Tools: []string{"r_read"},
				Emit: func(ev agenttools.ProgressEvent) {
					if mode == "panic" && ev.Kind == "started" {
						panic("progress panic")
					}
					done = done || ev.Kind == "done"
					errorEvent = errorEvent || ev.Kind == "error"
				}}
			var runErr error
			var panicked any
			func() { defer func() { panicked = recover() }(); _, runErr = engine.Dispatch(ctx, spec) }()
			if mode == "panic" {
				if panicked != "progress panic" {
					t.Fatalf("panic changed: %v", panicked)
				}
			} else if runErr == nil || !errorEvent {
				t.Errorf("failure not surfaced: err=%v errorEvent=%v", runErr, errorEvent)
			}
			if done {
				t.Error("published done before durable finalization")
			}
			if s.coexistence.owners["probe-child"] == nil {
				t.Error("failed scope released child")
			} else {
				t.Cleanup(func() { _ = s.coexistence.owners["probe-child"].Close() })
			}
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Errorf("failed scope lost blocker: %v", err)
			}
		})
	}
}

type preparedFailureProvider struct {
	*scriptedProvider
	mode    string
	started chan struct{}
}

func (p preparedFailureProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (llm.StreamReader, error) {
	switch p.mode {
	case "provider-panic":
		panic("provider panic")
	case "cancel":
		close(p.started)
		<-ctx.Done()
		return nil, ctx.Err()
	case "backend":
		return nil, errors.New("backend unavailable")
	}
	return p.scriptedProvider.StreamChat(ctx, req)
}

func TestPreparedTaskrunEngineFailureAndIsolation(t *testing.T) {
	for _, mode := range []string{"provider-panic", "cancel", "backend"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, owner, tasks := preparedWakeFixture(t)
			s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
			if err := s.ConfigurePreparedWorker(preparedWorkerPolicy(t)); err != nil {
				t.Fatal(err)
			}
			reg := agenttools.NewRegistry()
			reg.MustRegister(stubDispatchTool{name: "r_read", perm: agenttools.PermR, retText: "ok"})
			s.SetToolRegistry(reg)
			provider := preparedFailureProvider{&scriptedProvider{caps: inference.Capabilities{SupportsTools: true}}, mode, make(chan struct{})}
			engine := dispatch.NewEngine(func() inference.Tiers {
				return inference.Tiers{Destinations: map[config.Destination]inference.Candidate{config.Destination("secondary"): {Provider: provider}}}
			}, func() locus.Mode { return locus.OpenOnly }, nil)
			engine.SetDestinationModelFor(func(inference.Selection, config.Tier) string { return "fixture" })
			s.toolSvc.SetEngine(engine)
			m, err := taskrun.NewPrepared(tasks, engine.Dispatch, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close(context.Background())
			launched, err := m.Launch(ctx, dispatch.Spec{Mode: dispatch.Agentic, ConversationID: owner.Conversation, Task: "inspect", Tools: []string{"r_read"}, MaxIterations: 2})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "cancel" {
				select {
				case <-provider.started:
				case <-time.After(5 * time.Second):
					t.Fatal("provider not entered")
				}
				if err := m.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			waitctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			rec, err := m.Wait(waitctx, owner.Conversation, launched.ID)
			if err != nil || (rec.Status != "failed" && !(mode == "cancel" && rec.Status == "interrupted")) {
				t.Fatalf("terminal task=%+v err=%v", rec, err)
			}
			child := s.coexistence.owners[launched.ChildID]
			if mode == "provider-panic" || mode == "cancel" {
				if child == nil {
					t.Fatal("panic released child before TaskManager recovery")
				}
				defer child.Close()
				if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("panic lost blocker: %v", err)
				}
			} else if child != nil {
				defer child.Close()
				t.Fatal("durably recorded returned backend error retained child")
			}
			// A failed dispatch must not poison either a same-parent sibling or a
			// distinct parent's retained work, even when the failed child stays held.
			sibling, complete, err := s.beginPreparedDispatch(ctx, owner.Conversation, "sibling")
			if err != nil {
				t.Fatal(err)
			}
			if persistsvc.WriteFailure(sibling) != nil {
				t.Fatal("same-parent failure leak")
			}
			if err := complete(nil); err != nil {
				t.Fatal(err)
			}
			other, err := s.Retain(ctx, "other-parent")
			if err != nil {
				t.Fatal(err)
			}
			cap := s.coexistence.owners["other-parent"]
			defer cap.Close()
			otherctx := WithPreparedTaskOwnership(conversation.WithOwnership(ctx, cap), other, cap)
			independent, finish, err := s.beginPreparedDispatch(otherctx, "other-parent", "other-child")
			if err != nil {
				t.Fatal(err)
			}
			if persistsvc.WriteFailure(independent) != nil {
				t.Fatal("distinct-parent failure leak")
			}
			if err := finish(nil); err != nil {
				t.Fatal(err)
			}
			if err := s.ReleaseRetained(otherctx, other, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
