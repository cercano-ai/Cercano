package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
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
	case "canceled-write-failure":
		persistsvc.ReportWriteFailure(ctx, errors.New("fixture persistence failure"))
		return nil, context.Canceled
	case "canceled":
		return nil, context.Canceled
	case "write-failure":
		persistsvc.ReportWriteFailure(ctx, errors.New("fixture persistence failure"))
	}
	return p.scriptedProvider.StreamChat(ctx, req)
}

// Regression from the historic characterization: joined inference failure is not a failed write.
// No test code advances or fails tickets; the barrier is inside inference.
func TestRealCompletionOrdinaryClose(t *testing.T) {
	for _, outcome := range []string{"success", "error", "canceled", "write-failure", "canceled-write-failure", "ack-write-failure", "output-write-failure", "ensure-write-failure"} {
		t.Run(outcome, func(t *testing.T) {
			uncertain := outcome == "ensure-write-failure" || outcome == "write-failure" || outcome == "canceled-write-failure" || outcome == "ack-write-failure" || outcome == "output-write-failure"
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
				result, err := s.runAgenticDispatch(ctx, spec, inference.Selection{Provider: child}, "fixture")
				if err == nil && outcome == "ensure-write-failure" {
					db, openErr := sql.Open("sqlite", s.coexistence.options.StorePath)
					if openErr != nil {
						return result, openErr
					}
					defer db.Close()
					_, err = db.Exec(fmt.Sprintf("CREATE TRIGGER ensure_failure BEFORE INSERT ON conversations WHEN NEW.id = '%s' BEGIN SELECT RAISE(ABORT, 'real ensure failure'); END", owner.Conversation))
				}
				return result, err
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
			if outcome == "ack-write-failure" || outcome == "output-write-failure" {
				db, err := sql.Open("sqlite", s.coexistence.options.StorePath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				table, operation, when := "delegated_tasks", "UPDATE", "WHEN NEW.notification_pending = 0"
				if outcome == "output-write-failure" {
					table, operation, when = "turns", "INSERT", "WHEN NEW.role = 'assistant'"
				}
				if _, err := db.Exec(fmt.Sprintf("CREATE TRIGGER completion_failure BEFORE %s ON %s %s BEGIN SELECT RAISE(ABORT, 'real completion mutation failure'); END", operation, table, when)); err != nil {
					t.Fatal(err)
				}
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
			if outcome == "success" || outcome == "write-failure" || outcome == "ack-write-failure" {
				if latest.Content != "followup persisted" {
					t.Fatalf("missing output: %+v", latest)
				}
			} else if outcome == "ensure-write-failure" {
				if latest.Content != "" {
					t.Fatalf("setup failure unexpectedly persisted: %+v", latest)
				}
			} else if latest.Role != "system" {
				t.Fatalf("unexpected output: %+v", latest)
			}
			wf := persistsvc.WriteFailure(runctx)
			t.Logf("outcome=%s WriteFailure=%v pending=%+v snapshot=%+v latest=%+v", outcome, wf, pending, after, s.latestCompletionTurn(owner.Conversation))
			if uncertain && wf == nil {
				t.Fatal("missing write failure control")
			}
			if !uncertain && wf != nil {
				t.Fatalf("unexpected SQL/write failure: %v", wf)
			}
			if outcome == "success" {
				if len(pending) != 0 || after.UnresolvedWork != 0 {
					t.Fatalf("success retained: %+v %+v", pending, after)
				}
			} else {
				wantUnresolved := 0
				if uncertain {
					wantUnresolved = 1
				}
				if len(pending) != 1 || !pending[0].WakeClaimed || after.UnresolvedWork != wantUnresolved || after.Work[broker.PersistenceWork] != wantUnresolved {
					t.Fatalf("pending state or uncertainty accounting: %+v %+v", pending, after)
				}
			}
			_, sourceEndpoint := serveCutoverFixture(t, s)
			options := s.coexistence.options
			options.Host = "B"
			dir := t.TempDir()
			b, err := NewPreparedDurableServer(ctx, PreparedServerOptions{
				Coexistence: options, Config: config.Config{OpenRuntime: "synthetic", CloudModel: "fixture"},
				ConfigPath: filepath.Join(dir, "never-read"), RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
				Agent: func(st conversation.Store) (*agent.Agent, error) {
					return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
				},
			}, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Shutdown()
			b.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
			if err := b.ConfigurePreparedWorker(preparedWorkerPolicy(t)); err != nil {
				t.Fatal(err)
			}
			b.SetToolRegistry(reg)
			b.workerRunner = nil
			nextProvider := &scriptedProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "human response"}}}}
			b.SetCloudLLMProvider(nextProvider)
			bc, endpoint := serveCutoverFixture(t, b)
			setResumeSource(b, s, sourceEndpoint)
			r := resumeRequest(b, endpoint)
			r.Owner.Conversation = owner.Conversation
			res, re := bc.LifecycleResume(ctx, r)
			t.Logf("ordinary resume=%+v error=%v source=%+v", res, re, s.coexistence.admission.Snapshot(owner))
			if !uncertain {
				if re != nil {
					t.Fatal(re)
				}
				newOwner := ownerOf(b.coexistence.owners[owner.Conversation])
				if newOwner == owner {
					t.Fatal("owner epoch did not change")
				}
				if err := tasks.HandleTaskNotification(owned, owner.Conversation, task.ID); err == nil {
					t.Fatal("old owner acknowledged after transfer")
				}
				// Resume is idempotent and must not schedule the claimed notification.
				for i := 0; i < 3; i++ {
					if _, err := bc.LifecycleResume(ctx, r); err != nil {
						t.Fatal(err)
					}
				}
				b.taskWakeWG.Wait()
				if nextProvider.calls != 0 {
					t.Fatalf("resume ran inference: %d", nextProvider.calls)
				}
				currentTasks := b.persistSvc.Store().(conversation.TaskStore)
				if outcome != "success" {
					pending, err := currentTasks.PendingTasks(ctx, owner.Conversation)
					if err != nil || len(pending) != 1 || !pending[0].WakeClaimed {
						t.Fatalf("resume changed pending state: %+v %v", pending, err)
					}
					// First human turn succeeds, but its notification followup fails at the real
					// provider (script exhausted). No mutation uncertainty may fence this owner.
					req := &pb.ProcessRequestRequest{ConversationId: owner.Conversation, Input: "check the result"}
					_ = b.StreamProcessRequest(req, newCollectingStream(foregroundBinding(ctx, newOwner)))
					pending, err = currentTasks.PendingTasks(ctx, owner.Conversation)
					if err != nil || len(pending) != 1 || !pending[0].WakeClaimed {
						t.Fatalf("failed human handling lost evidence: %+v %v", pending, err)
					}
					if snap := b.coexistence.admission.Snapshot(newOwner); snap.UnresolvedWork != 0 {
						t.Fatalf("human inference failure fenced ownership: %+v", snap)
					}
					db, err := sql.Open("sqlite", options.StorePath)
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					if _, err := db.Exec(`CREATE TABLE ack_audit (task TEXT); CREATE TRIGGER count_ack AFTER UPDATE ON delegated_tasks WHEN OLD.notification_pending = 1 AND NEW.notification_pending = 0 BEGIN INSERT INTO ack_audit VALUES (NEW.id); END`); err != nil {
						t.Fatal(err)
					}
					handled := &scriptedProvider{caps: inference.Capabilities{SupportsTools: true}, scripts: [][]llm.Block{{{Type: llm.BlockText, Text: "human again"}}, {{Type: llm.BlockText, Text: "handled pending child"}}}}
					b.SetCloudLLMProvider(handled)
					if err := b.StreamProcessRequest(req, newCollectingStream(foregroundBinding(ctx, newOwner))); err != nil {
						t.Fatal(err)
					}
					if handled.calls != 2 || !strings.Contains(fmt.Sprint(handled.seen[1]), task.ID) {
						t.Fatalf("human boundary did not observe child result: %+v", handled.seen)
					}
					pending, err = currentTasks.PendingTasks(ctx, owner.Conversation)
					if err != nil || len(pending) != 0 {
						t.Fatalf("not acknowledged: %+v %v", pending, err)
					}
					var count int
					if err := db.QueryRow(`SELECT count(*) FROM ack_audit`).Scan(&count); err != nil || count != 1 {
						t.Fatalf("ack count=%d err=%v", count, err)
					}
				}

			} else if re == nil {
				t.Fatal("unexpected resume")
			}
		})
	}
}
