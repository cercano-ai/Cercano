package server

import (
	"context"
	"encoding/json"
	"os"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// LegacyProcessStop is a trusted, injected platform boundary, not signal authority
// derived from argv alone. Prepare must prove UID, PID/start identity, executable
// and exact host attribution and return a stable opaque snapshot. Stop must hold
// the source launch exclusion, revalidate that complete snapshot, call validate
// immediately before any signal, verify death, and fail closed on ambiguity or
// unsupported platforms. RecoveryGuard independently proves absence afterward.
// There is deliberately no default implementation using kill(pid) or pkill.
type LegacyProcessStop interface {
	Prepare(context.Context, *pb.LifecycleIdentity) (string, error)
	Stop(context.Context, *pb.LifecycleIdentity, string, func() error) error
}

func validateLegacyApproval(l *coexistenceLifecycle, r *pb.LifecycleControlRequest) (*pb.LifecycleControlRequest, error) {
	b, err := os.ReadFile(resumeJournal(l, r.GetOwner().GetConversation()))
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, "legacy approval has no durable operation")
	}
	op := new(pb.LifecycleControlRequest)
	if json.Unmarshal(b, op) != nil {
		return nil, status.Error(codes.FailedPrecondition, "invalid durable legacy operation")
	}
	// An offer without a verified process snapshot grants no stop authority.
	// This is the normal production refusal until a safe platform adapter exists.
	if op.LegacyProcessIdentity == "" {
		return nil, status.Error(codes.FailedPrecondition, "safe legacy process stop unavailable; no signal sent")
	}
	if !proto.Equal(op.Destination, l.controlIdentity()) || !proto.Equal(r.Target, op.Destination) || !proto.Equal(r.Source, op.Source) || !proto.Equal(r.Owner, op.Owner) || r.HandoverId == "" || r.HandoverId != op.HandoverId || op.LegacyProcessIdentity == "" || r.LegacyProcessIdentity != op.LegacyProcessIdentity {
		return nil, status.Error(codes.FailedPrecondition, "stale legacy process/source/owner/destination/operation approval")
	}
	if r.Requester.GetHost() == "" || !proto.Equal(r.Requester, op.Requester) || r.Requester.Host == op.Source.Host || op.Source.Host == l.options.Host {
		return nil, status.Error(codes.FailedPrecondition, "refusing stop of requester or destination host")
	}
	return op, nil
}

func (s *Server) resumeConfirmedLegacy(ctx context.Context, r, op *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l := s.coexistence
	validate := func() error {
		if _, err := validateLegacyApproval(l, r); err != nil {
			return err
		}
		v, err := conversation.ReadOwnership(ctx, l.store, op.Owner.Conversation)
		if err != nil {
			return err
		}
		if v.State != "owned" || v.Owner != op.Owner.Host || v.Epoch != op.Owner.Epoch {
			return status.Error(codes.FailedPrecondition, "legacy owner changed before stop")
		}
		if l.options.ResolveSource == nil {
			return status.Error(codes.FailedPrecondition, "source resolver unavailable")
		}
		source, _, err := l.options.ResolveSource(ctx, op.Owner.Host)
		if err != nil {
			return err
		}
		if !proto.Equal(source, op.Source) {
			return status.Error(codes.FailedPrecondition, "source attribution changed before stop")
		}
		return nil
	}
	if err := validate(); err != nil {
		return nil, err
	}
	if l.options.LegacyStop == nil || l.options.RecoveryGuard == nil {
		return nil, status.Error(codes.FailedPrecondition, "safe legacy process stop unavailable; no signal sent")
	}
	if err := l.options.LegacyStop.Stop(ctx, op.Source, op.LegacyProcessIdentity, validate); err != nil {
		return nil, err
	}
	// Do not equate a successful stop call with death. The existing guard must
	// verify absence under launch exclusion before exact-epoch recovery.
	return s.LifecycleRecover(ctx, &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: op.Owner.Conversation, ExpectedOwner: op.Owner, Confirm: true})
}
