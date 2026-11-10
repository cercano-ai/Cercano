package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// ContainedSourceStop is stronger than LegacyProcessStop: the adapter must qualify
// the historical writer contract and retain launch exclusion through use. A host
// absence check alone is not sufficient to implement this interface.
type ContainedSourceStop interface {
	WithStoppedSource(context.Context, *pb.LifecycleIdentity, string, func() error, func(context.Context) error) error
}

type heldRecoverySource struct{}

// liveSourceRecovery is the normal reopen coordinator. Journals and optional new
// source RPCs are deliberately not consulted as ownership authority.
func (s *Server) liveSourceRecovery(ctx context.Context, l *coexistenceLifecycle, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, bool, error) {
	id := r.GetOwner().GetConversation()
	state, err := conversation.ReadOwnership(ctx, l.store, id)
	if err != nil {
		return nil, true, err
	}
	// Local ready ownership is handled by ordinary acquisition, including its
	// completed-journal operation metadata. Never replace that result here.
	if state.Owner == l.options.Host && state.State == "owned" {
		return nil, false, nil
	}
	if state.Owner == "" && r.RecoveryChallenge == "" {
		return nil, false, nil
	}
	adapter, supported := l.options.LegacyStop.(ContainedSourceStop)
	if !supported {
		return nil, true, status.Error(codes.FailedPrecondition, "live-source recovery requires a qualified containment adapter")
	}
	if r.RecoveryChallenge != "" {
		c := l.recoveryOffers[r.RecoveryChallenge]
		delete(l.recoveryOffers, r.RecoveryChallenge)
		if c == nil || c.offer.Operation.GetLegacyProcessIdentity() == "" || time.Now().After(c.expires) || !r.ConfirmOriginalRecovery || !proto.Equal(c.requester, r.Requester) || c.offer.Operation.Owner.Conversation != id {
			return nil, true, status.Error(codes.PermissionDenied, "fresh single-use live-source confirmation required")
		}
		op := c.offer.Operation
		validate := func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			current, e := conversation.ReadOwnership(ctx, l.store, id)
			if e != nil {
				return e
			}
			encoded, _ := json.Marshal(current)
			if string(encoded) != c.liveState || current != state || current.Owner != op.Owner.Host || current.Epoch != op.Owner.Epoch || (current.State != "owned" && current.State != "reserved") {
				return status.Error(codes.FailedPrecondition, "live-source consent is stale")
			}
			selection, e := l.options.RecoverySelection(ctx, l.options.Host)
			if e != nil {
				return e
			}
			source, _, e := l.options.ResolveSource(ctx, op.Source.Host)
			if e != nil {
				return e
			}
			if !proto.Equal(selection, c.offer.Selection) || !proto.Equal(source, op.Source) {
				return status.Error(codes.FailedPrecondition, "recovery source or selected destination changed")
			}
			return nil
		}
		if err := validate(); err != nil {
			return nil, true, err
		}
		var result *pb.LifecycleControlResponse
		err = adapter.WithStoppedSource(ctx, op.Source, op.LegacyProcessIdentity, validate, func(held context.Context) error {
			if e := validate(); e != nil {
				return e
			}
			var e error
			result, e = s.LifecycleRecover(context.WithValue(held, heldRecoverySource{}, state), &pb.LifecycleRecoverRequest{Target: r.Target, Conversation: id, ExpectedOwner: op.Owner, Confirm: true})
			return e
		})
		if err != nil {
			return nil, true, err
		}
		if result == nil || !result.Ready || result.Owner.GetHost() != l.options.Host || result.Owner.GetConversation() != id || result.Owner.GetEpoch() == op.Owner.Epoch {
			return nil, true, status.Error(codes.FailedPrecondition, "recovery did not return exact current ownership")
		}
		result.CurrentOwner = true
		result.OperationId = op.HandoverId
		result.OriginalRecovery = c.offer
		return result, true, nil
	}
	if state.Owner == "" || (state.State != "owned" && state.State != "reserved") {
		return nil, false, nil
	}
	if r.Requester == nil || r.Requester.Host == state.Owner || l.options.ResolveSource == nil || l.options.RecoverySelection == nil {
		return nil, true, status.Error(codes.FailedPrecondition, "authenticated source and selected destination required")
	}
	src, _, err := l.options.ResolveSource(ctx, state.Owner)
	if err != nil {
		return nil, true, err
	}
	selection, err := l.options.RecoverySelection(ctx, l.options.Host)
	if err != nil {
		return nil, true, err
	}
	if !proto.Equal(selection.GetIdentity(), l.controlIdentity()) || !controlLoopback(selection.GetEndpoint()) || selection.GetTokenFile() == "" {
		return nil, true, status.Error(codes.FailedPrecondition, "exact destination unavailable")
	}
	if src.GetHost() != state.Owner || src.GetStore() != l.controlIdentity().GetStore() {
		return nil, true, status.Error(codes.FailedPrecondition, "resolved source is not the durable owner")
	}
	snapshot, err := l.options.LegacyStop.Prepare(ctx, src)
	if err != nil {
		return nil, true, err
	}
	if snapshot == "" {
		return nil, true, status.Error(codes.FailedPrecondition, "source process identity unavailable")
	}
	nonce, err := recoveryNonce()
	if err != nil {
		return nil, true, err
	}
	owner := &pb.LifecycleOwner{Conversation: id, Host: state.Owner, Epoch: state.Epoch}
	op := &pb.LifecycleControlRequest{Target: src, Source: src, Destination: l.controlIdentity(), Owner: owner, HandoverId: conversation.RecoveryID(id, state, l.options.Host), LegacyProcessIdentity: snapshot}
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, true, err
	}
	offer := &pb.LifecycleOriginalRecovery{Operation: op, Selection: selection, Challenge: nonce}
	if l.recoveryOffers == nil {
		l.recoveryOffers = make(map[string]*containedRecoveryConsent)
	}
	for key, c := range l.recoveryOffers {
		if time.Now().After(c.expires) || (proto.Equal(c.requester, r.Requester) && c.offer.Operation.Owner.Conversation == id) {
			delete(l.recoveryOffers, key)
		}
	}
	if len(l.recoveryOffers) >= 128 {
		return nil, true, status.Error(codes.ResourceExhausted, "too many recovery confirmations")
	}
	l.recoveryOffers[nonce] = &containedRecoveryConsent{liveState: string(encoded), offer: proto.Clone(offer).(*pb.LifecycleOriginalRecovery), requester: proto.Clone(r.Requester).(*pb.LifecycleIdentity), expires: time.Now().Add(5 * time.Minute)}
	return &pb.LifecycleControlResponse{Identity: l.controlIdentity(), Owner: owner, OperationId: op.HandoverId, ConfirmationRequired: true, OriginalRecovery: offer}, true, nil
}

// The historical protobuf envelope is reused only by contained recovery.
type containedRecoveryConsent struct {
	offer     *pb.LifecycleOriginalRecovery
	requester *pb.LifecycleIdentity
	expires   time.Time
	liveState string
}

func recoveryNonce() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
