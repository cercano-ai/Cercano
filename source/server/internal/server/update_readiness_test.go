package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
)

func TestUpdateReadinessRealStreams(t *testing.T) {
	s := &Server{events: newEventHub()}
	client, _ := drainRPCClient(t, s, s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, err := client.SubscribeEvents(ctx, &proto.SubscribeEventsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitSubscribers := func(n int) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for s.events.subscriberCount() != n {
			if time.Now().After(deadline) {
				t.Fatal("subscriber timeout")
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitSubscribers(1)
	token := strings.Repeat("a", 32)
	// Invalid input and a failed seal must not announce anything.
	bad, _ := client.DrainForUpdate(ctx, &proto.DrainForUpdateRequest{OperationToken: "bad"})
	if _, e := bad.Recv(); e == nil {
		t.Fatal("invalid accepted")
	}
	s.updateWork.mu.Lock()
	s.updateWork.paused = true
	s.updateWork.mu.Unlock()
	failed, _ := client.DrainForUpdate(ctx, &proto.DrainForUpdateRequest{OperationToken: token})
	if _, e := failed.Recv(); e == nil {
		t.Fatal("failed seal accepted")
	}
	s.events.mu.Lock()
	announced := s.events.readinessRevision
	s.events.mu.Unlock()
	if announced != 0 {
		t.Fatal("false announcement")
	}
	s.updateWork.mu.Lock()
	s.updateWork.paused = false
	s.updateWork.mu.Unlock()
	work, err := s.updateWork.enter()
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx, stop := context.WithCancel(ctx)
	defer stop()
	owner, err := client.DrainForUpdate(ownerCtx, &proto.DrainForUpdateRequest{OperationToken: token})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, e := owner.Recv(); e != nil {
			t.Fatal(e)
		}
	}
	read := func(stream proto.Agent_SubscribeEventsClient, phase proto.UpdateReadiness_Phase) {
		t.Helper()
		ev, e := stream.Recv()
		if e != nil {
			t.Fatal(e)
		}
		n := ev.GetUpdateReadiness()
		if n.GetOperationToken() != token || n.GetPhase() != phase {
			t.Fatalf("unexpected notice %v", ev)
		}
	}
	read(first, proto.UpdateReadiness_DRAINING)
	late, e := client.SubscribeEvents(ctx, &proto.SubscribeEventsRequest{})
	if e != nil {
		t.Fatal(e)
	}
	read(late, proto.UpdateReadiness_DRAINING)
	competing, _ := client.DrainForUpdate(ctx, &proto.DrainForUpdateRequest{OperationToken: token})
	if _, e := competing.Recv(); e == nil {
		t.Fatal("competing owner accepted")
	}
	s.events.broadcast(&proto.ClientEvent{Event: &proto.ClientEvent_PermissionModeChanged{PermissionModeChanged: &proto.PermissionModeChanged{Mode: "strict"}}})
	for _, stream := range []proto.Agent_SubscribeEventsClient{first, late} {
		ev, e := stream.Recv()
		if e != nil || ev.GetPermissionModeChanged().GetMode() != "strict" {
			t.Fatal("ordinary event lost", ev, e)
		}
	}
	work()
	if _, e := owner.Recv(); e != nil {
		t.Fatal(e)
	}
	read(first, proto.UpdateReadiness_IDLE)
	read(late, proto.UpdateReadiness_IDLE)
	stop()
	read(first, proto.UpdateReadiness_RELEASED)
	read(late, proto.UpdateReadiness_RELEASED)
	// Late subscribers after release see empty live state, not historical release.
	empty, e := client.SubscribeEvents(ctx, &proto.SubscribeEventsRequest{})
	if e != nil {
		t.Fatal(e)
	}
	waitSubscribers(3)
	s.events.broadcast(&proto.ClientEvent{Event: &proto.ClientEvent_PermissionModeChanged{PermissionModeChanged: &proto.PermissionModeChanged{Mode: "strict"}}})
	ev, e := empty.Recv()
	if e != nil || ev.GetPermissionModeChanged() == nil {
		t.Fatal("release replayed", ev, e)
	}
}

func TestUpdateReadinessGenerationAndCoalescing(t *testing.T) {
	h := newEventHub()
	ch, unsubscribe := h.subscribe()
	defer unsubscribe()
	for i := 0; i < 16; i++ {
		h.broadcast(&proto.ClientEvent{Event: &proto.ClientEvent_ConfigChanged{ConfigChanged: &proto.ConfigChanged{Field: "x"}}})
	}
	h.setUpdateReadiness(1, "old", proto.UpdateReadiness_DRAINING)
	h.setUpdateReadiness(2, "new", proto.UpdateReadiness_DRAINING)
	h.setUpdateReadiness(1, "old", proto.UpdateReadiness_RELEASED)
	if h.readiness.GetUpdateReadiness().GetOperationToken() != "new" {
		t.Fatal("old owner cleared new")
	}
	h.setUpdateReadiness(2, "new", proto.UpdateReadiness_RELEASED)
	h.setUpdateReadiness(2, "new", proto.UpdateReadiness_IDLE)
	if h.readiness.GetUpdateReadiness().GetPhase() != proto.UpdateReadiness_RELEASED {
		t.Fatal("delayed callback resurrected owner")
	}
	for i := 0; i < 16; i++ {
		if (<-ch).GetConfigChanged() == nil {
			t.Fatal("ordinary queue displaced")
		}
	}
}

func TestUpdateReadinessCallbacksOutsideAdmissionLock(t *testing.T) {
	var g updateAdmission
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release, e := g.drainObserved(ctx, func(_ uint64, _ proto.UpdateReadiness_Phase) { g.mu.Lock(); g.mu.Unlock() })
	if e != nil {
		t.Fatal(e)
	}
	release()
}

// Pause delivery to deterministically coalesce an entire intervening owner.
type readinessBlockedStream struct {
	proto.Agent_SubscribeEventsServer
	ctx    context.Context
	sent   chan *proto.ClientEvent
	resume chan struct{}
}

func (s *readinessBlockedStream) Context() context.Context { return s.ctx }
func (s *readinessBlockedStream) Send(e *proto.ClientEvent) error {
	select {
	case s.sent <- e:
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	select {
	case <-s.resume:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}
func TestUpdateReadinessCoalescedReleaseClearsDisplayedOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s := &Server{events: newEventHub()}
	stream := &readinessBlockedStream{ctx: ctx, sent: make(chan *proto.ClientEvent), resume: make(chan struct{})}
	s.events.setUpdateReadiness(1, "old", proto.UpdateReadiness_IDLE)
	done := make(chan error, 1)
	go func() { done <- s.SubscribeEvents(&proto.SubscribeEventsRequest{}, stream) }()
	receive := func() *proto.UpdateReadiness {
		t.Helper()
		select {
		case e := <-stream.sent:
			return e.GetUpdateReadiness()
		case <-ctx.Done():
			t.Fatal("delivery timeout")
			return nil
		}
	}
	if receive().GetOperationToken() != "old" {
		t.Fatal("missing original")
	}
	s.events.setUpdateReadiness(1, "old", proto.UpdateReadiness_RELEASED)
	s.events.setUpdateReadiness(2, "new", proto.UpdateReadiness_IDLE)
	s.events.setUpdateReadiness(2, "new", proto.UpdateReadiness_RELEASED)
	stream.resume <- struct{}{}
	if n := receive(); n.GetOperationToken() != "old" || n.GetPhase() != proto.UpdateReadiness_RELEASED {
		t.Fatal("displayed owner not withdrawn", n)
	}
	cancel()
	<-done
}

func TestReadinessReleaseKeepsSealUntilWithdrawal(t *testing.T) {
	var g updateAdmission
	entered, resume := make(chan struct{}), make(chan struct{})
	release, err := g.drainObserved(context.Background(), func(_ uint64, phase proto.UpdateReadiness_Phase) {
		if phase == proto.UpdateReadiness_RELEASED {
			close(entered)
			<-resume
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { release(); close(done) }()
	<-entered
	work, admissionErr := g.enter()
	if work != nil {
		work()
	}
	_, ownerErr := g.drain(context.Background())
	concurrentDone := make(chan struct{})
	go func() { release(); close(concurrentDone) }()
	close(resume)
	<-done
	<-concurrentDone
	next, err := g.drain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release() // stale release must not reopen the next owner
	if work, err := g.enter(); err == nil {
		work()
		t.Fatal("old release stole owner")
	}
	next()
	if admissionErr == nil || ownerErr == nil {
		t.Fatal("admission reopened before readiness withdrawal")
	}
}

func TestZeroServerReadinessBeforeSubscribe(t *testing.T) {
	s := &Server{}
	client, _ := drainRPCClient(t, s, s)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	owner, err := client.DrainForUpdate(ctx, &proto.DrainForUpdateRequest{OperationToken: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = owner.Recv(); err != nil {
			t.Fatal(err)
		}
	}
	sub, err := client.SubscribeEvents(ctx, &proto.SubscribeEventsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ev, err := sub.Recv()
	if err != nil || ev.GetUpdateReadiness().GetPhase() != proto.UpdateReadiness_IDLE {
		t.Fatal("lost pre-subscription readiness", ev, err)
	}
}

func TestReadinessCancelDuringCallback(t *testing.T) {
	var g updateAdmission
	ctx, cancel := context.WithCancel(context.Background())
	entered, resume := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	var phases []proto.UpdateReadiness_Phase
	go func() {
		_, err := g.drainObserved(ctx, func(_ uint64, p proto.UpdateReadiness_Phase) {
			phases = append(phases, p)
			if p == proto.UpdateReadiness_DRAINING {
				close(entered)
				<-resume
			}
		})
		done <- err
	}()
	<-entered
	cancel()
	close(resume)
	if err := <-done; err == nil {
		t.Fatal("cancellation ignored")
	}
	if len(phases) != 2 || phases[1] != proto.UpdateReadiness_RELEASED {
		t.Fatal(phases)
	}
	work, err := g.enter()
	if err != nil {
		t.Fatal(err)
	}
	work()
}

func TestZeroServerConcurrentSubscriptionsAndDrain(t *testing.T) {
	s := &Server{}
	client, _ := drainRPCClient(t, s, s)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() {
			sub, err := client.SubscribeEvents(ctx, &proto.SubscribeEventsRequest{})
			if err == nil {
				_, err = sub.Recv()
			}
			done <- err
		}()
	}
	owner, err := client.DrainForUpdate(ctx, &proto.DrainForUpdateRequest{OperationToken: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = owner.Recv(); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 16; i++ {
		if err = <-done; err != nil {
			t.Fatal(err)
		}
	}
}
