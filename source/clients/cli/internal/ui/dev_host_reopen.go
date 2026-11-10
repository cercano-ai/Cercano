package ui

// Reopening is an explicit user action, never a side effect of publication or
// connection loss. The lifecycle service remains the authority for busy refusal.
import (
	"context"
	"fmt"
	"time"

	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	protobuf "google.golang.org/protobuf/proto"
)

func reopenDevHost(ctx context.Context, destination DevHostSelection, conversation string) (DevHostSelection, error) {
	return reopenDevHostFrom(ctx, destination, conversation, destination.Identity)
}

func reopenDevHostFrom(ctx context.Context, destination DevHostSelection, conversation string, requester *proto.LifecycleIdentity) (DevHostSelection, error) {
	if err := destination.AcceptLaunchDelivery(ctx); err != nil {
		return destination, err
	}
	dc, err := destination.controls(ctx)
	if err != nil {
		return destination, err
	}
	defer dc.Close()
	// The selected coordinator may authenticate an exact original destination.
	// The CLI never discovers hosts or treats journal endpoints as authority.
	result, err := dc.LifecycleResume(ctx, &proto.LifecycleControlRequest{Target: destination.Identity, Owner: &proto.LifecycleOwner{Conversation: conversation}, DestinationEndpoint: destination.Endpoint, Requester: requester})
	if err != nil {
		return destination, err
	}
	if !protobuf.Equal(result.GetIdentity(), destination.Identity) {
		return destination, fmt.Errorf("resume returned different destination")
	}
	if result.GetOriginalRecovery() != nil {
		if err := validateContainedRecoveryOffer(result.OriginalRecovery, conversation); err != nil {
			return destination, err
		}
		if !result.GetConfirmationRequired() {
			return destination, fmt.Errorf("original recovery requires fresh confirmation")
		}
		return destination, &forcedResumeOffer{destination: destination, original: result.OriginalRecovery, requester: requester}
	}
	if result.GetConfirmationRequired() {
		return destination, fmt.Errorf("legacy recovery offer is no longer supported; upgrade the destination server and reopen for fresh recovery confirmation; no stop authorized")
	}

	if !result.GetReady() || result.GetPending() || result.GetOwner().GetConversation() != conversation || result.GetOwner().GetHost() != destination.Identity.Host || result.GetOwner().GetEpoch() == "" {
		return destination, fmt.Errorf("reopen returned no ownership")
	}
	destination.LaunchOperation, destination.OperationFile = "", ""
	destination.Conversation, destination.Epoch = conversation, result.Owner.Epoch
	return destination, nil
}

func (m Model) beginDevHostReopen(conversation string) (Model, tea.Cmd) {
	if m.devHost.busy || m.devHost.detached {
		m.errMsg = "Reopen unavailable while disconnected or another reopen is running"
		return m, nil
	}
	// Each reopen queries the destination afresh. A failed confirmed attempt
	// invalidates its offer; it must never become approval for a later retry.
	m.devHost.busy = true
	selection := m.devHost.defaultSelection
	if selection.Identity == nil {
		selection = m.devHost.selection
	}
	origin, originID, generation := m.agent, m.convID, m.taskSubscriptionGeneration
	requester := m.devHost.selection.Identity
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, err := reopenDevHostFrom(ctx, selection, conversation, requester)
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
		if err := selection.AcceptLaunchDelivery(ctx); err != nil {
			r.err = err
			return r
		}
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
		selection.LaunchOperation, selection.OperationFile = "", ""
		selection.Conversation, selection.Epoch = conversation, owner.Epoch
		r.selection = &selection
		r.client, r.err = selection.Dial(ctx)
		return r
	}
}
