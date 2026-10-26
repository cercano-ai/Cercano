package server

import (
	"cercano/source/server/internal/broker"
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"net"
	"testing"
)

func TestLifecycleControlsLocalhostTransfer(t *testing.T) {
	ctx := context.Background()
	a, b, c := lifecycleFixture(t)
	dial := func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		return grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	serve := func(s *Server) (pb.AgentClient, string) {
		s.coexistence.options.ControlAuthorize = func(context.Context) error { return nil } // synthetic fixture trust ONLY
		s.coexistence.options.ControlDial = dial
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		g := grpc.NewServer()
		pb.RegisterAgentServer(g, s)
		go g.Serve(listener)
		t.Cleanup(g.Stop)
		conn, e := dial(ctx, listener.Addr().String())
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { conn.Close() })
		return pb.NewAgentClient(conn), listener.Addr().String()
	}
	ac, _ := serve(a)
	bc, endpoint := serve(b)
	cc, _ := serve(c)
	owned, e := ac.LifecycleAcquire(ctx, &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "transport"}})
	if e != nil {
		t.Fatal(e)
	}
	r := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Owner: owned.Owner, HandoverId: "transfer", Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Management: true}
	accepted, e := bc.LifecyclePrepare(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	r.Receipt = accepted.Receipt
	if len(r.Receipt) != 64 {
		t.Fatal("opaque receipt missing")
	}
	if _, e = bc.LifecycleRetire(ctx, &pb.LifecycleControlRequest{Target: r.Destination}); e == nil {
		t.Fatal("retired pending incoming")
	}
	if _, e = bc.LifecycleClaim(ctx, r); e == nil {
		t.Fatal("claimed before source commit")
	}
	source := proto.Clone(r).(*pb.LifecycleControlRequest)
	source.Target = source.Source
	ticket, e := a.AdmitRetainedWork(controlOwner(r), broker.ForegroundWork)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ac.LifecycleRelinquish(ctx, source); e == nil {
		t.Fatal("busy transfer succeeded")
	}
	attachment, e := a.AttachRetained(controlOwner(r))
	if e != nil {
		t.Fatal("busy transfer damaged source", e)
	}
	attachment.Detach()
	ticket.Complete()
	// Ignore the successful source response, modeling loss after durable reservation.
	if _, e = ac.LifecycleRelinquish(ctx, source); e != nil {
		t.Fatal(e)
	}
	wrong := proto.Clone(r).(*pb.LifecycleControlRequest)
	wrong.Target = c.coexistence.controlIdentity()
	if _, e = cc.LifecycleClaim(ctx, wrong); e == nil {
		t.Fatal("C claimed B receipt")
	}
	if _, e = cc.LifecycleAcquire(ctx, &pb.LifecycleControlRequest{Target: wrong.Target, Owner: r.Owner}); e == nil {
		t.Fatal("C acquired pending reservation")
	}
	claimed, e := bc.LifecycleClaim(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := bc.LifecycleClaim(ctx, r)
	if e != nil || retry.GetOwner().GetEpoch() != claimed.Owner.Epoch {
		t.Fatal("claim retry", retry, e)
	}
	if _, e = ac.LifecycleRelease(ctx, source); e == nil {
		t.Fatal("stale source released")
	}
}
func TestLifecycleControlsDisabledWithoutTrust(t *testing.T) {
	a, _, _ := lifecycleFixture(t)
	if _, e := a.LifecycleList(context.Background(), &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity()}); e == nil {
		t.Fatal("unauthenticated controls enabled")
	}
}
