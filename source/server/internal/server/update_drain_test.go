package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"google.golang.org/grpc"
)

func TestDrainRejectsRootsBeforeIdle(t *testing.T) {
	var g updateAdmission
	done, _ := g.enter()
	defer done()
	cancel, result := startTestDrain(t, &g)
	defer cancel()
	if release, err := g.enter(); err == nil {
		release()
		t.Error("new root admitted while drain waits: roots can starve drain")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestDrainInterceptorNestedCrossesSeal(t *testing.T) {
	s := &Server{}
	_, err := s.UpdateAdmissionUnaryInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: agentFullMethodPrefix() + "SetConfig"}, func(ctx context.Context, _ any) (any, error) {
		cancel, result := startTestDrain(t, &s.updateWork)
		defer func() { cancel(); <-result }()
		release, err := s.admitUpdateWork(ctx)
		if err == nil {
			release()
		}
		return nil, err
	})
	if err != nil {
		t.Fatalf("admitted handler continuation rejected after seal: %v", err)
	}
}

// Wait on the gate notification, not a sleep, to establish the sealing boundary.
func startTestDrain(t *testing.T, g *updateAdmission) (context.CancelFunc, <-chan error) {
	t.Helper()
	g.mu.Lock()
	if g.changed == nil {
		g.changed = make(chan struct{})
	}
	changed := g.changed
	g.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		release, err := g.drain(ctx)
		if release != nil {
			release()
		}
		result <- err
	}()
	select {
	case <-changed:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("drain did not seal")
	}
	return cancel, result
}

func TestDrainOwnershipAndCancel(t *testing.T) {
	var g, foreign updateAdmission
	ctx, root, err := g.enterContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancel, result := startTestDrain(t, &g)
	defer cancel()
	if _, release, err := foreign.enterContext(ctx); err == nil {
		release()
		t.Fatal("foreign capability accepted")
	}
	childCtx, child, err := g.enterContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root()
	if _, release, err := g.enterContext(ctx); err == nil {
		release()
		t.Fatal("retired parent capability accepted")
	}
	grandCtx, grand, err := g.enterContext(childCtx)
	if err != nil {
		t.Fatal(err)
	}
	_ = grandCtx
	child()
	grand()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	// Even with no seal a stale capability is not silently upgraded to a root.
	if _, release, err := g.enterContext(ctx); err == nil {
		release()
		t.Fatal("stale capability upgraded")
	}
	work, err := g.enter()
	if err != nil {
		t.Fatal(err)
	}
	defer work()
	cancel2, result2 := startTestDrain(t, &g)
	cancel2()
	if err := <-result2; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	next, err := g.enter()
	if err != nil {
		t.Fatal(err)
	}
	next()
}

func TestDrainOwnerCancellationAfterIdleAndCompetingOwner(t *testing.T) {
	var g updateAdmission
	ctx, cancel := context.WithCancel(context.Background())
	release, err := g.drain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if other, err := g.drain(context.Background()); err == nil {
		other()
		t.Fatal("competing owner accepted")
	}
	g.mu.Lock()
	changed := g.changed
	g.mu.Unlock()
	cancel()
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("owner loss did not reopen")
	}
	work, err := g.enter()
	if err != nil {
		t.Fatal(err)
	}
	work()
	// Releasing an old seal cannot release a newer owner's seal.
	newer, err := g.drain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer newer()
	release()
	if work, err := g.enter(); err == nil {
		work()
		t.Fatal("stale release opened newer seal")
	}
}

func TestDrainAsyncCompactionContinuationTracksActualReturn(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	g := compactiongen.New(nil, nil, compactor.Config{}, nil, time.Hour)
	s.SetCompactionGenerator(g)
	g.SetEnabled(true)
	g.SetToolElisionOnly(true)
	entered, finish := make(chan context.Context, 1), make(chan struct{})
	g.SetElideOnlyFn(func(ctx context.Context, _ string) (int, int, int, bool, error) {
		entered <- ctx
		<-finish
		return 0, 0, 0, false, nil
	})
	rootCtx, root, err := s.updateWork.enterContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cancel, result := startTestDrain(t, &s.updateWork)
	defer cancel()
	if err := g.CompactAsyncContext(rootCtx, "drain-continuation"); err != nil {
		t.Fatal(err)
	}
	root()
	var asyncCtx context.Context
	select {
	case asyncCtx = <-entered:
	case <-time.After(time.Second):
		t.Fatal("async work did not start")
	}
	// The job owns a fresh lease after its initiating request has returned.
	_, nested, err := s.updateWork.enterContext(asyncCtx)
	if err != nil {
		t.Fatal(err)
	}
	nested()
	select {
	case err := <-result:
		t.Fatalf("drain retired running job: %v", err)
	default:
	}
	close(finish)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("job lease leaked")
	}
	if _, release, err := s.updateWork.enterContext(asyncCtx); err == nil {
		release()
		t.Fatal("completed async owner remains live")
	}
	if err := g.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDrainStreamContinuationAndControls(t *testing.T) {
	s := &Server{}
	err := s.UpdateAdmissionStreamInterceptor()(nil, &updateAdmissionFakeStream{ctx: context.Background()}, &grpc.StreamServerInfo{FullMethod: agentFullMethodPrefix() + "StreamProcessRequest"}, func(_ any, stream grpc.ServerStream) error {
		cancel, result := startTestDrain(t, &s.updateWork)
		defer func() { cancel(); <-result }()
		release, err := s.admitUpdateWork(stream.Context())
		if err != nil {
			return err
		}
		release()
		_, err = s.UpdateAdmissionUnaryInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: agentFullMethodPrefix() + "CancelRuntimeModelDownload"}, func(context.Context, any) (any, error) { return nil, nil })
		if err != nil {
			return err
		}
		return s.UpdateAdmissionStreamInterceptor()(nil, &updateAdmissionFakeStream{ctx: context.Background()}, &grpc.StreamServerInfo{FullMethod: agentFullMethodPrefix() + "SubscribeEvents"}, func(any, grpc.ServerStream) error { return nil })
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDrainForceStillRejectsContinuations(t *testing.T) {
	var g updateAdmission
	ctx, root, _ := g.enterContext(context.Background())
	defer root()
	if err := g.sealForForce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, release, err := g.enterContext(ctx); err == nil {
		release()
		t.Fatal("force seal admitted continuation")
	}
}

func TestDrainCoverageErrorReleasesSeal(t *testing.T) {
	s := &Server{}
	root, _ := s.updateWork.enter()
	s.updateWork.mu.Lock()
	s.updateWork.changed = make(chan struct{})
	changed := s.updateWork.changed
	s.updateWork.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		release, err := s.drainForUpdate(ctx)
		if release != nil {
			release()
		}
		result <- err
	}()
	select {
	case <-changed:
	case <-ctx.Done():
		root()
		t.Fatal("drain did not seal")
	}
	s.setInjectedUpdateTrackingErr("runtime", errors.New("tracking lost"))
	root()
	if err := <-result; !errors.Is(err, errUpdateCoverageIncomplete) {
		t.Fatalf("coverage refusal: %v", err)
	}
	next, err := s.updateWork.enter()
	if err != nil {
		t.Fatalf("coverage error leaked seal: %v", err)
	}
	next()
}
