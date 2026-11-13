package server

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"testing"
	"time"
)

func TestResumePreAuthTimeoutMatrix(t *testing.T) {
	for _, phase := range []string{"resolve-new", "resolve-journal", "dial", "list", "list-child", "resolve-child", "recovery-resolve", "recovery-selection"} {
		t.Run(phase, func(t *testing.T) {
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
			l := b.coexistence
			stop := &containedFixture{}
			l.options.RecoverySource = stop
			l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
				return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: endpoint, TokenFile: "fixture"}, nil
			}
			calls := 0
			resolve := l.options.ResolveSource
			translated := func(ctx context.Context) error {
				<-ctx.Done()
				return status.Error(codes.Unavailable, "adapter translated deadline")
			}
			l.options.ResolveSource = func(ctx context.Context, host string) (*pb.LifecycleIdentity, string, error) {
				calls++
				if phase == "resolve-new" || phase == "resolve-child" || phase == "recovery-resolve" || (phase == "resolve-journal" && calls == 2) {
					return nil, "", translated(ctx)
				}
				return resolve(ctx, host)
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
			if err == nil || result.GetConfirmationRequired() || stop.stop != 0 {
				t.Fatalf("pre-auth expiration granted authority: %v %v stops=%d", result, err, stop.stop)
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
