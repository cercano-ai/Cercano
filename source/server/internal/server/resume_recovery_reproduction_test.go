package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// All ownership/journal transitions use real lifecycle paths and disposable stores.
func TestResumeRecoverySupersession(t *testing.T) {
	for _, failCleanup := range []bool{false, true} {
		for _, restart := range []bool{false, true} {
			t.Run(map[bool]string{false: "same-process", true: "restart"}[restart]+map[bool]string{false: "", true: "-cleanup-failure"}[failCleanup], func(t *testing.T) {
				ctx := reproLoopbackContext()
				a, b, c := lifecycleFixture(t)
				serveCutoverFixture(t, a)
				_, endpoint := serveCutoverFixture(t, b)
				source := &resumeSource{Server: a, forceError: broker.ErrAdmissionBusy}
				setResumeSource(b, a, serveResumeSource(t, source))
				if _, err := a.Retain(ctx, "chat"); err != nil {
					t.Fatal(err)
				}
				p := &containedFixture{beforeStop: func() {
					// Simulated process termination releases its real ownership lock, not its row.
					if err := a.coexistence.owners["chat"].Close(); err != nil {
						t.Error(err)
					}
				}}
				b.coexistence.options.RecoverySource = p
				b.coexistence.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
					return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: endpoint, TokenFile: "fixture"}, nil
				}
				r := resumeRequest(b, endpoint)
				r.Requester = b.coexistence.controlIdentity()
				offer, err := b.LifecycleResume(ctx, r)
				if err != nil || !offer.GetConfirmationRequired() {
					t.Fatalf("offer: %v %v", offer, err)
				}
				data, err := os.ReadFile(resumeJournal(b.coexistence, "chat"))
				if err != nil {
					t.Fatal(err)
				}
				var op pb.LifecycleControlRequest
				if err = json.Unmarshal(data, &op); err != nil {
					t.Fatal(err)
				}
				if len(b.coexistence.incoming) != 1 {
					t.Fatalf("incoming before recovery: %d", len(b.coexistence.incoming))
				}
				var exact *IncomingAcceptance
				for _, v := range b.coexistence.incoming {
					exact = v
				}
				want := broker.IncomingHandover{Conversation: "chat", SourceHost: op.Owner.Host, SourceEpoch: op.Owner.Epoch, HandoverID: op.HandoverId, DestinationHost: "B"}
				if exact.h != want || exact.receipt != op.Receipt {
					t.Fatalf("acceptance does not match durable journal: %+v", exact.h)
				}
				r.RecoveryChallenge = offer.OriginalRecovery.Challenge
				r.ConfirmOriginalRecovery = true
				if failCleanup {
					b.coexistence.resumeJournalSync = func(*os.File) error { return errors.New("cleanup sync failure") }
				}
				got, err := b.LifecycleResume(ctx, r)
				if failCleanup {
					if err == nil || got != nil || b.coexistence.incoming[exact.h] != exact || p.stop != 1 {
						t.Fatalf("uncertain cleanup: %v %v", got, err)
					}
					b.coexistence.resumeJournalSync = nil
					if restart {
						// The receipt survives release as well as a crash before cleanup.
						cap := b.coexistence.owners["chat"]
						if e := b.ReleaseRetained(ctx, ownerOf(cap), nil); e != nil {
							t.Fatal(e)
						}
						b, _, endpoint = restartResumeFixture(t, b)
					}
					r = resumeRequest(b, endpoint)
					got, err = b.LifecycleResume(ctx, r)
				}

				if err != nil || !got.GetReady() || p.stop != 1 {
					t.Fatalf("recovery: %v %v stops=%d", got, err, p.stop)
				}
				if b.coexistence.incoming[exact.h] != nil {
					t.Fatal("superseded exact acceptance retained")
				}
				if _, err = conversation.HandoverResult(ctx, b.coexistence.store, "chat", "B", op.HandoverId); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("old cooperative result: %v", err)
				}
				receipt, err := conversation.HandoverResult(ctx, b.coexistence.store, "chat", "B", conversation.RecoveryID("chat", conversation.OwnershipState{Owner: op.Owner.Host, Epoch: op.Owner.Epoch}, "B"))
				if err != nil || receipt == "" {
					t.Fatalf("recovery receipt: %q %v", receipt, err)
				}
				if err = b.ReleaseRetained(ctx, controlOwner(&pb.LifecycleControlRequest{Owner: got.Owner}), nil); err != nil {
					t.Fatal(err)
				}
				foreign, err := c.Retain(ctx, "chat")
				if err != nil {
					t.Fatal(err)
				}
				r.RecoveryChallenge = ""
				r.ConfirmOriginalRecovery = false
				if restart {
					b, _, endpoint = restartResumeFixture(t, b)
					r = resumeRequest(b, endpoint)
				}
				serveCutoverFixture(t, c)
				setResumeSource(b, c, serveResumeSource(t, &resumeSource{Server: c}))
				got, err = b.LifecycleResume(ctx, r)
				if err != nil || !got.GetReady() || got.Owner.Epoch == foreign.Epoch {
					t.Fatalf("resume later foreign epoch: %v %v", got, err)
				}
				for i := 0; i < 2; i++ {
					retry, e := b.LifecycleResume(ctx, r)
					if e != nil || !retry.GetReady() || retry.Owner.Epoch != got.Owner.Epoch {
						t.Fatalf("retry: %v %v", retry, e)
					}
				}
				releaseResumeAndRetire(t, b, got)
			})
		}
	}

}

