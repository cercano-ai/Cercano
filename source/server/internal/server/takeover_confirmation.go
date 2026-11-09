package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
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

// Called before older resume journal discovery. Takeover journals identify exact
// retry candidates, but no journal or client hint grants source authority.
// Unsupported/offline source discovery leaves the older resume paths intact;
// they may offer their own, separately confirmed recovery, never takeover.
func (s *Server) offerTakeover(ctx context.Context, l *coexistenceLifecycle, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, bool, error) {
	if r.RecoveryChallenge != "" || r.ConfirmOriginalRecovery || r.ConfirmLegacyRecovery || r.ConfirmLegacyStop {
		return nil, false, nil
	}
	state, e := conversation.ReadOwnership(ctx, l.store, r.GetOwner().GetConversation())
	if e != nil || state.Owner == "" || (state.State != "owned" && state.State != "reserved") {
		return nil, false, nil
	}
	if l.options.ResolveSource == nil || l.options.RecoverySelection == nil || l.options.ControlDial == nil || r.Requester == nil {
		return nil, false, nil
	}
	// Journals are discovery hints only. Authenticate both retained acceptances
	// and check durable ownership before offering an exact retry.
	var pending *pb.LifecycleControlRequest
	paths, _ := filepath.Glob(filepath.Join(l.options.StorePath+".takeover-operations", "*.json"))
	for _, path := range paths {
		b, err := os.ReadFile(path)
		hint := new(pb.LifecycleControlRequest)
		if err != nil || json.Unmarshal(b, hint) != nil || hint.GetOwner().GetConversation() != r.Owner.Conversation || !proto.Equal(hint.Destination, l.controlIdentity()) {
			continue
		}
		if !takeoverStateMatches(state.Owner, state.Epoch, state.State, state.Handover, hint) {
			continue
		}
		if state.Owner == l.options.Host && state.State == "owned" {
			epoch, err := conversation.HandoverResult(ctx, l.store, hint.Owner.Conversation, hint.Destination.Host, hint.HandoverId)
			if err != nil || epoch != state.Epoch {
				continue
			}
		}
		peer, conn, _, err := recoveryPeer(ctx, l, hint.Source)
		if err != nil {
			return nil, true, err
		}
		proof, err := peer.LifecycleTakeoverProbe(ctx, hint)
		conn.Close()
		if err != nil {
			return nil, true, err
		}
		key := proto.Clone(hint).(*pb.LifecycleControlRequest)
		key.DestinationEndpoint = ""
		if !proto.Equal(proof.GetIdentity(), hint.Source) || !proto.Equal(proof.GetOriginalRecovery().GetOperation(), key) {
			return nil, true, status.Error(codes.FailedPrecondition, "source did not retain exact takeover")
		}
		target := proto.Clone(hint).(*pb.LifecycleControlRequest)
		target.Target = hint.Destination
		if _, err = s.LifecycleTakeoverReceipt(ctx, target); err != nil {
			return nil, true, err
		}
		pending = hint
		break
	}
	if pending == nil && state.Owner == l.options.Host {
		return nil, false, nil
	}
	sourceHost := state.Owner
	if pending != nil {
		sourceHost = pending.Owner.Host
	}
	src, _, e := l.options.ResolveSource(ctx, sourceHost)
	if e != nil {
		return nil, false, nil
	}
	source, sc, _, e := recoveryPeer(ctx, l, src)
	if e != nil {
		return nil, false, nil
	}
	defer sc.Close()
	owner := &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: state.Owner, Epoch: state.Epoch}
	if pending != nil {
		owner = pending.Owner
	}
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
	if pending != nil {
		if pending.DestinationEndpoint != selection.Endpoint {
			return nil, true, status.Error(codes.FailedPrecondition, "exact takeover endpoint changed")
		}
		op = pending
		operation = op.HandoverId
	} else if retained := proof.GetOriginalRecovery().GetOperation(); retained != nil {
		return nil, true, status.Error(codes.FailedPrecondition, "retained takeover requires its durable exact journal and destination acceptance")
	}
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
	if !takeoverStateMatches(state.Owner, state.Epoch, state.State, state.Handover, op) {
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
	probe := &pb.LifecycleControlRequest{Target: op.Source, Owner: op.Owner}
	if op.Receipt != "" {
		probe = op
	}
	proof, e := source.LifecycleTakeoverProbe(ctx, probe)
	if e != nil {
		return nil, e
	}
	if !proto.Equal(proof.GetIdentity(), op.Source) {
		return nil, status.Error(codes.FailedPrecondition, "takeover source changed")
	}
	if op.Receipt != "" {
		key := proto.Clone(op).(*pb.LifecycleControlRequest)
		key.DestinationEndpoint = ""
		if !proto.Equal(proof.GetOriginalRecovery().GetOperation(), key) {
			return nil, status.Error(codes.FailedPrecondition, "exact source acceptance changed")
		}
	}
	target := proto.Clone(op).(*pb.LifecycleControlRequest)
	target.Target = op.Destination
	if op.Receipt != "" {
		proof, e = s.LifecycleTakeoverReceipt(ctx, target)
	} else {
		proof, e = s.LifecycleTakeoverPrepare(ctx, target)
	}
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
	if _, err := os.Stat(path); err == nil {
		e = reaffirmResumeWithSync(path, op, nil)
	} else if os.IsNotExist(err) {
		e = saveResume(path, op)
	} else {
		e = err
	}
	if e != nil {
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

// A retained receipt permits retry, not replacement or obsolete replay.
func takeoverStateMatches(host, epoch, state, handover string, op *pb.LifecycleControlRequest) bool {
	if host == op.GetOwner().GetHost() && epoch == op.GetOwner().GetEpoch() && (state == "owned" || state == "reserved") {
		return true
	}
	return op.Receipt != "" && host == op.GetDestination().GetHost() && ((state == "reserved" && handover == op.HandoverId) || (state == "owned" && epoch != op.GetOwner().GetEpoch()))
}
