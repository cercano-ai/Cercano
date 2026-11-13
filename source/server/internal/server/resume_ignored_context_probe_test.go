package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"net"
	"os"
	"testing"
	"time"
)

// Ignored-context observers are released and joined by this disposable fixture.
func TestBoundedPreflightIgnoredContextDiagnostic(t *testing.T) {
	l := &coexistenceLifecycle{}
	release := make(chan struct{})
	cleaned := make(chan int, observationLimit)
	defer close(release)
	for i := 0; i < observationLimit; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		value, err := observePreflight(ctx, l, func(context.Context) (int, error) { <-release; return 42, nil }, func(v int) { cleaned <- v })
		cancel()
		if value != 0 || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("late result escaped: %d %v", value, err)
		}
	}
	_, err := observePreflight(context.Background(), l, func(context.Context) (int, error) { t.Error("saturated observer started"); return 0, nil }, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	// Release all slots and verify every late value is discarded exactly once.
	for i := 0; i < observationLimit; i++ {
		release <- struct{}{}
	}
	for i := 0; i < observationLimit; i++ {
		select {
		case v := <-cleaned:
			if v != 42 {
				t.Fatal(v)
			}
		case <-time.After(time.Second):
			t.Fatal("missing cleanup")
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		l.observationMu.Lock()
		n := l.observations
		l.observationMu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("slots retained after completion")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestObservationSharedBudgetAndCancellation(t *testing.T) {
	l := &coexistenceLifecycle{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := observePreflight(ctx, l, func(context.Context) (int, error) { time.Sleep(20 * time.Millisecond); return 1, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = observePreflight(ctx, l, func(ctx context.Context) (int, error) { <-ctx.Done(); return 2, nil }, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	_, err = observePreflight(canceled, l, func(context.Context) (int, error) { t.Error("canceled observer started"); return 0, nil }, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// The ordinary resolver never returns until after confirmation. Independent
// discovery must not rerun it, and its late result must not continue the journal.
func TestResumeIgnoredResolverOffersRecovery(t *testing.T) {
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
	a, b, _ := lifecycleFixture(t)
	if _, err := a.Retain(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	l := b.coexistence
	l.options.ControlAuthorize = func(context.Context) error { return nil }
	l.options.RecoverySource = &containedFixture{}
	l.options.ControlDial = func(context.Context, string, *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
		return nil, errors.New("unexpected dial")
	}
	release := make(chan struct{})
	finished := make(chan struct{})
	defer func() {
		close(release)
		select {
		case <-finished:
		case <-time.After(time.Second):
		}
	}()
	l.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
		defer close(finished)
		<-release
		return a.coexistence.controlIdentity(), "127.0.0.1:1", nil
	}
	l.options.RecoveryResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
		return a.coexistence.controlIdentity(), "127.0.0.1:1", nil
	}
	l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
		return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: "127.0.0.1:2", TokenFile: "fixture"}, nil
	}
	started := time.Now()
	result, err := b.LifecycleResume(ctx, &pb.LifecycleControlRequest{Target: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "chat"}, Requester: l.controlIdentity(), DestinationEndpoint: "127.0.0.1:2"})
	if err != nil || !result.GetConfirmationRequired() {
		t.Fatalf("offer: %v %v", result, err)
	}
	if elapsed := time.Since(started); elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Fatalf("shared budget: %v", elapsed)
	}
	if _, err := os.Stat(resumeJournal(l, "chat")); !os.IsNotExist(err) {
		t.Fatalf("observation published journal: %v", err)
	}
	if l.options.RecoverySource.(*containedFixture).stop != 0 {
		t.Fatal("timeout stopped source")
	}
}
