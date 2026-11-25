package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/dispatch"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/taskrun"
	"cercano/source/server/pkg/config"
	pb "cercano/source/server/pkg/proto"
)

type realCompletionCloseProvider struct {
	scriptedProvider
	entered chan context.Context
	proceed chan struct{}
	outcome string
}

func (p *realCompletionCloseProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (llm.StreamReader, error) {
	select {
	case p.entered <- ctx:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-p.proceed:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	switch p.outcome {
	case "error":
		return nil, errors.New("fixture inference execution error")
	case "canceled":
		return nil, context.Canceled
	case "write-failure":
		persistsvc.ReportWriteFailure(ctx, errors.New("fixture persistence failure"))
	}
	return p.scriptedProvider.StreamChat(ctx, req)
}

// Characterization of a real taskrun terminal write and automatic owned wake.
// No test code advances or fails tickets; the barrier is inside inference.
func TestRealCompletionOrdinaryClose(t *testing.T) {
	for _, outcome := range []string{"success", "error", "canceled", "write-failure"} {
		t.Run(outcome, func(t *testing.T) {
			s, owned, owner, tasks := preparedWakeFixtureWithConfig(t, config.Config{OpenRuntime: "synthetic", CloudModel: "fixture"})
			// Remove the helper's unrelated pending notification using the durable API.
			if err := tasks.HandleTaskNotification(owned, owner.Conversation, "one"); err != nil {
				t.Fatal(err)
			}
			s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
			if err := s.ConfigurePreparedWorker(preparedWorkerPolicy(t)); err != nil {
				t.Fatal(err)
			}
			reg := agenttools.NewRegistry()
			reg.MustRegister(stubDispatchTool{name: "r_read", perm: agenttools.PermR, retText: "unused"})
			s.SetToolRegistry(reg)
			p := &realCompletionCloseProvider{scriptedProvider: scriptedProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "followup persisted"}}}}, entered: make(chan context.Context, 1), proceed: make(chan struct{}), outcome: outcome}
			s.SetCloudLLMProvider(p)
			// Exercise the production in-process runner, not the fixture's non-executable worker image.
			s.workerRunner = nil
			child := &scriptedProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "child persisted"}}}}
			s.taskManager.Close(context.Background())
			m, err := taskrun.NewPreparedOwnedCompletion(tasks, func(ctx context.Context, spec dispatch.Spec) (dispatch.Result, error) {
				return s.runAgenticDispatch(ctx, spec, inference.Selection{Provider: child}, "fixture")
			}, s.notifyOwnedTaskChange, s.coexistence.admission, s.resolvePreparedTaskOwner)
			if err != nil {
				t.Fatal(err)
			}
			s.taskManager = m
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			released := false
			defer func() {
				if !released {
					close(p.proceed)
				}
				m.Close(ctx)
			}()
			task, err := m.Launch(owned, dispatch.Spec{ConversationID: owner.Conversation, Task: "real completed child", Tools: []string{"r_read"}, MaxIterations: 2})
			if err != nil {
				t.Fatal(err)
			}
			var runctx context.Context
			select {
			case runctx = <-p.entered:
			case <-ctx.Done():
				t.Fatal("followup provider not entered", ctx.Err())
			}
			record, err := m.Inspect(ctx, owner.Conversation, task.ID)
			if err != nil || record.Status != "completed" || !record.WakeClaimed {
				t.Fatalf("durable task=%+v err=%v", record, err)
			}
			s.coexistence.options.ControlAuthorize = func(context.Context) error { return nil }
			attachment, e := s.coexistence.admission.AttachTerminal(owner, "terminal")
			if e != nil {
				t.Fatal(e)
			}
			defer attachment.Detach()
			before := s.coexistence.admission.Snapshot(owner)
			t.Logf("entered owner=%+v snapshot=%+v task=%+v", owner, before, record)
			_, err = s.LifecycleRelease(foregroundBinding(reproLoopbackContext(), owner), &pb.LifecycleControlRequest{Target: s.coexistence.controlIdentity(), Source: s.coexistence.controlIdentity(), Owner: wireOwner(owner)})
			if err != nil {
				t.Fatal(err)
			}
			close(p.proceed)
			released = true
			joined := make(chan struct{})
			go func() { m.Close(ctx); s.taskWakeWG.Wait(); close(joined) }()
			select {
			case <-joined:
			case <-ctx.Done():
				t.Fatal("task/wake teardown", ctx.Err())
			}
			after := s.coexistence.admission.Snapshot(owner)
			pending, err := tasks.PendingTasks(ctx, owner.Conversation)
			if err != nil {
				t.Fatal(err)
			}
			if after.HostWork[broker.PersistenceWork] != 1 || after.UnresolvedHostWork != 0 || !after.Closed {
				t.Fatalf("closure: %+v", after)
			}
			latest := s.latestCompletionTurn(owner.Conversation)
			if outcome == "success" || outcome == "write-failure" {
				if latest.Content != "followup persisted" {
					t.Fatalf("missing output: %+v", latest)
				}
			} else if latest.Role != "system" {
				t.Fatalf("unexpected output: %+v", latest)
			}
			wf := persistsvc.WriteFailure(runctx)
			t.Logf("outcome=%s WriteFailure=%v pending=%+v snapshot=%+v latest=%+v", outcome, wf, pending, after, s.latestCompletionTurn(owner.Conversation))
			if outcome == "write-failure" && wf == nil {
				t.Fatal("missing write failure control")
			}
			if outcome != "write-failure" && wf != nil {
				t.Fatalf("unexpected SQL/write failure: %v", wf)
			}
			if outcome == "success" {
				if len(pending) != 0 || after.UnresolvedWork != 0 {
					t.Fatalf("success retained: %+v %+v", pending, after)
				}
			} else {
				if len(pending) != 1 || !pending[0].WakeClaimed || after.UnresolvedWork != 1 || after.Work[broker.PersistenceWork] != 1 {
					t.Fatalf("failure characterization changed: %+v %+v", pending, after)
				}
			}
			_, sourceEndpoint := serveCutoverFixture(t, s)
			options := s.coexistence.options
			options.Host = "B"
			b, err := NewCoexistenceServer(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Shutdown()
			bc, endpoint := serveCutoverFixture(t, b)
			setResumeSource(b, s, sourceEndpoint)
			r := resumeRequest(b, endpoint)
			r.Owner.Conversation = owner.Conversation
			res, re := bc.LifecycleResume(ctx, r)
			t.Logf("ordinary resume=%+v error=%v source=%+v", res, re, s.coexistence.admission.Snapshot(owner))
			if outcome == "success" {
				if re != nil {
					t.Fatal(re)
				}
			} else if re == nil {
				t.Fatal("unexpected resume")
			}
		})
	}
}
