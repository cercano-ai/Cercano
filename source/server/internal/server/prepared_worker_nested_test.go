package server

import (
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type preparedNestedStream struct {
	proto.UnimplementedWorkerServer
	earlyDone    <-chan struct{}
	terminalSent chan struct{}
}

func (fake *preparedNestedStream) RunTurn(stream proto.Worker_RunTurnServer) error {
	start, err := stream.Recv()
	if err != nil {
		return err
	}
	parent := start.GetStart().GetConversationId()
	for _, pair := range [][2]string{{"worker-child", parent}, {"worker-grandchild", "worker-child"}, {"worker-sibling", parent}} {
		if err := stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_EnsureSubagent{EnsureSubagent: &proto.EnsureSubagentConversation{Id: pair[0], ParentId: pair[1]}}}); err != nil {
			return err
		}
		msg, err := worker.MarshalMessage(llm.Message{Role: "assistant", Blocks: []llm.Block{{Type: llm.BlockText, Text: "fake descendant"}}})
		if err != nil {
			return err
		}
		if err := stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_Persist{Persist: &proto.PersistTurn{ConversationId: pair[0], Message: msg}}}); err != nil {
			return err
		}
		if err := stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_DispatchEvent{DispatchEvent: &proto.DispatchEventRequest{Id: 1, ConversationId: pair[0], Seq: 1, Kind: "model_request", PayloadJson: `{}`}}}); err != nil {
			return err
		}
		if fake.earlyDone != nil {
			<-fake.earlyDone
			err := stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_Done{Done: &proto.TurnDone{}}})
			close(fake.terminalSent)
			return err
		}
		ack, err := stream.Recv()
		if err != nil {
			return err
		}
		if ack.GetDispatchEventResponse().GetError() != "" {
			return fmt.Errorf("evidence refused: %s", ack.GetDispatchEventResponse().GetError())
		}
	}
	return stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_Done{Done: &proto.TurnDone{}}})
}
func TestPreparedWorkerNestedStream(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	lis := bufconn.Listen(1 << 20)
	defer lis.Close()
	srv := grpc.NewServer()
	proto.RegisterWorkerServer(srv, &preparedNestedStream{})
	go srv.Serve(lis)
	defer srv.Stop()
	dial := func(context.Context) (*grpc.ClientConn, error) {
		return grpc.NewClient("passthrough:///nested", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	r := worker.NewWorkerRunnerWithDial(s.persistSvc, s.cfgSvc, s.permBroker, dial)
	r.(worker.TurnAuthoritySetter).SetTurnAuthority(func(ctx context.Context, id string) (worker.TurnAuthority, error) {
		workctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
		if err != nil {
			return nil, err
		}
		return &preparedWorkerAuthority{server: s, ctx: workctx, id: id, ticket: ticket}, nil
	})
	r.(worker.DispatchEventSinkSetter).SetDispatchEventSink(func(ctx context.Context, ev conversation.DispatchEvent) error {
		if _, err := s.resolvePreparedTaskOwner(ctx, ev.ConversationID); err != nil {
			return err
		}
		return worker.HostDispatchEventSink(s.persistSvc.Store().(conversation.DispatchEventStore))(ctx, ev)
	})
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := r.RunTurn(ctx, runner.Request{ConversationID: owner.Conversation, Input: "fake"}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"worker-child", "worker-grandchild", "worker-sibling"} {
		state, err := conversation.ReadOwnership(ctx, s.coexistence.store, id)
		if err != nil || state.State != "released" {
			t.Fatalf("%s ownership=%+v err=%v", id, state, err)
		}
		turns, err := s.coexistence.store.GetTurns(ctx, id)
		if err != nil || len(turns) != 1 {
			t.Fatalf("%s turns=%v err=%v", id, turns, err)
		}
		rows, err := s.coexistence.store.(conversation.DispatchEventStore).ListDispatchEvents(ctx, id)
		if err != nil || len(rows) != 1 {
			t.Fatalf("%s evidence=%v err=%v", id, rows, err)
		}
	}
	if err := s.ReleaseRetained(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedWorkerDescendantAuthorityIsolation(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			s, ctx, owner, _ := preparedWakeFixture(t)
			workctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			a := &preparedWorkerAuthority{server: s, ctx: workctx, id: owner.Conversation, ticket: ticket}
			if err := a.Ensure(context.Background(), "foreign", owner.Conversation, "", "", nil); err == nil {
				t.Fatal("ambient authority accepted")
			}
			for _, pair := range [][2]string{{"wc", owner.Conversation}, {"wg", "wc"}, {"ws", owner.Conversation}} {
				if err := a.Ensure(ctx, pair[0], pair[1], "", "", nil); err != nil {
					t.Fatal(err)
				}
			}
			child, err := a.Context(ctx, "wc")
			if err != nil {
				t.Fatal(err)
			}
			grandchild, err := a.Context(ctx, "wg")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.resolvePreparedTaskOwner(child, "ws"); err == nil {
				t.Fatal("sibling authority accepted")
			}
			if _, _, err := s.beginPreparedDispatch(workctx, "wc", "forged"); err == nil {
				t.Fatal("root used as immediate parent")
			}
			if _, err := a.Context(ctx, "unknown"); err == nil {
				t.Fatal("ID alone authorized")
			}
			if err := a.Ensure(ctx, "wc", "ws", "", "", nil); err == nil {
				t.Fatal("duplicate reparented")
			}
			if failed {
				persistsvc.ReportWriteFailure(grandchild, errors.New("fake descendant write failure"))
			}
			a.Close(nil)
			for _, id := range []string{"wc", "wg", "ws"} {
				state, err := conversation.ReadOwnership(ctx, s.coexistence.store, id)
				if err != nil {
					t.Fatal(err)
				}
				wantReleased := !failed || id == "ws"
				if (state.State == "released") != wantReleased {
					t.Fatalf("%s state=%+v failed=%v", id, state, failed)
				}
			}
			if _, err := a.Context(ctx, "wc"); err == nil {
				t.Fatal("closed authority accessible")
			}
			if err := s.ReleaseRetained(ctx, owner, nil); (err == nil) == failed {
				t.Fatalf("root release failed=%v: %v", failed, err)
			}
		})
	}
}

func TestPreparedWorkerCloseWaitsForHostCallbacks(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	started, unblock, terminal := make(chan struct{}), make(chan struct{}), make(chan struct{})
	fake := &preparedNestedStream{earlyDone: started, terminalSent: terminal}
	lis := bufconn.Listen(1 << 20)
	defer lis.Close()
	srv := grpc.NewServer()
	proto.RegisterWorkerServer(srv, fake)
	go srv.Serve(lis)
	defer srv.Stop()
	dial := func(context.Context) (*grpc.ClientConn, error) {
		return grpc.NewClient("passthrough:///callback", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	r := worker.NewWorkerRunnerWithDial(s.persistSvc, s.cfgSvc, s.permBroker, dial)
	r.(worker.TurnAuthoritySetter).SetTurnAuthority(func(ctx context.Context, id string) (worker.TurnAuthority, error) {
		workctx, ticket, err := s.BeginRetainedWork(ctx, owner, broker.PersistenceWork)
		if err != nil {
			return nil, err
		}
		return &preparedWorkerAuthority{server: s, ctx: workctx, id: id, ticket: ticket}, nil
	})
	r.(worker.DispatchEventSinkSetter).SetDispatchEventSink(func(ctx context.Context, ev conversation.DispatchEvent) error {
		if _, err := s.resolvePreparedTaskOwner(ctx, ev.ConversationID); err != nil {
			return err
		}
		close(started)
		<-unblock // Deliberately ignore cancellation: cancellation is not drainage.
		return nil
	})
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	returned := make(chan error, 1)
	go func() {
		_, err := r.RunTurn(ctx, runner.Request{ConversationID: owner.Conversation, Input: "fake"}, nil, nil, nil)
		returned <- err
	}()
	select {
	case <-terminal:
	case <-ctx.Done():
		t.Fatal("fake did not publish terminal")
	}
	select {
	case err := <-returned:
		t.Fatalf("returned with callback running: %v", err)
	default:
	}
	state, err := conversation.ReadOwnership(ctx, s.coexistence.store, "worker-child")
	if err != nil || state.State == "released" {
		t.Fatalf("child closed while callback running: %+v %v", state, err)
	}
	if err := s.ReleaseRetained(ctx, owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("root escaped callback: %v", err)
	}
	close(unblock)
	select {
	case <-returned:
	case <-ctx.Done():
		t.Fatal("host failed to join callback")
	}
}
