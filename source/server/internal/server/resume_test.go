package server

import (
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/recoveryguard"
	pb "cercano/source/server/pkg/proto"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"os"
	"testing"
)

func TestDestinationResumeForceAndRetry(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	_, sourceEndpoint := serveCutoverFixture(t, a)
	bc, endpoint := serveCutoverFixture(t, b)
	ctx := context.Background()
	old, err := a.Retain(ctx, "resume-chat")
	if err != nil {
		t.Fatal(err)
	}
	b.coexistence.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
		return a.coexistence.controlIdentity(), sourceEndpoint, nil
	}
	r := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: old.Conversation}, DestinationEndpoint: endpoint}
	got, err := bc.LifecycleResume(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetOwner().GetHost() != "B" || got.GetOwner().GetEpoch() == old.Epoch || !got.Ready {
		t.Fatalf("not claimed: %v", got)
	}
	again, err := bc.LifecycleResume(ctx, r)
	if err != nil || again.GetOwner().GetEpoch() != got.Owner.Epoch || again.GetOperationId() == "" || again.GetOperationId() != got.OperationId {
		t.Fatalf("retry: %v %v", again, err)
	}
}

// These wrappers simulate protocol-version and transport boundaries, not death.
type resumeSource struct {
	*Server
	legacy, blocked, lost, unauth bool
	calls                         int
	shutdowns                     int
	beforeCooperative             func()
	forceError                    error
	legacyError                   error
	forceBlock                    bool
}

