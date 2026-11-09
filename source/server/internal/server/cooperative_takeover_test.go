package server

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func takeoverFixture(t *testing.T, interceptors ...grpc.UnaryServerInterceptor) (*exactRecoveryFixture, *pb.LifecycleControlRequest) {
	t.Helper()
	f := newExactRecoveryFixture(t, interceptors...)
	l := f.c.coexistence
	l.options.ResolveSource = func(_ context.Context, host string) (*pb.LifecycleIdentity, string, error) {
		for _, s := range []*Server{f.a, f.b, f.c} {
			if s.coexistence.options.Host == host {
				return s.coexistence.controlIdentity(), f.endpoints[s], nil
			}
		}
		return nil, "", status.Error(codes.NotFound, "unknown host")
	}
	l.options.RecoverySelection = func(_ context.Context, host string) (*pb.LifecycleRecoverySelection, error) {
		return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: f.endpoints[f.c], TokenFile: "fixture-private-token"}, nil
	}
	return f, &pb.LifecycleControlRequest{Target: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "chat"}, Requester: f.b.coexistence.controlIdentity(), DestinationEndpoint: f.endpoints[f.c]}
}
func takeoverOffer(t *testing.T, f *exactRecoveryFixture, r *pb.LifecycleControlRequest) *pb.LifecycleControlRequest {
	t.Helper()
	got, e := f.clients[f.c].LifecycleResume(f.ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if !got.GetConfirmationRequired() || !got.GetOriginalRecovery().GetTakeover() {
		t.Fatalf("no takeover offer: %v", got)
	}
	confirm := proto.Clone(r).(*pb.LifecycleControlRequest)
	confirm.RecoveryChallenge = got.OriginalRecovery.Challenge
	confirm.ConfirmOriginalRecovery = true
	return confirm
}

func TestTakeoverConfirmationIndependentOfJournalsAndOutgoingRequest(t *testing.T) {
	f, r := takeoverFixture(t)
	// A conflicting/corrupt journal must not prevent offering for durable ownership.
	dir := f.c.coexistence.options.StorePath + ".resume-operations"
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "conflict.json"), []byte("not a journal"), 0600); e != nil {
		t.Fatal(e)
	}
	// Model an inactive abandoned outgoing request to B with retained authority.
	l := f.a.coexistence
	o := controlOwner(f.op)
	closure, e := l.admission.ForceCloseOwner(o)
	if e != nil {
		t.Fatal(e)
	}
	ticket, e := l.admission.AdmitHostWork(broker.PersistenceWork)
	if e != nil {
		t.Fatal(e)
	}
	l.mu.Lock()
	l.closures = make(map[broker.Owner]*relinquishReservation)
	l.closures[o] = &relinquishReservation{closure: closure, ticket: ticket, control: &outgoingControl{request: proto.Clone(f.op).(*pb.LifecycleControlRequest)}}
	l.mu.Unlock()
	confirm := takeoverOffer(t, f, r)
	got, e := f.clients[f.c].LifecycleTakeover(f.ctx, confirm)
	if e != nil {
		t.Fatal(e)
	}
	if !got.Ready || !got.CurrentOwner || got.Owner.Host != "C" || got.Owner.Epoch == o.Epoch || got.BlockedReason == "" {
		t.Fatalf("unsafe result %v", got)
	}
	state, e := conversation.ReadOwnership(f.ctx, l.store, "chat")
	if e != nil || state.Owner != "C" || state.Epoch != got.Owner.Epoch {
		t.Fatalf("not current: %+v %v", state, e)
	}
	if _, e = f.clients[f.c].LifecycleTakeover(f.ctx, confirm); status.Code(e) != codes.PermissionDenied {
		t.Fatalf("reused challenge: %v", e)
	}
	// The abandoned B acceptance is not authority after the new claim.
	if _, e = f.clients[f.b].LifecycleClaim(f.ctx, f.claim()); e == nil {
		t.Fatal("old acceptance claimed")
	}
}
func TestTakeoverRejectsStaleExpiredAndDuplicateConfirmation(t *testing.T) {
	for _, mode := range []string{"stale", "expired", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			f, r := takeoverFixture(t)
			confirm := takeoverOffer(t, f, r)
			switch mode {
			case "stale":
				f.reserve(t)
			case "expired":
				l := f.c.coexistence
				l.resumeMu.Lock()
				l.recoveryOffers[confirm.RecoveryChallenge].expires = time.Now().Add(-time.Second)
				l.resumeMu.Unlock()
			case "duplicate":
				var wg sync.WaitGroup
				results := make(chan error, 2)
				for i := 0; i < 2; i++ {
					wg.Add(1)
					go func() { defer wg.Done(); _, e := f.clients[f.c].LifecycleTakeover(f.ctx, confirm); results <- e }()
				}
				wg.Wait()
				close(results)
				successes := 0
				for e := range results {
					if e == nil {
						successes++
					} else if status.Code(e) != codes.PermissionDenied {
						t.Fatal(e)
					}
				}
				if successes != 1 {
					t.Fatalf("%d successes", successes)
				}
				return
			}
			if _, e := f.clients[f.c].LifecycleTakeover(f.ctx, confirm); e == nil {
				t.Fatal("invalid consent accepted")
			}
			if _, e := f.clients[f.c].LifecycleTakeover(f.ctx, confirm); status.Code(e) != codes.PermissionDenied {
				t.Fatalf("failed challenge reusable: %v", e)
			}
		})
	}
}
func TestTakeoverRefusesUncertainPersistenceAndMissingAuthority(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncertain", true: "committed-old-handover"}[missing], func(t *testing.T) {
			f, r := takeoverFixture(t)
			if missing {
				f.reserve(t)
				if _, e := f.clients[f.c].LifecycleResume(f.ctx, r); e == nil {
					t.Fatal("invented quiescence authority")
				}
				return
			}
			work, e := f.a.coexistence.admission.AdmitWork(controlOwner(f.op), broker.PersistenceWork)
			if e != nil {
				t.Fatal(e)
			}
			if e = work.PersistenceFailed(); e != nil {
				t.Fatal(e)
			}
			confirm := takeoverOffer(t, f, r)
			if _, e = f.clients[f.c].LifecycleTakeover(f.ctx, confirm); status.Code(e) != codes.FailedPrecondition {
				t.Fatalf("uncertainty accepted: %v", e)
			}
			state, e := conversation.ReadOwnership(f.ctx, f.a.coexistence.store, "chat")
			if e != nil || state.Owner != "A" || state.State != "owned" {
				t.Fatalf("ownership changed %+v %v", state, e)
			}
			if e = work.Complete(); e == nil {
				t.Fatal("uncertain ticket completed")
			}
		})
	}
}

