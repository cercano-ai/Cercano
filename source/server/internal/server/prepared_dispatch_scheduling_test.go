package server

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/recap"
	"cercano/source/server/internal/taskrun"
	"cercano/source/server/pkg/config"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPreparedDispatchChildScheduling(t *testing.T) {
	s, ctx, owner, tasks := preparedWakeFixture(t, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
		return recap.New(st, func(context.Context, string) (string, error) { return "fixture recap", nil }, time.Millisecond, 10), nil
	})
	defer s.preparedScheduling.close()
	s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
	if err := s.ConfigurePreparedWorker(preparedWorkerPolicy(t)); err != nil {
		t.Fatal(err)
	}
	reg := agenttools.NewRegistry()
	reg.MustRegister(stubDispatchTool{name: "r_read", perm: agenttools.PermR, retText: "ok"})
	s.SetToolRegistry(reg)
	p := &scriptedProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "FIXTURE_OK child"}}}}
	engine := dispatch.NewEngine(func() inference.Tiers {
		return inference.Tiers{Destinations: map[config.Destination]inference.Candidate{config.Destination("secondary"): {Provider: p}}}
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
	defer func() {
		if cap := s.coexistence.owners[launched.ChildID]; cap != nil {
			_ = cap.Close()
		}
	}()
	waitctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rec, err := m.Wait(waitctx, owner.Conversation, launched.ID)
	if err != nil || rec.Status != "completed" {
		t.Fatalf("task status=%s error=%s result=%s wait=%v", rec.Status, rec.Error, rec.ResultJSON, err)
	}
	info, err := s.coexistence.store.Get(ctx, launched.ChildID)
	if err != nil || info.Recap != "fixture recap" {
		t.Fatalf("recap not committed: %+v %v", info, err)
	}
	state, err := conversation.ReadOwnership(ctx, s.coexistence.store, launched.ChildID)
	if err != nil || state.State != "released" {
		t.Fatalf("child not released: %+v %v", state, err)
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedDispatchJoinsChildFinalCallback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "write-failure"}[fail], func(t *testing.T) {
			entered, finish := make(chan struct{}), make(chan struct{})
			s, ctx, owner, _ := preparedWakeFixture(t, func(st conversation.Store) (*recap.Generator, *compactiongen.Generator) {
				c := compactiongen.New(st, nil, compactor.Config{}, nil, 0)
				c.SetEnabled(true)
				c.SetToolElisionOnly(true)
				c.SetElideOnlyFn(func(ctx context.Context, id string) (int, int, int, bool, error) {
					if id != "scheduled-child" {
						t.Errorf("wrong child: %s", id)
					}
					return 100, 10, 1, true, nil
				})
				c.SetContextUsageFn(func(ctx context.Context, _ string, _, _ int) {
					close(entered)
					<-finish
					if fail {
						persistsvc.ReportWriteFailure(ctx, errors.New("child callback write failed"))
					}
				})
				return nil, c
			})
			released := false
			defer func() {
				if !released {
					close(finish)
				}
				s.preparedScheduling.close()
				if cap := s.coexistence.owners["scheduled-child"]; cap != nil {
					_ = cap.Close()
				}
			}()
			scoped, complete, err := s.beginPreparedDispatch(ctx, owner.Conversation, "scheduled-child")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.agent.ScheduleCompactionContext(ctx, "scheduled-child"); !errors.Is(err, broker.ErrAdmissionStale) {
				t.Fatalf("parent authorized child: %v", err)
			}
			if err := s.agent.ScheduleCompactionContext(scoped, "scheduled-child"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("callback not entered")
			}
			joined := make(chan error, 1)
			go func() { joined <- complete(nil) }()
			select {
			case err := <-joined:
				t.Fatalf("released during callback: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("parent escaped: %v", err)
			}
			state, err := conversation.ReadOwnership(ctx, s.coexistence.store, "scheduled-child")
			if err != nil || state.State == "released" {
				t.Fatalf("child released early: %+v %v", state, err)
			}
			close(finish)
			released = true
			select {
			case err := <-joined:
				if fail && (err == nil || !strings.Contains(err.Error(), "child callback write failed")) {
					t.Fatalf("write failure lost: %v", err)
				}
				if !fail && err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("did not join")
			}
			if fail {
				state, err := conversation.ReadOwnership(ctx, s.coexistence.store, "scheduled-child")
				if err != nil || state.State == "released" {
					t.Fatalf("failed child released: %+v %v", state, err)
				}
				if err := s.agent.ScheduleCompactionContext(scoped, "scheduled-child"); !errors.Is(err, broker.ErrAdmissionClosed) {
					t.Fatalf("closed dispatch admitted more work: %v", err)
				}
			}
			err = s.ReleaseRetained(ctx, owner, nil)
			if fail && !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("failure blocker lost: %v", err)
			}
			if !fail && err != nil {
				t.Fatal(err)
			}
		})
	}
}
