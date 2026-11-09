package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type originalRecoveryConsent struct {
	offer     *pb.LifecycleOriginalRecovery
	requester *pb.LifecycleIdentity
	expires   time.Time
}

// Journals select candidates, not authority. Even a perfectly formed hint cannot
// grant acceptance, prove source reservation, or supply a route or credential.
func originalRecoveryCandidate(ctx context.Context, l *coexistenceLifecycle, id string) (*pb.LifecycleControlRequest, string, error) {
	dir := l.options.StorePath + ".resume-operations"
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() || info.Mode()&0077 != 0 {
		return nil, "", status.Error(codes.FailedPrecondition, "private recovery journal directory required")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, "", err
	}
	var candidate *pb.LifecycleControlRequest
	var candidatePath string
	for _, path := range files {
		info, e := os.Lstat(path)
		if e != nil {
			return nil, "", e
		}
		if !info.Mode().IsRegular() || info.Mode()&0077 != 0 {
			return nil, "", status.Error(codes.FailedPrecondition, "unsafe recovery hint")
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return nil, "", e
		}
		op := new(pb.LifecycleControlRequest)
		if e = json.Unmarshal(b, op); e != nil {
			return nil, "", e
		}
		if op.GetOwner().GetConversation() != id {
			continue
		}
		if op.Source == nil || op.Destination == nil || op.Owner.Host != op.Source.Host || op.Owner.Epoch == "" || op.HandoverId == "" || op.Receipt == "" || !op.Management || !proto.Equal(op.Target, op.Source) || op.Source.Host == op.Destination.Host || op.Source.Store != l.options.StorePath || op.Destination.Store != l.options.StorePath {
			return nil, "", status.Error(codes.FailedPrecondition, "invalid original-operation hint")
		}
		// A durable completed handover is historical, not an incomplete operation.
		if _, e = conversation.HandoverResult(ctx, l.store, id, op.Destination.Host, op.HandoverId); e == nil {
			continue
		}
		if candidate != nil {
			return nil, "", status.Error(codes.FailedPrecondition, "ambiguous incomplete recovery operations")
		}
		candidate = op
		candidatePath = path
	}
	return candidate, candidatePath, nil
}

func recoveryOwnerMatches(ctx context.Context, l *coexistenceLifecycle, op *pb.LifecycleControlRequest) error {
	v, e := conversation.ReadOwnership(ctx, l.store, op.Owner.Conversation)
	if e != nil {
		return e
	}
	if v.Owner != op.Owner.Host || v.Epoch != op.Owner.Epoch || (v.State != "owned" && v.State != "reserved") || (v.State == "reserved" && (v.Destination != op.Destination.Host || v.Handover != op.HandoverId)) {
		return status.Error(codes.FailedPrecondition, "original recovery source/epoch/operation changed")
	}
	return nil
}

func recoveryPeer(ctx context.Context, l *coexistenceLifecycle, want *pb.LifecycleIdentity) (pb.AgentClient, *grpc.ClientConn, string, error) {
	if l.options.ResolveSource == nil || l.options.ControlDial == nil {
		return nil, nil, "", status.Error(codes.FailedPrecondition, "trusted recovery resolver unavailable")
	}
	got, endpoint, e := l.options.ResolveSource(ctx, want.GetHost())
	if e != nil {
		return nil, nil, "", e
	}
	if !proto.Equal(got, want) || want.GetBuild() == "" || want.GetContract() != l.options.Contract.Contract || want.GetSchema() != int32(l.options.Contract.SchemaVersion) || !controlLoopback(endpoint) {
		return nil, nil, "", status.Error(codes.FailedPrecondition, "original recovery peer changed")
	}
	conn, e := l.options.ControlDial(ctx, endpoint, want)
	if e != nil {
		return nil, nil, "", e
	}
	client := pb.NewAgentClient(conn)
	check, e := client.LifecycleList(ctx, &pb.LifecycleControlRequest{Target: want})
	if e != nil || !proto.Equal(check.GetIdentity(), want) {
		conn.Close()
		if e == nil {
			e = status.Error(codes.FailedPrecondition, "authenticated original peer mismatch")
		}
		return nil, nil, "", e
	}
	return client, conn, endpoint, nil
}