func TestTakeoverUnsupportedPeerNeverFallsBack(t *testing.T) {
	for _, method := range []string{pb.Agent_LifecycleTakeoverProbe_FullMethodName, pb.Agent_LifecycleTakeoverReceipt_FullMethodName} {
		t.Run(method, func(t *testing.T) {
			f, r := takeoverFixture(t, func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				if info.FullMethod == method {
					return nil, status.Error(codes.Unimplemented, "old peer")
				}
				return next(ctx, req)
			})
			var err error
			if method == pb.Agent_LifecycleTakeoverProbe_FullMethodName {
				_, err = f.clients[f.c].LifecycleResume(f.ctx, r)
			} else {
				confirm := takeoverOffer(t, f, r)
				_, err = f.clients[f.c].LifecycleTakeover(f.ctx, confirm)
			}
			if status.Code(err) != codes.Unimplemented {
				t.Fatalf("unsupported peer did not fail closed: %v", err)
			}
			state, err := conversation.ReadOwnership(f.ctx, f.a.coexistence.store, "chat")
			if err != nil || state.Owner != "A" || state.Epoch != f.op.Owner.Epoch || state.State != "owned" {
				t.Fatalf("unsupported control changed ownership: %+v %v", state, err)
			}
			if retired, retained, open := f.a.coexistence.admission.CheckRetained(controlOwner(f.op)); retired || !retained || !open {
				t.Fatalf("unsupported destination changed source admission: retired=%v retained=%v open=%v", retired, retained, open)
			}
		})
	}
}

