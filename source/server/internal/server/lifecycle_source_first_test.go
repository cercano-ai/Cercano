package server

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func sourceFirstPeer(t *testing.T, s *Server, interceptor grpc.UnaryServerInterceptor) (*agentclient.LifecycleControls, string) {
	t.Helper()
	dial := func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		return grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	s.coexistence.options.ControlAuthorize = func(context.Context) error { return nil } // synthetic trust only
	s.coexistence.options.ControlDial = dial
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var opts []grpc.ServerOption
	if interceptor != nil {
		opts = append(opts, grpc.UnaryInterceptor(interceptor))
	}
	g := grpc.NewServer(opts...)
	pb.RegisterAgentServer(g, s)
	go g.Serve(listener)
	t.Cleanup(g.Stop)
	c, err := agentclient.DialLifecycleControls(context.Background(), listener.Addr().String(), dial)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, listener.Addr().String()
}
func sourceFirstRequest(t *testing.T, a, b *Server, endpoint, id string) *pb.LifecycleControlRequest {
	t.Helper()
	o, err := a.Retain(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Owner: wireOwner(o), HandoverId: id, Management: true}
}
func TestSourceFirstBusyNoDestinationParticipation(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ac, _ := sourceFirstPeer(t, a, nil)
	bc, ep := sourceFirstPeer(t, b, nil)
	r := sourceFirstRequest(t, a, b, ep, "busy-first")
	ticket, err := a.AdmitRetainedWork(controlOwner(r), broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ac.Transfer(context.Background(), bc, r); err == nil {
		t.Fatal("busy transfer succeeded")
	}
	if len(b.coexistence.incoming) != 0 {
		t.Fatal("destination participated in busy refusal")
	}
	ticket.Complete()
	attachment, err := a.AttachRetained(controlOwner(r))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ac.Transfer(context.Background(), bc, r); err == nil {
		t.Fatal("extra attachment accepted")
	}
	if len(b.coexistence.incoming) != 0 {
		t.Fatal("destination participated in attachment refusal")
	}
	attachment.Detach()
	result, err := ac.Transfer(context.Background(), bc, r)
	if err != nil || result.GetOwner().GetHost() != r.Destination.Host {
		t.Fatal(result, err)
	}
}
func TestSourceFirstClosedAndAmbiguousPrepareRetry(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ac, _ := sourceFirstPeer(t, a, nil)
	entered, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	bc, ep := sourceFirstPeer(t, b, func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		result, err := next(ctx, req)
		if info.FullMethod == pb.Agent_LifecyclePrepare_FullMethodName {
			lost := false
			once.Do(func() { close(entered); <-proceed; lost = true })
			if lost {
				return nil, errors.New("lost prepare response")
			}
		}
		return result, err
	})
	r := sourceFirstRequest(t, a, b, ep, "ambiguous-first")
	done := make(chan error, 1)
	go func() { _, err := ac.Transfer(context.Background(), bc, r); done <- err }()
	<-entered
	if _, err := a.AdmitRetainedWork(controlOwner(r), broker.ForegroundWork); err == nil {
		t.Fatal("work admitted after closure")
	}
	if err := b.RetireCoexistence(context.Background()); err == nil {
		t.Fatal("destination retired accepted transfer")
	}
	close(proceed)
	if err := <-done; err == nil {
		t.Fatal("lost response reported success")
	}
	if err := a.RetireCoexistence(context.Background()); err == nil {
		t.Fatal("source uncertainty lost blocker")
	}
	if _, err := ac.Transfer(context.Background(), bc, r); err != nil {
		t.Fatal("explicit retry", err)
	}
}
func TestSourceFirstReciprocalNoMutexAcrossRPC(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	entered := make(chan struct{}, 2)
	proceed := make(chan struct{})
	barrier := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod == pb.Agent_LifecyclePrepare_FullMethodName {
			entered <- struct{}{}
			select {
			case <-proceed:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return next(ctx, req)
	}
	ac, ae := sourceFirstPeer(t, a, barrier)
	bc, be := sourceFirstPeer(t, b, barrier)
	ab := sourceFirstRequest(t, a, b, be, "ab")
	ba := sourceFirstRequest(t, b, a, ae, "ba")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 2)
	go func() { _, err := ac.Transfer(ctx, bc, ab); done <- err }()
	go func() { _, err := bc.Transfer(ctx, ac, ba); done <- err }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("peer prepare deadlock")
		}
	}
	close(proceed)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
func TestSourceFirstRetiredDestinationKeepsSourceClosed(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ac, _ := sourceFirstPeer(t, a, nil)
	bc, ep := sourceFirstPeer(t, b, nil)
	r := sourceFirstRequest(t, a, b, ep, "retired-first")
	if err := b.RetireCoexistence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := ac.Transfer(context.Background(), bc, r); err == nil {
		t.Fatal("retired destination accepted")
	}
	if _, err := a.AdmitRetainedWork(controlOwner(r), broker.ForegroundWork); err == nil {
		t.Fatal("uncertain source reopened")
	}
	if len(b.coexistence.incoming) != 0 {
		t.Fatal("retired destination participated")
	}
}
func TestSourceFirstCopiedReservationRejectedBeforeCommit(t *testing.T) {
	a, _, _ := lifecycleFixture(t)
	o, err := a.Retain(context.Background(), "copied-first")
	if err != nil {
		t.Fatal(err)
	}
	l := a.coexistence
	l.mu.Lock()
	defer l.mu.Unlock()
	r, err := l.reserveRelinquishLocked(o, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied := *r
	if err = l.commitRelinquishLocked(context.Background(), o, &copied, nil); !errors.Is(err, broker.ErrAdmissionStale) {
		t.Fatal(err)
	}
	if err = l.commitRelinquishLocked(context.Background(), o, r, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSourceFirstCommitErrorPreservesReceiptForExplicitClaim(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	a.coexistence.options.Cleanup = func(context.Context, broker.Owner) error { return errors.New("uncertain cleanup") }
	ac, _ := sourceFirstPeer(t, a, nil)
	bc, ep := sourceFirstPeer(t, b, nil)
	r := sourceFirstRequest(t, a, b, ep, "commit-error-first")
	result, err := ac.Transfer(context.Background(), bc, r)
	if err == nil || result.GetReceipt() == "" || !result.Pending {
		t.Fatal("missing pending error receipt", result, err)
	}
	if err := a.RetireCoexistence(context.Background()); err == nil {
		t.Fatal("cleanup uncertainty lost blocker")
	}
	r.Target = r.Destination
	r.Receipt = result.Receipt
	if _, err := bc.LifecycleClaim(context.Background(), r); err != nil {
		t.Fatal("explicit claim", err)
	}
}

func TestSourceFirstLostSourceResponseExplicitRetry(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	ac, _ := sourceFirstPeer(t, a, nil)
	bc, ep := sourceFirstPeer(t, b, nil)
	r := sourceFirstRequest(t, a, b, ep, "lost-source-first")
	if _, err := ac.LifecycleRelinquish(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	result, err := ac.Transfer(context.Background(), bc, r)
	if err != nil || result.GetOwner().GetHost() != r.Destination.Host {
		t.Fatal(result, err)
	}
}
