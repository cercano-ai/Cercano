package server

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type drainRPCFixture struct {
	*Server
	entered  chan context.Context
	finish   chan struct{}
	returned chan struct{}
}

func (s *drainRPCFixture) GetConfig(ctx context.Context, _ *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	s.entered <- ctx
	<-ctx.Done() // cancellation must not retire the actual handler lifetime
	<-s.finish
	close(s.returned)
	return nil, ctx.Err()
}
func drainRPCClient(t *testing.T, s proto.AgentServer, gate *Server) (proto.AgentClient, *grpc.ClientConn) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(grpc.UnaryInterceptor(gate.UpdateAdmissionUnaryInterceptor()), grpc.StreamInterceptor(gate.UpdateAdmissionStreamInterceptor()))
	proto.RegisterAgentServer(gs, s)
	go gs.Serve(listener)
	conn, err := grpc.NewClient("passthrough:///buf", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(); gs.Stop(); listener.Close() })
	return proto.NewAgentClient(conn), conn
}
func waitDrainOpen(t *testing.T, s *Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		s.updateWork.mu.Lock()
		paused, changed := s.updateWork.paused, s.updateWork.changed
		s.updateWork.mu.Unlock()
		if !paused {
			return
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatal("seal leaked")
		}
	}
}
func TestUpdateDrainRPCRealLifetime(t *testing.T) {
	s := &drainRPCFixture{Server: &Server{}, entered: make(chan context.Context, 1), finish: make(chan struct{}), returned: make(chan struct{})}
	client, _ := drainRPCClient(t, s, s.Server)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	workCtx, stopWork := context.WithCancel(ctx)
	defer stopWork()
	go client.GetConfig(workCtx, &proto.GetConfigRequest{})
	var admitted context.Context
	select {
	case admitted = <-s.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	owner, err := agentclient.OpenUpdateDrain(ctx, client, strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	for _, phase := range []proto.UpdateDrainState_Phase{proto.UpdateDrainState_SEALED, proto.UpdateDrainState_DRAINING} {
		f, e := owner.Recv()
		if e != nil || f.GetPhase() != phase {
			t.Fatalf("frame %v: %v", f, e)
		}
	}
	if _, e := client.GetConfig(ctx, &proto.GetConfigRequest{}); status.Code(e) != codes.Unavailable {
		t.Fatalf("root: %v", e)
	}
	_, nested, e := s.updateWork.enterContext(admitted)
	if e != nil {
		t.Fatal(e)
	}
	stopWork()
	if _, e := client.CancelRuntimeModelDownload(ctx, &proto.CancelRuntimeModelDownloadRequest{}); e != nil {
		t.Fatalf("cancel unavailable: %v", e)
	}
	if _, e := client.GetAgentHealth(ctx, &proto.GetAgentHealthRequest{}); e != nil {
		t.Fatalf("health unavailable: %v", e)
	}
	idle := make(chan error, 1)
	go func() {
		f, e := owner.Recv()
		if e == nil && f.GetPhase() != proto.UpdateDrainState_IDLE {
			e = status.Error(codes.Internal, "not idle")
		}
		idle <- e
	}()
	close(s.finish)
	<-s.returned
	select {
	case e := <-idle:
		t.Fatalf("nested lifetime ignored: %v", e)
	case <-time.After(30 * time.Millisecond):
	}
	nested()
	select {
	case e := <-idle:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	owner.Close()
	waitDrainOpen(t, s.Server)
}
func TestUpdateDrainRPCValidationCompetitionAndDisconnect(t *testing.T) {
	s := &Server{}
	client, conn := drainRPCClient(t, s, s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, token := range []string{"", "wrong", strings.Repeat("A", 32), strings.Repeat("a", 33)} {
		stream, e := client.DrainForUpdate(ctx, &proto.DrainForUpdateRequest{OperationToken: token})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = stream.Recv(); status.Code(e) != codes.InvalidArgument {
			t.Fatalf("token %q: %v", token, e)
		}
	}
	owner, e := agentclient.OpenUpdateDrain(ctx, client, strings.Repeat("a", 32))
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	for i := 0; i < 3; i++ {
		if _, e = owner.Recv(); e != nil {
			t.Fatal(e)
		}
	}
	for _, token := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32), "wrong"} {
		otherCtx, stop := context.WithCancel(ctx)
		stream, e := client.DrainForUpdate(otherCtx, &proto.DrainForUpdateRequest{OperationToken: token})
		if e != nil {
			t.Fatal(e)
		}
		_, e = stream.Recv()
		stop()
		want := codes.FailedPrecondition
		if token == "wrong" {
			want = codes.InvalidArgument
		}
		if status.Code(e) != want {
			t.Fatalf("competitor: %v", e)
		}
		if release, e := s.updateWork.enter(); e == nil {
			release()
			t.Fatal("competitor released owner")
		}
	}
	conn.Close()
	waitDrainOpen(t, s)
}

func TestUpdateDrainRPCCancelWhileDraining(t *testing.T) {
	s := &Server{}
	client, _ := drainRPCClient(t, s, s)
	work, err := s.updateWork.enter()
	if err != nil {
		t.Fatal(err)
	}
	defer work()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	owner, err := agentclient.OpenUpdateDrain(ctx, client, strings.Repeat("c", 32))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = owner.Recv(); err != nil {
			t.Fatal(err)
		}
	}
	owner.Close()
	waitDrainOpen(t, s)
	next, err := s.updateWork.enter()
	if err != nil {
		t.Fatal(err)
	}
	next()
}
