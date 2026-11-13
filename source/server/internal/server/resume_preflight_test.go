package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type countedPreflightRecovery struct {
	containedFixture
	prepares atomic.Int32
}

func (p *countedPreflightRecovery) Prepare(ctx context.Context, id *pb.LifecycleIdentity) (string, error) {
	p.prepares.Add(1)
	return p.containedFixture.Prepare(ctx, id)
}

func TestResumePreAuthTimeoutMatrix(t *testing.T) {
	for _, mode := range []string{"raw", "grpc", "translated"} {
		for _, phase := range []string{"resolve-new", "resolve-journal", "dial", "list", "list-child", "resolve-child", "recovery-resolve", "recovery-selection"} {
			t.Run(mode+"/"+phase, func(t *testing.T) {
				a, b, _ := lifecycleFixture(t)
				serveCutoverFixture(t, a)
				_, endpoint := serveCutoverFixture(t, b)
				if _, err := a.Retain(context.Background(), "chat"); err != nil {
					t.Fatal(err)
				}
				failure := func(ctx context.Context) error {
					if mode == "raw" {
						return context.DeadlineExceeded
					}
					if mode == "grpc" {
						return status.Error(codes.DeadlineExceeded, "peer deadline")
					}
					<-ctx.Done()
					return status.Error(codes.Unavailable, "adapter translated deadline")
				}
				var force atomic.Int32
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				transport := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
					if info.FullMethod == pb.Agent_LifecycleForceRelinquish_FullMethodName {
						force.Add(1)
					}
					if info.FullMethod == pb.Agent_LifecycleList_FullMethodName {
						e := failure(ctx)
						if errors.Is(e, context.DeadlineExceeded) {
							e = status.FromContextError(e).Err()
						}
						return nil, e
					}
					return handler(ctx, req)
				}))
				pb.RegisterAgentServer(transport, a)
				go transport.Serve(listener)
				defer transport.Stop()
				setResumeSource(b, a, listener.Addr().String())
				l := b.coexistence
				stop := &countedPreflightRecovery{}
				l.options.RecoverySource = stop
				l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
					return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: endpoint, TokenFile: "fixture"}, nil
				}
				calls := 0
				resolve := l.options.ResolveSource
				translated := failure
				l.options.ResolveSource = func(ctx context.Context, host string) (*pb.LifecycleIdentity, string, error) {
					calls++
					if phase == "resolve-new" || phase == "resolve-child" || phase == "recovery-resolve" || (phase == "resolve-journal" && calls == 2) {
						return nil, "", translated(ctx)
					}
					return resolve(ctx, host)
				}
				if phase == "recovery-resolve" {
					l.options.RecoveryResolveSource = l.options.ResolveSource
				}
				if phase == "dial" {
					l.options.ControlDial = func(ctx context.Context, _ string, _ *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
						return nil, translated(ctx)
					}
				}
				if phase == "recovery-selection" {
					l.options.RecoverySelection = func(ctx context.Context, _ string) (*pb.LifecycleRecoverySelection, error) {
						return nil, translated(ctx)
					}
				}
				r := resumeRequest(b, endpoint)
				r.Requester = l.controlIdentity()
				if phase == "recovery-resolve" || phase == "recovery-selection" {
					r.DestinationEndpoint = ""
				}
				budget := 30 * time.Millisecond
				if phase == "list-child" || phase == "resolve-child" {
					budget = 10 * time.Second
				}
				ctx, cancel := context.WithTimeout(reproLoopbackContext(), budget)
				defer cancel()
				result, err := b.LifecycleResume(ctx, r)
				if budget == 10*time.Second && ctx.Err() != nil {
					t.Fatal("caller expired before child phase returned")
				}
				wantOffer := phase != "recovery-resolve" && phase != "recovery-selection" && (mode != "translated" || budget == 10*time.Second)
				if wantOffer && ctx.Err() == nil {
					if err != nil || !result.GetConfirmationRequired() || stop.stop != 0 || stop.prepares.Load() != 1 || force.Load() != 0 {
						t.Fatalf("timeout did not offer safe confirmation: %v %v", result, err)
					}
					return
				}
				if (!errors.Is(err, context.DeadlineExceeded) && status.Code(err) != codes.DeadlineExceeded) || result.GetConfirmationRequired() || stop.stop != 0 || stop.prepares.Load() != 0 || force.Load() != 0 {
					t.Fatalf("pre-auth expiration granted authority: %v %v stops=%d prepares=%d force=%d", result, err, stop.stop, stop.prepares.Load(), force.Load())
				}
				l.recoveryOffersMu.Lock()
				n := len(l.recoveryOffers)
				l.recoveryOffersMu.Unlock()
				if n != 0 {
					t.Fatal("timeout published consent")
				}
			})
		}
	}

}
