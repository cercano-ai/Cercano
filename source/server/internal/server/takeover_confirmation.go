package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"time"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func takeoverNonce() (string, error) {
	var b [32]byte
	_, e := rand.Read(b[:])
	return hex.EncodeToString(b[:]), e
}

// Called before journal discovery: journals cannot suppress a cooperative offer
// for the current durable remote owner. No journal or client hint grants source
// authority. Unsupported/offline sources leave the older resume paths intact;
// they may offer their own, separately confirmed recovery, never takeover.
func (s *Server) offerTakeover(ctx context.Context, l *coexistenceLifecycle, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, bool, error) {
	if r.RecoveryChallenge != "" || r.ConfirmOriginalRecovery || r.ConfirmLegacyRecovery || r.ConfirmLegacyStop {
		return nil, false, nil
	}
	state, e := conversation.ReadOwnership(ctx, l.store, r.GetOwner().GetConversation())
	if e != nil || state.Owner == "" || state.Owner == l.options.Host || (state.State != "owned" && state.State != "reserved") {
		return nil, false, nil
	}
	if l.options.ResolveSource == nil || l.options.RecoverySelection == nil || l.options.ControlDial == nil || r.Requester == nil {
		return nil, false, nil
	}
	src, _, e := l.options.ResolveSource(ctx, state.Owner)
	if e != nil {
		return nil, false, nil
	}
	source, sc, _, e := recoveryPeer(ctx, l, src)
	if e != nil {
		return nil, false, nil
	}
	defer sc.Close()
	owner := &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: state.Owner, Epoch: state.Epoch}
	proof, e := source.LifecycleTakeoverProbe(ctx, &pb.LifecycleControlRequest{Target: src, Owner: owner})
	if e != nil {
		return nil, true, e
	}
	if !proto.Equal(proof.GetIdentity(), src) {
		return nil, true, status.Error(codes.FailedPrecondition, "takeover source identity changed")
	}
	selection, e := l.options.RecoverySelection(ctx, l.options.Host)
	if e != nil {
		return nil, true, e
	}
	if !proto.Equal(selection.GetIdentity(), l.controlIdentity()) || selection.GetTokenFile() == "" || !controlLoopback(selection.GetEndpoint()) {
		return nil, true, status.Error(codes.FailedPrecondition, "trusted exact takeover destination unavailable")
	}
	token, e := takeoverNonce()
	if e != nil {
		return nil, true, e
	}
	operation, e := takeoverNonce()
	if e != nil {
		return nil, true, e
	}
	op := &pb.LifecycleControlRequest{Target: src, Source: src, Destination: l.controlIdentity(), DestinationEndpoint: selection.Endpoint, Owner: owner, Management: true, HandoverId: operation}
	offer := &pb.LifecycleOriginalRecovery{Operation: op, Selection: selection, Challenge: token, Takeover: true}
	if l.recoveryOffers == nil {
		l.recoveryOffers = make(map[string]*originalRecoveryConsent)
	}
	for key, c := range l.recoveryOffers {
		if time.Now().After(c.expires) || (proto.Equal(c.requester, r.Requester) && c.offer.Operation.Owner.Conversation == owner.Conversation) {
			delete(l.recoveryOffers, key)
		}
	}
	if len(l.recoveryOffers) >= 128 {
		return nil, true, status.Error(codes.ResourceExhausted, "too many takeover confirmations")
	}
	l.recoveryOffers[token] = &originalRecoveryConsent{offer: proto.Clone(offer).(*pb.LifecycleOriginalRecovery), requester: proto.Clone(r.Requester).(*pb.LifecycleIdentity), expires: time.Now().Add(5 * time.Minute)}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: owner, OperationId: operation, ConfirmationRequired: true, OriginalRecovery: offer}, true, nil
}

