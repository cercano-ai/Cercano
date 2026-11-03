package ui

// Reopening is an explicit user action, never a side effect of publication or
// connection loss. The lifecycle service remains the authority for busy refusal.
import (
	"context"
	"fmt"
	"time"

	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	protobuf "google.golang.org/protobuf/proto"
)

func reopenDevHost(ctx context.Context, destination DevHostSelection, conversation string) (DevHostSelection, error) {
	dc, err := destination.controls(ctx)
	if err != nil {
		return destination, err
	}
	defer dc.Close()
	// Only the selected destination is authoritative. Never discover or dial a
	// historical owner: endpoint availability is neither ownership nor death proof.
	result, acquireErr := dc.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: destination.Identity, Owner: &proto.LifecycleOwner{Conversation: conversation}})
	if acquireErr != nil {
		if status.Code(acquireErr) == codes.Unauthenticated || status.Code(acquireErr) == codes.PermissionDenied {
			return destination, acquireErr
		}
		probe, recoveryErr := dc.LifecycleRecover(ctx, &proto.LifecycleRecoverRequest{Target: destination.Identity, Conversation: conversation})
		if recoveryErr != nil {
			return destination, fmt.Errorf("destination acquisition refused (%v); destination recovery blocked: %w", acquireErr, recoveryErr)
		}
		if !protobuf.Equal(probe.GetIdentity(), destination.Identity) || probe.GetOwner().GetConversation() != conversation || probe.GetOwner().GetHost() == "" || probe.GetOwner().GetHost() == destination.Identity.Host || probe.GetOwner().GetEpoch() == "" {
			return destination, fmt.Errorf("destination recovery inspection returned invalid ownership")
		}
		return destination, &forcedResumeOffer{destination: destination, owner: probe.Owner, dead: true}
	}
	if result.GetPending() || !protobuf.Equal(result.GetIdentity(), destination.Identity) || result.GetOwner().GetConversation() != conversation || result.GetOwner().GetHost() != destination.Identity.Host || result.GetOwner().GetEpoch() == "" {
		return destination, fmt.Errorf("reopen returned no ownership")
	}
	destination.Conversation, destination.Epoch = conversation, result.Owner.Epoch
	return destination, nil
}

func (m Model) beginDevHostReopen(conversation string) (Model, tea.Cmd) {
	if m.devHost.busy || m.devHost.detached {
		m.errMsg = "Reopen unavailable while disconnected or another reopen is running"
		return m, nil
	}
	if pending := m.devHost.pendingCutover; pending != nil && pending.owner.Conversation == conversation {
		next, cmd := m.offerForcedResume(pending)
		return next.(Model), cmd
	}
	m.devHost.busy = true
	selection := m.devHost.defaultSelection
	if selection.Identity == nil {
		selection = m.devHost.selection
	}
	origin, originID, generation := m.agent, m.convID, m.taskSubscriptionGeneration
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, err := reopenDevHost(ctx, selection, conversation)
		r := devHostResult{action: "reopen", err: err, fenced: true, origin: origin, originID: originID, generation: generation}
		if err == nil {
			r.selection = &s
			r.client, r.err = s.Dial(ctx)
		}
		return r
	}
}

// Fresh IDs need no discovery or transfer: acquire only on the exact current host.
func (m Model) beginDevHostFreshConversation() (Model, tea.Cmd) {
	if m.devHost.busy || m.devHost.detached {
		m.errMsg = "Clear unavailable while disconnected or another lifecycle operation is running"
		return m, nil
	}
	m.devHost.busy = true
	selection := m.devHost.defaultSelection
	if selection.Identity == nil {
		selection = m.devHost.selection
	}
	conversation := newConvID()
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		r := devHostResult{action: "reopen"}
		c, err := selection.controls(ctx)
		if err != nil {
			r.err = err
			return r
		}
		defer c.Close()
		response, err := c.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: selection.Identity, Owner: &proto.LifecycleOwner{Conversation: conversation}})
		if err != nil {
			r.err = err
			return r
		}
		owner := response.GetOwner()
		if response.GetPending() || owner.GetConversation() != conversation || owner.GetHost() != selection.Identity.Host || owner.GetEpoch() == "" {
			r.err = fmt.Errorf("fresh acquisition returned invalid ownership")
			return r
		}
		selection.Conversation, selection.Epoch = conversation, owner.Epoch
		r.selection = &selection
		r.client, r.err = selection.Dial(ctx)
		return r
	}
}