func (s *Server) offerOriginalRecovery(ctx context.Context, l *coexistenceLifecycle, r, op *pb.LifecycleControlRequest, forceLegacy bool) (*pb.LifecycleControlResponse, error) {
	if err := recoveryOwnerMatches(ctx, l, op); err != nil {
		return nil, err
	}
	source, sc, _, err := recoveryPeer(ctx, l, op.Source)
	if err != nil {
		return nil, err
	}
	defer sc.Close()
	_ = source
	dest, dc, endpoint, err := recoveryPeer(ctx, l, op.Destination)
	if err != nil {
		return nil, err
	}
	defer dc.Close()
	target := proto.Clone(op).(*pb.LifecycleControlRequest)
	target.Target = op.Destination
	target.DestinationEndpoint = endpoint
	// This older read-only RPC is required even for legacy consent: do not
	// restore an acceptance from a journal or create one using Prepare.
	proof, err := dest.LifecycleValidateReceipt(ctx, target)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(proof.GetIdentity(), op.Destination) || proof.GetReceipt() != op.Receipt {
		return nil, status.Error(codes.FailedPrecondition, "original acceptance not authenticated")
	}
	retry, err := dest.LifecycleRetryReceipt(ctx, target)
	legacy := forceLegacy || status.Code(err) == codes.Unimplemented
	if err != nil && status.Code(err) != codes.Unimplemented {
		return nil, err
	}
	if err == nil && (!proto.Equal(retry.GetIdentity(), op.Destination) || retry.GetReceipt() != op.Receipt) {
		return nil, status.Error(codes.FailedPrecondition, "original retry receipt mismatch")
	}
	if l.options.RecoverySelection == nil {
		return nil, status.Error(codes.FailedPrecondition, "trusted original selection unavailable")
	}
	selection, err := l.options.RecoverySelection(ctx, op.Destination.Host)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(selection.GetIdentity(), op.Destination) || selection.GetEndpoint() != endpoint || selection.GetTokenFile() == "" {
		return nil, status.Error(codes.FailedPrecondition, "trusted original selection mismatch")
	}
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		return nil, err
	}
	offer := &pb.LifecycleOriginalRecovery{Operation: proto.Clone(op).(*pb.LifecycleControlRequest), Selection: selection, Challenge: hex.EncodeToString(token[:]), Legacy: legacy}
	offer.Operation.DestinationEndpoint = endpoint
	if l.recoveryOffers == nil {
		l.recoveryOffers = make(map[string]*originalRecoveryConsent)
	}
	// Only the latest offer for this conversation/requester survives. Bounded,
	// short-lived consent is deliberately not durable or reusable after restart.
	for key, c := range l.recoveryOffers {
		if time.Now().After(c.expires) || (proto.Equal(c.requester, r.Requester) && c.offer.Operation.Owner.Conversation == op.Owner.Conversation) {
			delete(l.recoveryOffers, key)
		}
	}
	if len(l.recoveryOffers) >= 128 {
		return nil, status.Error(codes.ResourceExhausted, "too many outstanding recovery confirmations")
	}
	l.recoveryOffers[offer.Challenge] = &originalRecoveryConsent{offer: proto.Clone(offer).(*pb.LifecycleOriginalRecovery), requester: r.Requester, expires: time.Now().Add(5 * time.Minute)}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: op.Owner, OperationId: op.HandoverId, ConfirmationRequired: true, OriginalRecovery: offer}, nil
}

// Called with resumeMu held, before acquire, guarded recovery, or minting an op.
func (s *Server) originalRecovery(ctx context.Context, l *coexistenceLifecycle, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, bool, error) {
	if r.RecoveryChallenge != "" || r.ConfirmOriginalRecovery || r.ConfirmLegacyRecovery {
		c := l.recoveryOffers[r.RecoveryChallenge]
		delete(l.recoveryOffers, r.RecoveryChallenge) // consume even a failed confirmation
		if c == nil || c.offer.Takeover || time.Now().After(c.expires) || !proto.Equal(c.requester, r.Requester) || c.offer.Operation.Owner.Conversation != r.GetOwner().GetConversation() || !r.ConfirmOriginalRecovery || r.ConfirmLegacyRecovery != c.offer.Legacy {
			return nil, true, status.Error(codes.PermissionDenied, "fresh exact original-recovery confirmation required")
		}
		result, err := s.confirmOriginalRecovery(ctx, l, r, c.offer)
		return result, true, err
	}
	op, _, err := originalRecoveryCandidate(ctx, l, r.GetOwner().GetConversation())
	if err != nil {
		return nil, true, err
	}
	if op == nil || proto.Equal(op.Destination, l.controlIdentity()) {
		return nil, false, nil
	}
	result, err := s.offerOriginalRecovery(ctx, l, r, op, false)
	return result, true, err
}

