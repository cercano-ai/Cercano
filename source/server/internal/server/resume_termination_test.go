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
		name   string
		err    error
		offer  bool
		legacy bool
	}{
		{"retained", broker.ErrAdmissionBusy, true, false},
		{"timeout", status.Error(codes.DeadlineExceeded, "source deadline"), true, false},
		{"auth", status.Error(codes.Unauthenticated, "bad token"), false, false},
		{"identity", status.Error(codes.PermissionDenied, "identity mismatch"), false, false},
		{"other_precondition", status.Error(codes.FailedPrecondition, "wrong identity"), false, false},
		{"legacy_retained", broker.ErrAdmissionBusy, true, true},
		{"legacy_timeout", status.Error(codes.DeadlineExceeded, "source deadline"), true, true},
		{"legacy_auth", status.Error(codes.Unauthenticated, "bad token"), false, true},
		{"legacy_permission", status.Error(codes.PermissionDenied, "identity mismatch"), false, true},
		{"legacy_near_busy", status.Error(codes.Unknown, broker.ErrAdmissionBusy.Error()+" extra"), false, true},
		{"legacy_busy_wrong_code", status.Error(codes.FailedPrecondition, broker.ErrAdmissionBusy.Error()), false, true},
		{"legacy_identity", status.Error(codes.FailedPrecondition, "wrong identity"), false, true},
		{"legacy_unresolved_commit", status.Error(codes.FailedPrecondition, "unresolved persistence outcomes remain"), false, true},
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
			if tc.legacy {
				source.legacy = true
				source.forceError = nil
				source.legacyError = tc.err
			}
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
