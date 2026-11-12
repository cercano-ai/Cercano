package server

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestResumeTerminationOfferAfterSourceRefusal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		offer bool
	}{
		{"retained", broker.ErrAdmissionBusy, true},
		{"timeout", status.Error(codes.DeadlineExceeded, "source deadline"), true},
		{"auth", status.Error(codes.Unauthenticated, "bad token"), false},
		{"identity", status.Error(codes.PermissionDenied, "identity mismatch"), false},
		{"other_precondition", status.Error(codes.FailedPrecondition, "wrong identity"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			a, b, _ := lifecycleFixture(t)
			a.coexistence.options.ControlAuthorize = func(context.Context) error { return nil }
			b.coexistence.options.ControlAuthorize = func(context.Context) error { return nil }
			if _, err := a.Retain(ctx, "chat"); err != nil {
				t.Fatal(err)
			}
			source := &resumeSource{Server: a, forceError: tc.err, forceBlock: tc.name == "timeout"}
			endpoint := serveResumeSource(t, source)
			setResumeSource(b, a, endpoint)
			bc, destination := serveCutoverFixture(t, b)
			p := &containedFixture{}
			b.coexistence.options.RecoverySource = p
			b.coexistence.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
				return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: destination, TokenFile: "fixture"}, nil
			}
			r := resumeRequest(b, destination)
			r.Requester = b.coexistence.controlIdentity()
			before, _ := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			got, err := bc.LifecycleResume(ctx, r)
			if tc.offer {
				if err != nil || !got.GetConfirmationRequired() {
					t.Fatalf("expected explicit termination offer: %v %v", got, err)
				}
			} else if err == nil || got.GetConfirmationRequired() {
				t.Fatalf("unsafe offer: %v %v", got, err)
			}
			after, _ := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			if before != after || p.stop != 0 {
				t.Fatal("offer/decline changed source")
			}
		})
	}
}
