package server

import (
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/broker"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/runner"
	"cercano/source/server/pkg/proto"
	"context"
	"errors"
	"google.golang.org/grpc/metadata"
	"sync/atomic"
	"testing"
)

func foregroundBinding(ctx context.Context, o broker.Owner) context.Context {
	return metadata.NewIncomingContext(ctx, metadata.Pairs("cercano-host-instance", o.Host, "cercano-conversation-id", o.Conversation, "cercano-ownership-epoch", o.Epoch, "cercano-terminal-attachment", "terminal"))
}

func TestPreparedForegroundExecutesAndRetains(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "write-failure"}[fail], func(t *testing.T) {
			s, ctx, owner, tasks := preparedWakeFixture(t)
			s.SetCloudLLMProvider(&scriptedProvider{})
			s.toolSvc.SetRegistry(agenttools.NewRegistry())
			entered, proceed := make(chan struct{}), make(chan struct{})
			var runs atomic.Int32
			s.inProcessRunner = ownedWakeRunner(func(runctx context.Context, persist runner.PersistFunc) (runner.Result, error) {
				if _, err := s.resolvePreparedTaskOwner(runctx, "parent"); err != nil {
					t.Error(err)
				}
				if runs.Add(1) == 1 {
					close(entered)
					<-proceed
				}
				if fail {
					persistsvc.ReportWriteFailure(runctx, errors.New("disk"))
				} else {
					persist(llm.Message{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "done"}}})
				}
				return runner.Result{FinalText: "done", Model: "synthetic"}, nil
			})
			done := make(chan error, 1)
			go func() {
				done <- s.StreamProcessRequest(&proto.ProcessRequestRequest{ConversationId: "parent", Input: "hello"}, newCollectingStream(foregroundBinding(ctx, owner)))
			}()
			<-entered
			if err := s.ReleaseRetained(ctx, owner, nil); err == nil {
				t.Errorf("active release: %v", err)
			}
			close(proceed)
			err := <-done
			s.taskWakeWG.Wait()
			if (err != nil) != fail {
				t.Fatalf("result: %v", err)
			}
			if !fail {
				pending, lookupErr := tasks.PendingTasks(ctx, "parent")
				if lookupErr != nil || len(pending) != 0 || runs.Load() != 2 {
					t.Fatalf("queued completion not drained: %v %v runs=%d", pending, lookupErr, runs.Load())
				}
			}
			err = s.ReleaseRetained(ctx, owner, nil)
			if fail && !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("failed persistence released: %v", err)
			}
			if !fail && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreparedForegroundStaleNeverExecutes(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	s.SetCloudLLMProvider(&scriptedProvider{})
	s.toolSvc.SetRegistry(agenttools.NewRegistry())
	var calls atomic.Int32
	s.inProcessRunner = ownedWakeRunner(func(context.Context, runner.PersistFunc) (runner.Result, error) {
		calls.Add(1)
		return runner.Result{}, nil
	})
	owner.Epoch = "stale"
	bound := foregroundBinding(ctx, owner)
	req := &proto.ProcessRequestRequest{ConversationId: "parent", DebugMode: true}
	if err := s.StreamProcessRequest(req, newCollectingStream(bound)); err == nil {
		t.Fatal("stale stream accepted")
	}
	if _, err := s.ProcessRequest(bound, req); err == nil {
		t.Fatal("stale unary accepted")
	}
	if _, err := s.InvokeTool(bound, &proto.InvokeToolRequest{Name: "Write"}); err == nil {
		t.Fatal("unfenced tool accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("stale executor called")
	}
}
