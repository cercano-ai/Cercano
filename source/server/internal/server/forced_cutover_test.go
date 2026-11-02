package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/taskrun"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func serveCutoverFixture(t *testing.T, s *Server) (pb.AgentClient, string) {
	t.Helper()
	dial := func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		return grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	s.coexistence.options.ControlAuthorize = func(context.Context) error { return nil } // disposable fixture only
	s.coexistence.options.ControlDial = dial
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	pb.RegisterAgentServer(g, s)
	go g.Serve(listener)
	t.Cleanup(g.Stop)
	c, err := dial(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return pb.NewAgentClient(c), listener.Addr().String()
}
func TestForcedCutoverTransfersStoppedAndExecutingSessions(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "stopped-with-stale-client", true: "cancel-independent-task"}[active], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a, b, _ := lifecycleFixture(t)
			ac, _ := serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			owner, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			sibling, err := a.Retain(ctx, "other-chat")
			if err != nil {
				t.Fatal(err)
			}
			old, err := a.coexistence.admission.AttachTerminal(owner, "stale-terminal")
			if err != nil {
				t.Fatal(err)
			}
			var task conversation.TaskRecord
			if active {
				started := make(chan struct{})
				manager, err := taskrun.NewPreparedOwned(a.coexistence.store.(conversation.TaskStore), func(ctx context.Context, s dispatch.Spec) (dispatch.Result, error) {
					close(started)
					<-ctx.Done()
					return dispatch.Result{Text: "partial result"}, ctx.Err()
				}, nil, a.coexistence.admission, func(context.Context, string) (*conversation.Ownership, error) {
					return a.coexistence.owners[owner.Conversation], nil
				})
				if err != nil {
					t.Fatal(err)
				}
				a.taskManager = manager
				t.Cleanup(func() { manager.Close(context.Background()) })
				task, err = manager.Launch(ctx, dispatch.Spec{ConversationID: "chat", Task: "wait for cancellation"})
				if err != nil {
					t.Fatal(err)
				}
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			request := &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Owner: wireOwner(owner), Management: true, HandoverId: "forced-op"}
			wrong := *request
			wrong.Owner = wireOwner(broker.Owner{Conversation: owner.Conversation, Host: owner.Host, Epoch: "stale"})
			if _, err := ac.LifecycleForceRelinquish(ctx, &wrong); err == nil {
				t.Fatal("stale epoch cancelled source")
			}
			select {
			case <-old.Revoked():
				t.Fatal("bad request revoked client")
			default:
			}
			result, err := ac.LifecycleForceRelinquish(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-old.Revoked():
			default:
				t.Fatal("stale terminal not revoked")
			}
			if active {
				record, err := a.taskManager.Inspect(ctx, "chat", task.ID)
				if err != nil || record.Status != "stopped" {
					t.Fatalf("task not stopped: %+v %v", record, err)
				}
			}
			// Lost source response is retryable with the same exact operation.
			retry, err := ac.LifecycleForceRelinquish(ctx, request)
			if err != nil || retry.GetReceipt() != result.GetReceipt() {
				t.Fatalf("retry: %v %v", retry, err)
			}
			claim := *request
			claim.Target = claim.Destination
			claim.Receipt = result.Receipt
			moved, err := bc.LifecycleClaim(ctx, &claim)
			if err != nil {
				t.Fatal(err)
			}
			if moved.Owner.Host != "B" || moved.Owner.Epoch == owner.Epoch {
				t.Fatalf("not on new server: %+v", moved)
			}
			if _, err := a.coexistence.admission.AttachTerminal(owner, "reconnect"); err == nil {
				t.Fatal("old host reattached")
			}
			w, err := b.AdmitRetainedWork(controlOwner(&pb.LifecycleControlRequest{Owner: moved.Owner}), broker.ForegroundWork)
			if err != nil {
				t.Fatal(err)
			}
			w.Complete()
			w, err = a.AdmitRetainedWork(sibling, broker.ForegroundWork)
			if err != nil {
				t.Fatal("unrelated chat disrupted", err)
			}
			w.Complete()
		})
	}
}
func TestForcedCutoverWorkerCancellationAccounting(t *testing.T) {
	for _, mode := range []string{"cancelled", "write-failure", "callback-failure"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, owner, _ := preparedWakeFixture(t)
			workctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			authority := &preparedWorkerAuthority{server: s, ctx: workctx, id: owner.Conversation, ticket: ticket}
			var outcome error = context.Canceled
			if mode == "write-failure" {
				persistsvc.ReportWriteFailure(workctx, errors.New("uncertain write"))
			}
			if mode == "callback-failure" {
				outcome = errors.Join(outcome, errors.New("callback failed"))
			}
			authority.Close(outcome) // host invokes this only after cleanup and callback join
			err = s.ReleaseRetained(ctx, owner, nil)
			if mode == "cancelled" && err != nil {
				t.Fatalf("stopped worker counted as unresolved persistence: %v", err)
			}
			if mode != "cancelled" && err == nil {
				t.Fatal("uncertain outcome released")
			}
		})
	}
}

func TestForcedCutoverCancelsAdmittedForegroundAndWaitsForWriteback(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ac, _ := serveCutoverFixture(t, a)
	bc, endpoint := serveCutoverFixture(t, b)
	owner, err := a.Retain(context.Background(), "late-start")
	if err != nil {
		t.Fatal(err)
	}
	bound := metadata.NewIncomingContext(context.Background(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "old"))
	workctx, work, err := a.beginForeground(bound, owner.Conversation)
	if err != nil {
		t.Fatal(err)
	}
	// Work has been admitted but no broker turn has registered yet.
	request := &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Owner: wireOwner(owner), Management: true, HandoverId: "late-start-cutover"}
	deadline, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = ac.LifecycleForceRelinquish(deadline, request)
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("released while writeback held: %v", err)
	}
	select {
	case <-workctx.Done():
	case <-time.After(time.Second):
		t.Fatal("admitted foreground not cancelled")
	}
	if _, err := b.Retain(context.Background(), owner.Conversation); err == nil {
		t.Fatal("destination acquired before writeback")
	}
	work.finish()
	ctx, cancelRetry := context.WithTimeout(context.Background(), time.Second)
	defer cancelRetry()
	response, err := ac.LifecycleForceRelinquish(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	claim := *request
	claim.Target = claim.Destination
	claim.Receipt = response.Receipt
	if _, err := bc.LifecycleClaim(ctx, &claim); err != nil {
		t.Fatal(err)
	}
}
