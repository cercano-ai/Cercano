package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResumeGateCancellationAndCleanup(t *testing.T) {
	l := &coexistenceLifecycle{}
	for i := 0; i < 100; i++ {
		release, err := l.enterResume(context.Background(), "chat")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			unlock, err := l.enterResume(ctx, "chat")
			if unlock != nil {
				unlock()
				done <- fmt.Errorf("same-conversation overlap")
				return
			}
			done <- err
		}()
		until := time.Now().Add(time.Second)
		for {
			l.resumeGateMu.Lock()
			refs := l.resumeGates["chat"].refs
			l.resumeGateMu.Unlock()
			if refs == 2 {
				break
			}
			if time.Now().After(until) {
				t.Fatal("waiter not registered")
			}
			runtime.Gosched()
		}
		other, err := l.enterResume(context.Background(), "other")
		if err != nil {
			t.Fatal(err)
		}
		other()
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("waiter blocked")
		}
		release()
		l.resumeGateMu.Lock()
		n := len(l.resumeGates)
		l.resumeGateMu.Unlock()
		if n != 0 {
			t.Fatal("leaked entries")
		}
	}
}

func TestResumeGateContextTokenRace(t *testing.T) {
	l := &coexistenceLifecycle{}
	for i := 0; i < 1000; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		release, _ := l.enterResume(context.Background(), "chat")
		done := make(chan error, 1)
		go func() {
			unlock, err := l.enterResume(ctx, "chat")
			if unlock != nil {
				unlock()
			}
			done <- err
		}()
		cancel()
		release()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("expired token winner: %v", err)
		}
		if len(l.resumeGates) != 0 {
			t.Fatal("leaked race entry")
		}
	}
}

func TestBoundedPreflightExpirationClassification(t *testing.T) {
	for _, translated := range []error{nil, context.DeadlineExceeded, status.Error(codes.DeadlineExceeded, "peer deadline"), status.Error(codes.Unavailable, "translated"), status.Error(codes.PermissionDenied, "auth")} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		_, err := boundedPreflight(ctx, func(phase context.Context) (int, error) { <-phase.Done(); return 0, translated })
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expired phase classified as %v", err)
		}
	}
	for _, deadline := range []error{context.DeadlineExceeded, status.Error(codes.DeadlineExceeded, "peer deadline")} {
		_, err := boundedPreflight(context.Background(), func(context.Context) (int, error) { return 0, deadline })
		if !errors.Is(err, context.DeadlineExceeded) && status.Code(err) != codes.DeadlineExceeded {
			t.Fatalf("deadline classification: %v", err)
		}
	}
	_, err := boundedPreflight(context.Background(), func(phase context.Context) (int, error) {
		deadline, ok := phase.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("missing phase bound")
		}
		return 0, status.Error(codes.PermissionDenied, "identity")
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
}

func TestRecoveryOfferConcurrentRegistry(t *testing.T) {
	l := &coexistenceLifecycle{}
	offer := func(id string, expiry time.Time) *containedRecoveryConsent {
		return &containedRecoveryConsent{requester: &pb.LifecycleIdentity{Host: "requester"}, offer: &pb.LifecycleOriginalRecovery{Operation: &pb.LifecycleControlRequest{Owner: &pb.LifecycleOwner{Conversation: id}}}, expires: expiry}
	}
	var wg sync.WaitGroup
	var inserted atomic.Int32
	for i := 0; i < 256; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprint(i)
			err := l.publishRecoveryOffer(id, offer(id, time.Now().Add(time.Minute)))
			if err == nil {
				inserted.Add(1)
			} else if status.Code(err) != codes.ResourceExhausted {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if inserted.Load() != 128 || len(l.recoveryOffers) != 128 {
		t.Fatal("capacity not atomic")
	}
	var key string
	for k := range l.recoveryOffers {
		key = k
		break
	}
	var consumed atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.takeRecoveryOffer(key) != nil {
				consumed.Add(1)
			}
		}()
	}
	wg.Wait()
	if consumed.Load() != 1 {
		t.Fatal("duplicate consumption")
	}
	// Expired entries free capacity during publication, without a background sweep.
	for k, c := range l.recoveryOffers {
		l.recoveryOffers[k] = offer(c.offer.Operation.Owner.Conversation, time.Now().Add(-time.Second))
	}
	if err := l.publishRecoveryOffer("fresh", offer("fresh", time.Now().Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if len(l.recoveryOffers) != 1 {
		t.Fatal("expired offers retained")
	}
}

func TestRecoveryOfferConcurrentBinding(t *testing.T) {
	l := &coexistenceLifecycle{}
	var wg sync.WaitGroup
	for _, host := range []string{"one", "two"} {
		for _, conversation := range []string{"chat", "other"} {
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(host, conversation string, i int) {
					defer wg.Done()
					c := &containedRecoveryConsent{requester: &pb.LifecycleIdentity{Host: host}, offer: &pb.LifecycleOriginalRecovery{Operation: &pb.LifecycleControlRequest{Owner: &pb.LifecycleOwner{Conversation: conversation}}}, expires: time.Now().Add(time.Minute)}
					if err := l.publishRecoveryOffer(fmt.Sprintf("%s/%s/%d", host, conversation, i), c); err != nil {
						t.Error(err)
					}
				}(host, conversation, i)
			}
		}
	}
	wg.Wait()
	if len(l.recoveryOffers) != 4 {
		t.Fatal("refresh crossed requester/conversation binding")
	}
	seen := map[string]bool{}
	for _, c := range l.recoveryOffers {
		key := c.requester.Host + "/" + c.offer.Operation.Owner.Conversation
		if seen[key] {
			t.Fatal("duplicate binding")
		}
		seen[key] = true
	}
}