// LifecycleTakeover is confirmation-only. A challenge is single-use even when
// the request fails. New attempts need a fresh view/consent, never auto replay.
func (s *Server) LifecycleTakeover(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	l, e := s.control(ctx, r)
	if e != nil {
		return nil, e
	}
	l.resumeMu.Lock()
	defer l.resumeMu.Unlock()
	c := l.recoveryOffers[r.RecoveryChallenge]
	delete(l.recoveryOffers, r.RecoveryChallenge)
	if c == nil || !c.offer.Takeover || c.offer.Legacy || time.Now().After(c.expires) || !r.ConfirmOriginalRecovery || r.ConfirmLegacyRecovery || r.ConfirmLegacyStop || !proto.Equal(c.requester, r.Requester) || c.offer.Operation.Owner.Conversation != r.GetOwner().GetConversation() {
		return nil, status.Error(codes.PermissionDenied, "fresh exact takeover confirmation required")
	}
	offer := c.offer
	op := proto.Clone(offer.Operation).(*pb.LifecycleControlRequest)
	state, e := conversation.ReadOwnership(ctx, l.store, op.Owner.Conversation)
	if e != nil {
		return nil, e
	}
	if state.Owner != op.Owner.Host || state.Epoch != op.Owner.Epoch || (state.State != "owned" && state.State != "reserved") {
		return nil, status.Error(codes.FailedPrecondition, "takeover confirmation is stale; refresh ownership")
	}
	selection, e := l.options.RecoverySelection(ctx, l.options.Host)
	if e != nil {
		return nil, e
	}
	if !proto.Equal(selection, offer.Selection) {
		return nil, status.Error(codes.FailedPrecondition, "takeover destination changed; confirm again")
	}
	source, sc, _, e := recoveryPeer(ctx, l, op.Source)
	if e != nil {
		return nil, e
	}
	defer sc.Close()
	proof, e := source.LifecycleTakeoverProbe(ctx, &pb.LifecycleControlRequest{Target: op.Source, Owner: op.Owner})
	if e != nil {
		return nil, e
	}
	if !proto.Equal(proof.GetIdentity(), op.Source) {
		return nil, status.Error(codes.FailedPrecondition, "takeover source changed")
	}
	target := proto.Clone(op).(*pb.LifecycleControlRequest)
	target.Target = op.Destination
	proof, e = s.LifecycleTakeoverPrepare(ctx, target)
	if e != nil {
		return nil, e
	}
	if !proto.Equal(proof.GetIdentity(), op.Destination) || proof.GetReceipt() == "" {
		return nil, status.Error(codes.FailedPrecondition, "takeover acceptance mismatch")
	}
	op.Receipt = proof.Receipt
	target.Receipt = proof.Receipt
	// Separate journal namespace: conflicting old outgoing operations are not
	// authority, and must not be overwritten to make a new takeover possible.
	path := filepath.Join(l.options.StorePath+".takeover-operations", op.HandoverId+".json")
	if e = saveResume(path, op); e != nil {
		return nil, e
	}
	proof, e = source.LifecycleTakeoverRelinquish(ctx, op)
	if e != nil {
		return nil, e
	}
	if !proto.Equal(proof.GetIdentity(), op.Source) || proof.GetReceipt() != op.Receipt {
		return nil, status.Error(codes.FailedPrecondition, "takeover source response mismatch")
	}
	claimed, e := s.LifecycleTakeoverClaim(ctx, target)
	if e != nil {
		return nil, e
	}
	if !claimed.GetCurrentOwner() || claimed.GetOwner().GetHost() != op.Destination.Host || claimed.GetOwner().GetEpoch() == "" || claimed.GetOwner().GetEpoch() == op.Owner.Epoch {
		return nil, status.Error(codes.FailedPrecondition, "takeover did not produce current fresh ownership")
	}
	// ClaimIncoming reloads durable history before exposing a ready capability.
	// Preserve the offered operation exactly for UI challenge/result binding.
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: claimed.Owner, Ready: true, CurrentOwner: true, OperationId: op.HandoverId, OriginalRecovery: offer, BlockedReason: "Interrupted work is not replayed. External effects are not rolled back; history is reloaded."}, nil
}