func TestResumeFencedReconnect(t *testing.T) {
	ctx := reproLoopbackContext()
	a, b, _ := lifecycleFixture(t)
	serveCutoverFixture(t, a)
	_, endpoint := serveCutoverFixture(t, b)
	owner, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	work, err := a.AdmitRetainedWork(owner, broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	if err = work.PersistenceFailed(); err != nil {
		t.Fatal(err)
	}
	r := &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Owner: wireOwner(owner), Management: true, HandoverId: "repro-fenced"}
	_, err = a.LifecycleForceRelinquish(ctx, r)
	if status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "1 unresolved persistence outcomes") {
		t.Fatalf("cutover: %v", err)
	}
	if !a.forcedCutover(owner) {
		t.Fatal("not fenced")
	}
	reconnect := &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Owner: wireOwner(owner)}
	acquired, err := a.LifecycleAcquire(ctx, reconnect)
	if err == nil || acquired.GetReady() || acquired.GetCurrentOwner() {
		t.Fatalf("closed acquire: %v %v", acquired, err)
	}
	got, err := a.LifecycleResume(ctx, reconnect)
	if !errors.Is(err, broker.ErrAdmissionBusy) || got.GetReady() || got.GetCurrentOwner() {
		t.Fatalf("closed resume: %v %v", got, err)
	}
	if ticket, e := a.AdmitRetainedWork(owner, broker.ForegroundWork); e == nil {
		ticket.Complete()
		t.Fatal("fenced owner admitted work")
	}
	v, err := conversation.ReadOwnership(ctx, a.coexistence.store, "chat")
	if err != nil || v.State != "owned" || v.Owner != owner.Host || v.Epoch != owner.Epoch {
		t.Fatalf("closed admission must not change durable ownership: %+v %v", v, err)
	}

}

// The fake peer blocks only a real preflight RPC, never fabricates ownership.
type reproListPeer struct {
	*Server
	entered chan bool
	release chan struct{}
}