func (s *resumeSource) ShutdownAgent(context.Context, *pb.ShutdownAgentRequest) (*pb.ShutdownAgentResponse, error) {
	s.shutdowns++
	return nil, status.Error(codes.Internal, "ordinary resume must not shut down source")
}
func (s *resumeSource) LifecycleForceRelinquish(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	s.calls++
	if s.forceBlock {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	if s.forceError != nil {
		return nil, s.forceError
	}
	if s.legacy {
		return nil, status.Error(codes.Unimplemented, "old source")
	}
	result, err := s.Server.LifecycleForceRelinquish(ctx, r)
	if s.lost && err == nil {
		return nil, status.Error(codes.Unavailable, "lost source response")
	}
	return result, err
}
func (s *resumeSource) LifecycleRelinquish(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if s.beforeCooperative != nil {
		s.beforeCooperative()
	}
	if s.legacyError != nil {
		return nil, s.legacyError
	}
	if s.blocked {
		return nil, status.Error(codes.FailedPrecondition, "legacy source still has active clients")
	}
	return s.Server.LifecycleRelinquish(ctx, r)
}
func (s *resumeSource) LifecycleList(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if s.unauth {
		return nil, status.Error(codes.Unauthenticated, "bad source token")
	}
	return s.Server.LifecycleList(ctx, r)
}
func serveResumeSource(t *testing.T, s *resumeSource) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	transport := grpc.NewServer()
	pb.RegisterAgentServer(transport, s)
	go transport.Serve(listener)
	t.Cleanup(transport.Stop)
	return listener.Addr().String()
}
func resumeRequest(b *Server, endpoint string) *pb.LifecycleControlRequest {
	return &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "chat"}, DestinationEndpoint: endpoint}
}
func setResumeSource(b, a *Server, endpoint string) {
	b.coexistence.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
		return a.coexistence.controlIdentity(), endpoint, nil
	}
	b.coexistence.options.RecoveryResolveSource = b.coexistence.options.ResolveSource
}
func TestDestinationResumeLegacy(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "cooperative", true: "blocked"}[blocked], func(t *testing.T) {
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			ctx := context.Background()
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			source := &resumeSource{Server: a, legacy: true, blocked: blocked}
			setResumeSource(b, a, serveResumeSource(t, source))
			got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
			if blocked {
				if err == nil || got != nil {
					t.Fatalf("blocked old host must refuse without legacy offer: %v %v", got, err)
				}
				retry, retryErr := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
				if retryErr == nil || retry != nil {
					t.Fatalf("blocked retry: %v %v", retry, retryErr)
				}
				v, _ := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
				if v.Epoch != old.Epoch || v.Owner != old.Host {
					t.Fatal("blocked outcome changed owner")
				}
			} else if err != nil || !got.GetReady() || got.GetOwner().GetHost() != "B" {
				t.Fatalf("not cooperatively claimed: %v", got)
			}
		})
	}
}
func restartResumeFixture(t *testing.T, b *Server) (*Server, pb.AgentClient, string) {
	t.Helper()
	for _, cap := range b.coexistence.owners {
		if err := cap.Close(); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := NewCoexistenceServer(context.Background(), b.coexistence.options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, cap := range fresh.coexistence.owners {
			cap.Close()
		}
		fresh.coexistence.store.Close()
	})
	client, endpoint := serveCutoverFixture(t, fresh)
	return fresh, client, endpoint
}
func TestDestinationResumeInterruptedRetries(t *testing.T) {
	for _, point := range []string{"before-source", "after-source", "after-claim"} {
		t.Run(point, func(t *testing.T) {
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			ctx := context.Background()
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			source := &resumeSource{Server: a, lost: point == "after-source"}
			sourceEndpoint := serveResumeSource(t, source)
			setResumeSource(b, a, sourceEndpoint)
			realDial := b.coexistence.options.ControlDial
			if point == "before-source" {
				b.coexistence.options.ControlDial = func(context.Context, string, *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
					return nil, errors.New("interrupted before dial")
				}
			}
			first, firstErr := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
			if point == "after-claim" && firstErr != nil {
				t.Fatal(firstErr)
			}
			if point != "after-claim" && firstErr == nil {
				t.Fatal("interruption not exercised")
			}
			opBytes, err := os.ReadFile(resumeJournal(b.coexistence, "chat"))
			if err != nil {
				t.Fatal(err)
			}
			var op pb.LifecycleControlRequest
			if err = json.Unmarshal(opBytes, &op); err != nil {
				t.Fatal(err)
			}
			b.coexistence.options.ControlDial = realDial
			fresh, client, newEndpoint := restartResumeFixture(t, b)
			result, err := client.LifecycleResume(ctx, resumeRequest(fresh, newEndpoint))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Ready || result.Owner.Host != "B" || result.Owner.Epoch == old.Epoch || result.OperationId != op.HandoverId {
				t.Fatalf("bad retry: %v", result)
			}
			if first != nil && first.Owner.Epoch != result.Owner.Epoch {
				t.Fatal("restart minted a second epoch")
			}
			if point == "after-source" && source.calls != 1 {
				t.Fatal("reserved retry contacted source again")
			}
		})
	}
}
func TestDestinationResumeDeadOwner(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	bc, endpoint := serveCutoverFixture(t, b)
	ctx := context.Background()
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	a.coexistence.owners["chat"].Close()
	checks := 0
	b.coexistence.options.RecoveryGuard = func(_ context.Context, host string) (func(), error) {
		if host != old.Host {
			t.Fatal("wrong death target")
		}
		checks++
		return func() {}, nil
	}
	b.coexistence.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
		t.Fatal("dead owner attempted live resolution")
		return nil, "", nil
	}
	got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready || got.Owner.Host != "B" || checks != 2 {
		t.Fatalf("not automatically guarded: %v checks=%d", got, checks)
	}
}
func TestDestinationResumeRejectsSourceAuthAndOwnerRaces(t *testing.T) {
	for _, mode := range []string{"source-auth", "wrong-host", "owner-race", "caller-auth"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			ctx := context.Background()
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			source := &resumeSource{Server: a, unauth: mode == "source-auth"}
			if mode == "source-auth" {
				b.coexistence.options.ControlDial = func(context.Context, string, *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
					return nil, status.Error(codes.Unauthenticated, "bad source token")
				}
			}
			sourceEndpoint := serveResumeSource(t, source)
			calls := 0
			b.coexistence.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
				calls++
				if mode == "owner-race" && calls == 1 {
					if err := a.ReleaseRetained(ctx, old, nil); err != nil {
						t.Fatal(err)
					}
					if _, err := a.Retain(ctx, "chat"); err != nil {
						t.Fatal(err)
					}
				}
				identity := a.coexistence.controlIdentity()
				if mode == "wrong-host" {
					identity.Host = "impostor"
				}
				return identity, sourceEndpoint, nil
			}
			if mode == "caller-auth" {
				b.coexistence.options.ControlAuthorize = func(context.Context) error { return status.Error(codes.Unauthenticated, "bad destination token") }
			}
			if _, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint)); err == nil {
				t.Fatal("unsafe resume accepted")
			}
			v, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			if err != nil || v.Owner != "A" || v.State != "owned" {
				t.Fatalf("unsafe ownership mutation: %+v %v", v, err)
			}
			if mode != "owner-race" && source.calls != 0 {
				t.Fatal("unauthorized source mutation attempted")
			}
		})
	}
}