func TestTakeoverConsentBindsRequesterAndExactDestination(t *testing.T) {
	for _, change := range []string{"requester", "destination", "legacy-stop"} {
		t.Run(change, func(t *testing.T) {
			f, r := takeoverFixture(t)
			confirm := takeoverOffer(t, f, r)
			if change == "requester" {
				confirm.Requester = f.a.coexistence.controlIdentity()
			} else if change == "legacy-stop" {
				confirm.ConfirmLegacyStop = true
			} else {
				l := f.c.coexistence
				l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
					return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: f.endpoints[f.c], TokenFile: "changed-token-selection"}, nil
				}
			}
			if _, err := f.clients[f.c].LifecycleTakeover(f.ctx, confirm); err == nil {
				t.Fatal("changed consent accepted")
			}
			if _, err := f.clients[f.c].LifecycleTakeover(f.ctx, confirm); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("failed consent reusable: %v", err)
			}
			state, err := conversation.ReadOwnership(f.ctx, f.a.coexistence.store, "chat")
			if err != nil || state.Owner != "A" || state.State != "owned" {
				t.Fatalf("changed consent modified source: %+v %v", state, err)
			}
		})
	}
}

func TestTakeoverFreshConsentAfterLostResponse(t *testing.T) {
	for _, method := range []string{pb.Agent_LifecycleTakeoverRelinquish_FullMethodName, pb.Agent_LifecycleTakeover_FullMethodName} {
		t.Run(method, func(t *testing.T) {
			var mu sync.Mutex
			lost := false
			f, r := takeoverFixture(t, func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				result, err := next(ctx, req)
				mu.Lock()
				defer mu.Unlock()
				if info.FullMethod == method && err == nil && !lost {
					lost = true
					return nil, status.Error(codes.Unavailable, "lost successful response")
				}
				return result, err
			})
			first := takeoverOffer(t, f, r)
			if _, err := f.clients[f.c].LifecycleTakeover(f.ctx, first); status.Code(err) != codes.Unavailable {
				t.Fatal(err)
			}
			fresh := takeoverOffer(t, f, r)
			got, err := f.clients[f.c].LifecycleTakeover(f.ctx, fresh)
			if err != nil || !got.GetReady() {
				t.Fatalf("%v %v", got, err)
			}
		})
	}
}

func TestTakeoverFreshConsentAfterPersistenceResolution(t *testing.T) {
	f, r := takeoverFixture(t)
	work, err := f.a.coexistence.admission.AdmitWork(controlOwner(f.op), broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	if err = work.PersistenceFailed(); err != nil {
		t.Fatal(err)
	}
	first := takeoverOffer(t, f, r)
	if _, err = f.clients[f.c].LifecycleTakeover(f.ctx, first); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if err = work.ResolvePersistence(); err != nil {
		t.Fatal(err)
	}
	if err = work.Complete(); err != nil {
		t.Fatal(err)
	}
	fresh := takeoverOffer(t, f, r)
	got, err := f.clients[f.c].LifecycleTakeover(f.ctx, fresh)
	if err != nil || !got.GetReady() {
		t.Fatalf("%v %v", got, err)
	}
}

func TestTakeoverRejectsObsoleteRetainedOperation(t *testing.T) {
	f, r := takeoverFixture(t)
	got, err := f.clients[f.c].LifecycleTakeover(f.ctx, takeoverOffer(t, f, r))
	if err != nil {
		t.Fatal(err)
	}
	fresh := takeoverOffer(t, f, r)
	if err = f.c.ReleaseRetained(f.ctx, controlOwner(&pb.LifecycleControlRequest{Owner: got.Owner}), nil); err != nil {
		t.Fatal(err)
	}
	if _, err = f.c.Retain(f.ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	before, err := conversation.ReadOwnership(f.ctx, f.c.coexistence.store, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := f.clients[f.c].LifecycleTakeover(f.ctx, fresh); err == nil || result.GetReady() {
		t.Fatalf("obsolete operation ready: %v %v", result, err)
	}
	after, err := conversation.ReadOwnership(f.ctx, f.c.coexistence.store, "chat")
	if err != nil || before != after {
		t.Fatalf("obsolete operation mutated owner: %v %v", after, err)
	}
}