func (s *Server) confirmOriginalRecovery(ctx context.Context, l *coexistenceLifecycle, r *pb.LifecycleControlRequest, offer *pb.LifecycleOriginalRecovery) (*pb.LifecycleControlResponse, error) {
	op, path, err := originalRecoveryCandidate(ctx, l, r.Owner.Conversation)
	if err != nil {
		return nil, err
	}
	expected := proto.Clone(offer.Operation).(*pb.LifecycleControlRequest)
	if op == nil {
		return nil, status.Error(codes.FailedPrecondition, "original operation no longer pending")
	}
	original := proto.Clone(op).(*pb.LifecycleControlRequest)
	op.DestinationEndpoint = expected.DestinationEndpoint
	if !proto.Equal(op, expected) {
		return nil, status.Error(codes.FailedPrecondition, "original operation changed after confirmation")
	}
	if err = recoveryOwnerMatches(ctx, l, op); err != nil {
		return nil, err
	}
	source, sc, _, err := recoveryPeer(ctx, l, op.Source)
	if err != nil {
		return nil, err
	}
	defer sc.Close()
	dest, dc, endpoint, err := recoveryPeer(ctx, l, op.Destination)
	if err != nil {
		return nil, err
	}
	defer dc.Close()
	if endpoint != offer.Selection.Endpoint {
		return nil, status.Error(codes.FailedPrecondition, "original destination route changed; confirm again")
	}
	if l.options.RecoverySelection == nil {
		return nil, status.Error(codes.FailedPrecondition, "trusted original selection unavailable")
	}
	selection, err := l.options.RecoverySelection(ctx, op.Destination.Host)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(selection, offer.Selection) {
		return nil, status.Error(codes.FailedPrecondition, "original selection changed; confirm again")
	}
	target := proto.Clone(op).(*pb.LifecycleControlRequest)
	target.Target = op.Destination
	proof, err := dest.LifecycleValidateReceipt(ctx, target)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(proof.GetIdentity(), op.Destination) || proof.GetReceipt() != op.Receipt {
		return nil, status.Error(codes.FailedPrecondition, "original acceptance changed")
	}
	// Reaffirm the EXACT original publication, not a coordinator-local copy or
	// the route-adjusted request. Never overwrite a concurrently changed hint.
	if err = reaffirmResumeWithSync(path, original, l.resumeJournalSync); err != nil {
		return nil, err
	}
	if !offer.Legacy {
		proof, err = dest.LifecycleRetryReceipt(ctx, target)
		if status.Code(err) == codes.Unimplemented {
			return s.offerOriginalRecovery(ctx, l, r, op, true)
		}
		if err != nil {
			return nil, err
		}
		if !proto.Equal(proof.GetIdentity(), op.Destination) || proof.GetReceipt() != op.Receipt {
			return nil, status.Error(codes.FailedPrecondition, "original retry receipt changed")
		}
		proof, err = source.LifecycleRetryRelinquish(ctx, op)
		if status.Code(err) == codes.Unimplemented {
			return s.offerOriginalRecovery(ctx, l, r, op, true)
		}
	} else {
		// Explicit separate legacy consent may initiate the ORIGINAL closure. This
		// is not a strict existing-reservation-only retry, nor permission to Prepare.
		proof, err = source.LifecycleForceRelinquish(ctx, op)
		if status.Code(err) == codes.Unimplemented {
			proof, err = source.LifecycleRelinquish(ctx, op)
		}
	}
	if err != nil {
		return nil, err
	}
	if !proto.Equal(proof.GetIdentity(), op.Source) || proof.GetReceipt() != op.Receipt {
		return nil, status.Error(codes.FailedPrecondition, "source result mismatch; recovery outcome uncertain")
	}
	var claimed *pb.LifecycleControlResponse
	if offer.Legacy {
		claimed, err = dest.LifecycleClaim(ctx, target)
	} else {
		claimed, err = dest.LifecycleRetryClaim(ctx, target)
	}
	if err != nil {
		return nil, err
	}
	if !proto.Equal(claimed.GetIdentity(), op.Destination) || claimed.GetOwner().GetConversation() != op.Owner.Conversation || claimed.GetOwner().GetHost() != op.Destination.Host || claimed.GetOwner().GetEpoch() == "" {
		return nil, status.Error(codes.FailedPrecondition, "original claim result mismatch; outcome uncertain")
	}
	if !offer.Legacy && !claimed.GetCurrentOwner() {
		return nil, status.Error(codes.FailedPrecondition, "original claim is historical, not current ownership")
	}
	// Check the durable exact handover too, not only a peer's claimed owner.
	epoch, err := conversation.HandoverResult(ctx, l.store, op.Owner.Conversation, op.Destination.Host, op.HandoverId)
	if err != nil || epoch != claimed.Owner.Epoch {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("original claim durability unproven: %v", err))
	}
	// Legacy Claim has no CurrentOwner bit and may also return a historical
	// handover. Revalidate the current durable owner/state/epoch for both paths.
	// This is a snapshot, not a lease: subsequent work still needs admission.
	current, err := conversation.ReadOwnership(ctx, l.store, op.Owner.Conversation)
	if err != nil || current.State != "owned" || current.Owner != op.Destination.Host || current.Epoch != epoch {
		return nil, status.Error(codes.FailedPrecondition, "original destination no longer owns the claimed epoch")
	}
	claimed.Ready = true
	claimed.OperationId = op.HandoverId
	claimed.OriginalRecovery = &pb.LifecycleOriginalRecovery{Operation: op, Selection: offer.Selection, Legacy: offer.Legacy}
	return claimed, nil
}