func TestDestinationResumeLegacyOwnerRaceBeforeConfirmation(t *testing.T) {
	a, b, c := lifecycleFixture(t)
	serveCutoverFixture(t, a)
	ctx := context.Background()
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	source := &resumeSource{Server: a, legacy: true, blocked: true}
	source.beforeCooperative = func() {
		if err := a.ReleaseRetained(ctx, old, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Retain(ctx, "chat"); err != nil {
			t.Fatal(err)
		}
	}
	sourceEndpoint := serveResumeSource(t, source)
	bc, endpoint := serveCutoverFixture(t, b)
	b.coexistence.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
		return a.coexistence.controlIdentity(), sourceEndpoint, nil
	}
	got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
	if err == nil {
		t.Fatalf("stale source selection offered for confirmation: %v", got)
	}
	v, err := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
	if err != nil || v.Owner != "C" || v.State != "owned" {
		t.Fatalf("racing owner changed: %+v %v", v, err)
	}
}

// Successful ordinary routes must never need a host interruption adapter.
func TestResumeOrdinaryBeforeContainedRecovery(t *testing.T) {
	for _, mode := range []string{"cooperative", "adapterless", "reserved", "guard-error", "guard-live"} {
		t.Run(mode, func(t *testing.T) {
			a, b, _ := lifecycleFixture(t)
			serveCutoverFixture(t, a)
			bc, endpoint := serveCutoverFixture(t, b)
			ctx := context.Background()
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			source := &resumeSource{Server: a, legacy: true}
			setResumeSource(b, a, serveResumeSource(t, source))
			if mode == "guard-error" {
				b.coexistence.options.RecoveryGuard = func(context.Context, string) (func(), error) { return nil, fmt.Errorf("inspection failed") }
			}
			if mode == "guard-live" {
				b.coexistence.options.RecoveryGuard = func(context.Context, string) (func(), error) { return nil, recoveryguard.ErrLiveSource }
			}
			stop := &containedFixture{}
			if mode != "adapterless" {
				b.coexistence.options.RecoverySource = stop
			}
			if mode == "reserved" {
				op := &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoint, Owner: wireOwner(old), HandoverId: "exact-reservation", Receipt: "durable-receipt", Management: true}
				if err := a.coexistence.owners["chat"].ReserveHandover(ctx, "B", op.HandoverId); err != nil {
					t.Fatal(err)
				}
				if err := saveResume(resumeJournal(b.coexistence, "chat"), op); err != nil {
					t.Fatal(err)
				}
			}
			r := resumeRequest(b, endpoint)
			r.Requester = b.coexistence.controlIdentity()
			got, err := bc.LifecycleResume(ctx, r)
			if err != nil || !got.GetReady() || stop.stop != 0 || source.shutdowns != 0 {
				t.Fatalf("ordinary route: %v %v stops=%d", got, err, stop.stop)
			}
			if got.GetOwner().GetHost() != "B" || got.GetOwner().GetEpoch() == old.Epoch || (mode == "reserved" && got.OperationId != "exact-reservation") {
				t.Fatalf("wrong exact claim: %v", got)
			}
			if mode == "reserved" && source.calls != 0 {
				t.Fatal("reservation claim contacted source")
			}
			retry, err := bc.LifecycleResume(ctx, r)
			if err != nil || retry.GetOperationId() != got.GetOperationId() || retry.GetOperationId() == "" || retry.GetOwner().GetEpoch() != got.GetOwner().GetEpoch() || stop.stop != 0 || source.shutdowns != 0 {
				t.Fatalf("retry metadata: %v %v", retry, err)
			}
		})
	}
}

func TestResumeReadyRetainsCompletedOperation(t *testing.T) {
	a, b, _ := lifecycleFixture(t)
	bc, endpoint := serveCutoverFixture(t, b)
	ctx := context.Background()
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	b.coexistence.options.RecoverySource = &containedFixture{}
	const operation = "completed-resume"
	if err := a.coexistence.owners["chat"].ReserveHandover(ctx, "B", operation); err != nil {
		t.Fatal(err)
	}
	_, err = b.coexistence.acquireUsing(ctx, "chat", nil, nil, func() (*conversation.Ownership, error) {
		return conversation.ClaimHandover(ctx, b.coexistence.store, "chat", "B", operation)
	})
	if err != nil {
		t.Fatal(err)
	}
	op := &pb.LifecycleControlRequest{Destination: b.coexistence.controlIdentity(), Owner: wireOwner(old), HandoverId: operation}
	if err := saveResume(resumeJournal(b.coexistence, "chat"), op); err != nil {
		t.Fatal(err)
	}
	got, err := bc.LifecycleResume(ctx, resumeRequest(b, endpoint))
	if err != nil || !got.GetReady() || got.GetOperationId() != operation {
		t.Fatalf("completed metadata: %v %v", got, err)
	}
}