func (p *reproListPeer) LifecycleList(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	_, deadline := ctx.Deadline()
	p.entered <- deadline
	select {
	case <-p.release:
		return p.Server.LifecycleList(ctx, r)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func reproQueuedResume(s *Server, ctx context.Context, r *pb.LifecycleControlRequest, done chan error) {
	_, err := s.LifecycleResume(ctx, r)
	done <- err
}

func TestResumePreflightIsolation(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	serveCutoverFixture(t, a)
	_, endpoint := serveCutoverFixture(t, b)
	if _, err := a.Retain(context.Background(), "chat"); err != nil {
		t.Fatal(err)
	}
	p := &reproListPeer{Server: a, entered: make(chan bool, 1), release: make(chan struct{})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	transport := grpc.NewServer()
	pb.RegisterAgentServer(transport, p)
	go transport.Serve(listener)
	defer transport.Stop()
	setResumeSource(b, a, listener.Addr().String())
	ctx, cancel := context.WithCancel(reproLoopbackContext())
	defer cancel()
	first := make(chan error, 1)
	var initial *pb.LifecycleControlResponse
	joined := false
	go func() { var e error; initial, e = b.LifecycleResume(ctx, resumeRequest(b, endpoint)); first <- e }()
	defer func() {
		cancel()
		if joined {
			return
		}
		select {
		case <-first:
		case <-time.After(5 * time.Second):
			t.Error("first resume did not join")
		}
	}()
	select {
	case deadline := <-p.entered:
		if !deadline {
			t.Fatal("preflight must be bounded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("preflight not reached")
	}
	// A real same-conversation RPC must wait before touching its durable journal.
	journal := resumeJournal(b.coexistence, "chat")
	before, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	var operation pb.LifecycleControlRequest
	if err := json.Unmarshal(before, &operation); err != nil {
		t.Fatal(err)
	}
	sameCtx, sameCancel := context.WithCancel(reproLoopbackContext())
	sameDone := make(chan error, 1)
	go reproQueuedResume(b, sameCtx, resumeRequest(b, endpoint), sameDone)
	sameJoined := false
	defer func() {
		sameCancel()
		if !sameJoined {
			<-sameDone
		}
	}()
	until := time.After(time.Second)
	for {
		b.coexistence.resumeGateMu.Lock()
		refs := b.coexistence.resumeGates["chat"].refs
		b.coexistence.resumeGateMu.Unlock()
		if refs == 2 {
			break
		}
		select {
		case e := <-sameDone:
			sameJoined = true
			t.Fatalf("same conversation escaped gate: %v", e)
		case <-until:
			t.Fatal("same conversation did not queue")
		default:
			runtime.Gosched()
		}
	}
	sameCancel()
	select {
	case e := <-sameDone:
		sameJoined = true
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("queued cancellation: %v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("queued resume did not cancel")
	}
	after, err := os.ReadFile(journal)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("queued resume changed journal: %v", err)
	}
	queuedCtx, queuedCancel := context.WithCancel(reproLoopbackContext())
	queuedCancel()
	done := make(chan error, 1)
	r := resumeRequest(b, endpoint)
	r.Owner = &pb.LifecycleOwner{Conversation: "unrelated"}
	go reproQueuedResume(b, queuedCtx, r, done)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled waiter succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter blocked")
	}
	live := make(chan error, 1)
	go reproQueuedResume(b, reproLoopbackContext(), r, live)
	select {
	case err := <-live:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("unrelated resume blocked")
	}
	close(p.release)
	select {
	case err := <-first:
		joined = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("released resume blocked")
	}
	retry, err := b.LifecycleResume(reproLoopbackContext(), resumeRequest(b, endpoint))
	if err != nil || !retry.GetReady() || initial.GetOperationId() == "" || retry.GetOperationId() != initial.GetOperationId() {
		t.Fatalf("unstable retry: %v %v", retry, err)
	}
	after, err = os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	var retried pb.LifecycleControlRequest
	if err := json.Unmarshal(after, &retried); err != nil {
		t.Fatal(err)
	}
	if retried.Receipt == "" || retried.Receipt != operation.Receipt || retried.HandoverId != operation.HandoverId || retry.OperationId != operation.HandoverId {
		t.Fatal("operation/receipt changed across serialized retry")
	}
	b.coexistence.resumeGateMu.Lock()
	remaining := len(b.coexistence.resumeGates)
	b.coexistence.resumeGateMu.Unlock()
	if remaining != 0 {
		t.Fatal("resume leaked gate entries")
	}
}

func reproLoopbackContext() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
}
