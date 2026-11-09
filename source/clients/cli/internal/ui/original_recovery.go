package ui

import (
	"context"
	"fmt"
	"net"
	"time"

	pb "cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/protobuf/proto"
)

func validateOriginalOffer(o *pb.LifecycleOriginalRecovery, conversation string) error {
	op := o.GetOperation()
	sel := o.GetSelection()
	id := sel.GetIdentity()
	host, _, err := net.SplitHostPort(sel.GetEndpoint())
	ip := net.ParseIP(host)
	if (o.GetTakeover() && (o.GetLegacy() || op.GetSource().GetStore() != id.GetStore() || op.GetSource().GetContract() != id.GetContract() || op.GetSource().GetSchema() != id.GetSchema() || op.GetSource().GetBuild() == "")) || err != nil || ip == nil || !ip.IsLoopback() || o.GetChallenge() == "" || sel.GetTokenFile() == "" || id.GetHost() == "" || id.GetBuild() == "" || id.GetStore() == "" || id.GetContract() == "" || id.GetSchema() <= 0 || !proto.Equal(id, op.GetDestination()) || !proto.Equal(op.GetTarget(), op.GetSource()) || op.GetOwner().GetConversation() != conversation || op.GetOwner().GetEpoch() == "" || op.GetOwner().GetHost() != op.GetSource().GetHost() || op.GetSource().GetHost() == "" || op.GetSource().GetHost() == id.GetHost() || (op.GetReceipt() == "" && !o.GetTakeover()) || op.GetHandoverId() == "" {
		return fmt.Errorf("invalid authenticated original-operation selection")
	}
	return nil
}

func (m Model) offerOriginalRecovery(offer *forcedResumeOffer) (tea.Model, tea.Cmd) {
	o := offer.original
	op := o.Operation
	client, parent, gen := m.agent, m.convID, m.taskSubscriptionGeneration
	decline := func(m Model) (Model, tea.Cmd) { m.pendingConfirm = nil; return m, nil }
	title := "Retry the exact original transfer?"
	mode := "Retry-only controls will require an existing source reservation. Unsupported handlers will require a new, separate legacy confirmation; no silent fallback."
	if o.Takeover {
		title = "Take over this conversation on the selected host?"
		mode = "This is a NEW cooperative takeover, not an original-operation retry or an attachment takeover. Interrupted work will not be replayed; external effects are NOT rolled back. Persisted history will be reloaded."
	} else if o.Legacy {
		title = "Recover the original operation using legacy controls?"
		mode = "LEGACY: ordinary original transfer controls may create the missing source reservation FOR THIS ORIGINAL OPERATION. This is not an existing-reservation-only retry. No new destination acceptance will be created."
	}
	selectionDescription := fmt.Sprintf("Conversation %s: source %s, epoch %s, operation %s → selected destination %s.", op.Owner.Conversation, op.Source.Host, op.Owner.Epoch, op.HandoverId, op.Destination.Host)
	if !o.Takeover {
		selectionDescription += " This retries the ORIGINAL destination, not the newly selected host."
	}
	hints := "y: recover original operation · n/esc/enter: cancel"
	if o.Takeover {
		hints = "y: confirm cooperative takeover · n/esc/enter: cancel"
	}
	m.enqueueConfirmation(&confirmRequest{title: title, details: []string{
		selectionDescription,
		mode, "Recovery can fence the source and cancel conversation work. Pending or uncertain persistence must resolve before completion. The server revalidates authenticated acceptance, identities and the exact operation. Consent applies only to this attempt."}, hints: hints, extras: map[string]func(Model) (Model, tea.Cmd){"enter": decline}, onNo: decline, onYes: func(m Model) (Model, tea.Cmd) {
		m.pendingConfirm = nil
		if m.agent != client || m.convID != parent || m.taskSubscriptionGeneration != gen || m.devHost == nil || m.devHost.busy {
			return m, nil
		}
		m.devHost.busy = true
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result := devHostResult{action: "reopen", fenced: true, origin: client, originID: parent, generation: gen}
			dc, err := offer.destination.controls(ctx)
			if err != nil {
				result.err = err
				return result
			}
			defer dc.Close()
			request := &pb.LifecycleControlRequest{Target: offer.destination.Identity, Owner: &pb.LifecycleOwner{Conversation: op.Owner.Conversation}, Requester: offer.requester, RecoveryChallenge: o.Challenge, ConfirmOriginalRecovery: true, ConfirmLegacyRecovery: o.Legacy}
			var response *pb.LifecycleControlResponse
			if o.Takeover {
				response, err = dc.LifecycleTakeover(ctx, request)
			} else {
				response, err = dc.LifecycleResume(ctx, request)
			}
			if err != nil {
				result.err = err
				return result
			}
			if response.GetConfirmationRequired() {
				if o.Takeover {
					result.err = fmt.Errorf("takeover cannot fall back to legacy recovery")
					return result
				}
				next := response.GetOriginalRecovery()
				if !proto.Equal(response.GetIdentity(), offer.destination.Identity) || validateOriginalOffer(next, op.Owner.Conversation) != nil || !proto.Equal(next.Operation, op) || !next.Legacy {
					result.err = fmt.Errorf("invalid separate legacy confirmation")
					return result
				}
				result.err = &forcedResumeOffer{destination: offer.destination, original: next, requester: offer.requester}
				return result
			}
			recovered := response.GetOriginalRecovery()
			owner := response.GetOwner()
			if (o.Takeover && (!response.GetCurrentOwner() || !recovered.GetTakeover())) || !response.GetReady() || response.GetPending() || !proto.Equal(response.GetIdentity(), op.Destination) || !proto.Equal(recovered.GetOperation(), op) || !proto.Equal(recovered.GetSelection(), o.Selection) || owner.GetConversation() != op.Owner.Conversation || owner.GetHost() != op.Destination.Host || owner.GetEpoch() == "" || owner.GetEpoch() == op.Owner.Epoch || response.GetOperationId() != op.HandoverId {
				result.err = fmt.Errorf("original recovery returned unvalidated ownership")
				return result
			}
			selection := DevHostSelection{Identity: o.Selection.Identity, Endpoint: o.Selection.Endpoint, TokenFile: o.Selection.TokenFile, Conversation: owner.Conversation, Epoch: owner.Epoch, ConfigAuthority: offer.destination.ConfigAuthority}
			result.selection = &selection
			result.client, result.err = selection.Dial(ctx)
			return result
		}
	}})
	m.refreshViewport()
	return m, nil
}
