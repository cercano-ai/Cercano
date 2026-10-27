package server

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
