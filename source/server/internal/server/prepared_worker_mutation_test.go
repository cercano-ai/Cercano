package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/chatroute"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/worker"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// The peer deliberately never receives the callback response. Its exit is gated
// on the host completing the real SQL operation and observing its evidence.
type mutationLostResponsePeer struct {
	pb.UnimplementedWorkerServer
	request  *pb.WorkerToHost
	recorded <-chan struct{}
	exited   chan struct{}
	cancel   bool
}

func (p *mutationLostResponsePeer) RunTurn(stream pb.Worker_RunTurnServer) error {
	defer close(p.exited)
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if ev := p.request.GetDispatchEvent(); ev != nil {
		if err := stream.Send(&pb.WorkerToHost{Msg: &pb.WorkerToHost_EnsureSubagent{EnsureSubagent: &pb.EnsureSubagentConversation{Id: ev.ConversationId, ParentId: "parent"}}}); err != nil {
			return err
		}
	}
	if err := stream.Send(p.request); err != nil {
		return err
	}
	select {
	case <-p.recorded:
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	if p.cancel {
		return status.Error(codes.Canceled, "response discarded")
	}
	return nil // EOF without a response acknowledgment or TurnDone
}

// These use production session/task callbacks and the production ledger/evidence
// store adapters. The latter two use the explicit failure-evidence scope used by
// configureLifecycleWorker; this does NOT exercise its process-spawning wiring.
func TestPreparedWorkerRealMutationLostResponse(t *testing.T) {
	for _, kind := range []string{"session", "task", "autonomy", "dispatch"} {
		for _, mode := range []string{"write-failure", "success", "validation", "read-rejection"} {
			// Task has no read-only launch operation;
			// ledger/evidence validation has a distinct (broader) production error policy.
			if kind == "task" && mode == "read-rejection" {
				continue
			}
			if (kind == "autonomy" || kind == "dispatch") && mode != "write-failure" && mode != "success" {
				continue
			}
			for _, canceled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/cancel=%t", kind, mode, canceled), func(t *testing.T) {
					s, base, owner, _ := preparedWakeFixture(t)
					defer func() {
						if cap := s.coexistence.owners["evidence-child"]; cap != nil {
							_ = cap.Close()
						}
					}()
					ctx, cancel := context.WithTimeout(base, 5*time.Second)
					defer cancel()
					db, err := sql.Open("sqlite", s.coexistence.options.StorePath)
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					var request *pb.WorkerToHost
					table, operation := "", "INSERT"
					switch kind {
					case "session":
						if err := s.persistSvc.Store().(chatroute.Store).SetChatRoute(ctx, owner.Conversation, &chatroute.Route{Profile: "saved", Model: "fixture"}); err != nil {
							t.Fatal(err)
						}
						action := "clear"
						if mode == "validation" {
							action = "invalid"
						}
						if mode == "read-rejection" {
							action = "status"
						}
						request = &pb.WorkerToHost{Msg: &pb.WorkerToHost_SessionModelRequest{SessionModelRequest: &pb.SessionModelRequest{Id: "1", ConversationId: owner.Conversation, Action: action}}}
						table, operation = "conversation_chat_routes", "DELETE"
					case "task":
						task := "fixture work"
						if mode == "validation" {
							task = ""
						}
						request = &pb.WorkerToHost{Msg: &pb.WorkerToHost_TaskControlRequest{TaskControlRequest: &pb.TaskControlRequest{Id: "1", Operation: "launch", Task: task}}}
						table = "delegated_tasks"
					case "autonomy":
						raw, _ := json.Marshal(conversation.AutonomyRun{ConversationID: owner.Conversation, State: "running"})
						request = &pb.WorkerToHost{Msg: &pb.WorkerToHost_AutonomyRequest{AutonomyRequest: &pb.AutonomyLedgerRequest{Id: 1, Op: "create", RunJson: raw}}}
						table = "autonomy_runs"
					case "dispatch":
						request = &pb.WorkerToHost{Msg: &pb.WorkerToHost_DispatchEvent{DispatchEvent: &pb.DispatchEventRequest{Id: 1, ConversationId: "evidence-child", Seq: 1, Kind: "model_request", PayloadJson: `{}`}}}
						table = "dispatch_events"
					}
					if mode == "write-failure" {
						if _, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER injected_callback_failure BEFORE %s ON %s BEGIN SELECT RAISE(ABORT, 'injected real mutation failure'); END`, operation, table)); err != nil {
							t.Fatal(err)
						}
					}
					if mode == "read-rejection" {
						if _, err := db.Exec(`DROP TABLE conversation_chat_routes`); err != nil {
							t.Fatal(err)
						}
					}
					recorded, exited := make(chan struct{}), make(chan struct{})
					peer := &mutationLostResponsePeer{request: request, recorded: recorded, exited: exited, cancel: canceled}
					lis := bufconn.Listen(1 << 20)
					defer lis.Close()
					gs := grpc.NewServer()
					pb.RegisterWorkerServer(gs, peer)
					served := make(chan struct{})
					go func() { defer close(served); _ = gs.Serve(lis) }()
					defer func() {
						gs.Stop()
						select {
						case <-served:
						case <-time.After(5 * time.Second):
							t.Error("gRPC server did not join")
						}
					}()
					dial := func(context.Context) (*grpc.ClientConn, error) {
						return grpc.NewClient("passthrough:///mutation", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()))
					}
					r := worker.NewWorkerRunnerWithDial(s.persistSvc, s.cfgSvc, s.permBroker, dial)
					var authority *preparedWorkerAuthority
					r.(worker.TurnAuthoritySetter).SetTurnAuthority(func(c context.Context, id string) (worker.TurnAuthority, error) {
						work, ticket, err := s.BeginRetainedWork(c, owner, broker.PersistenceWork)
						if err != nil {
							return nil, err
						}
						authority = &preparedWorkerAuthority{server: s, ctx: work, id: id, ticket: ticket}
						return authority, nil
					})
					// Observe evidence BEFORE letting the peer drop the response. No test-created
					// MutationError or synthetic ReportWriteFailure is used for session/task.
					observe := func(c context.Context, err error) {
						if (err != nil) != (mode != "success") {
							t.Errorf("callback error=%v, mode=%s", err, mode)
						}
						if mode == "write-failure" && (err == nil || !strings.Contains(err.Error(), "injected real mutation failure")) {
							t.Errorf("did not reach injected SQL failure: %v", err)
						}
						if (persistsvc.WriteFailure(c) != nil) != (mode == "write-failure") {
							t.Errorf("evidence before response loss=%v", persistsvc.WriteFailure(c))
						}
						if kind == "task" && mode == "success" {
							var count int
							if e := db.QueryRowContext(c, `SELECT count(*) FROM delegated_tasks WHERE id != 'one'`).Scan(&count); e != nil || count != 1 {
								t.Errorf("successful task INSERT before response loss: rows=%d err=%v", count, e)
							}
						}
						close(recorded)
						select {
						case <-exited:
						case <-ctx.Done():
							t.Error("peer did not exit")
						}
					}
					r.(worker.SessionModelSetter).SetSessionModel(func(c context.Context, id string, q chatroute.Request) (chatroute.Status, error) {
						out, err := s.sessionModel(c, id, q)
						observe(c, err)
						return out, err
					})
					r.(worker.TasksSetter).SetTasks(mutationObservedTasks{serverTasks: serverTasks{s}, observe: observe})
					ledger := worker.HostAutonomyLedger(s.persistSvc.Store())
					r.(worker.AutonomyLedgerSetter).SetAutonomyLedger(func(c context.Context, op, raw, id string) (string, bool, error) {
						out, found, err := ledger(c, op, raw, id)
						if err != nil {
							persistsvc.ReportWriteFailure(c, err)
						}
						observe(c, err)
						return out, found, err
					})
					sink := worker.HostDispatchEventSink(s.persistSvc.Store().(conversation.DispatchEventStore))
					r.(worker.DispatchEventSinkSetter).SetDispatchEventSink(func(c context.Context, ev conversation.DispatchEvent) error {
						err := sink(c, ev)
						if err != nil {
							persistsvc.ReportWriteFailure(c, err)
						}
						observe(c, err)
						return err
					})
					_, _ = r.RunTurn(ctx, runner.Request{ConversationID: owner.Conversation, Input: "fixture"}, nil, nil, nil)
					select {
					case <-recorded:
					default:
						t.Fatal("callback never reached mutation boundary")
					}
					if authority == nil {
						t.Fatal("missing production authority")
					}
					authority.Close(nil) // cannot erase a failure on repeated close
					snap := s.coexistence.admission.Snapshot(owner)
					if mode != "write-failure" {
						if snap.UnresolvedWork != 0 {
							t.Fatalf("false uncertain ticket: %+v", snap)
						}
						return
					}
					wantUnresolved := 1
					if kind == "dispatch" {
						wantUnresolved = 2
					} // root plus ensured descendant
					if snap.UnresolvedWork != wantUnresolved || snap.Work[broker.PersistenceWork] != wantUnresolved {
						t.Fatalf("missing unresolved persistence ticket: %+v", snap)
					}
					attachment, err := s.coexistence.admission.AttachTerminal(owner, "mutation-terminal")
					if err != nil {
						t.Fatal(err)
					}
					defer attachment.Detach()
					closure, err := s.coexistence.admission.DrainTerminal(owner, "mutation-terminal")
					if err != nil {
						t.Fatal(err)
					}
					if closure.Drained() {
						t.Fatal("uncertain mutation drained")
					}
					if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) && !errors.Is(err, broker.ErrAdmissionClosed) {
						t.Fatalf("release=%v", err)
					}
					if after := s.coexistence.admission.Snapshot(owner); after.UnresolvedWork != wantUnresolved {
						t.Fatalf("terminal drain discarded tickets: %+v", after)
					}
					state, err := conversation.ReadOwnership(ctx, s.coexistence.store, owner.Conversation)
					if err != nil || state.Owner != owner.Host || state.Epoch != owner.Epoch || state.State != "owned" {
						t.Fatalf("owner changed: %+v %v", state, err)
					}
					successor, err := conversation.AcquireOwnership(ctx, s.coexistence.store, owner.Conversation, "successor")
					if successor != nil {
						_ = successor.Close()
					}
					if err == nil {
						t.Fatal("successor acquired uncertain owner")
					}
				})
			}
		}
	}
}

type mutationObservedTasks struct {
	serverTasks
	observe func(context.Context, error)
}

func (x mutationObservedTasks) Launch(ctx context.Context, spec dispatch.Spec) (conversation.TaskRecord, error) {
	out, err := x.serverTasks.Launch(ctx, spec)
	x.observe(ctx, err)
	return out, err
}
