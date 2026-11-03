package ui

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	protobuf "google.golang.org/protobuf/proto"
)

type forcedResumeOffer struct {
	source, destination DevHostSelection
	owner               *proto.LifecycleOwner
	receipt             string
	dead                bool
}

func (o *forcedResumeOffer) Error() string {
	return "conversation belongs to another server; forced resumption requires confirmation"
}
func (o *forcedResumeOffer) request() *proto.LifecycleControlRequest {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%q/%q/%q/%q", o.owner.Conversation, o.owner.Host, o.owner.Epoch, o.destination.Identity.Host)))
	return &proto.LifecycleControlRequest{Target: o.source.Identity, Source: o.source.Identity, Destination: o.destination.Identity, Owner: o.owner, DestinationEndpoint: o.destination.Endpoint, HandoverId: fmt.Sprintf("force-%x", sum[:16]), Management: true, Receipt: o.receipt}
}
func (m Model) offerForcedResume(offer *forcedResumeOffer) (tea.Model, tea.Cmd) {
	client, parent, gen := m.agent, m.convID, m.taskSubscriptionGeneration
	decline := func(m Model) (Model, tea.Cmd) { m.pendingConfirm = nil; return m, nil }
	title := "Force resume on the new server?"
	details := []string{fmt.Sprintf("Move conversation %s from server %s to %s.", offer.owner.Conversation, offer.owner.Host, offer.destination.Identity.Host), "This revokes the old clients and cancels unfinished work. External actions already performed cannot be undone. The old server will not be used as a fallback."}
	hints := "y: cancel work and move · n/esc/enter: cancel"
	if offer.dead {
		title = "Recover conversation from the exited server?"
		details = []string{fmt.Sprintf("Server %s has been verified absent. Recover conversation %s on %s?", offer.owner.Host, offer.owner.Conversation, offer.destination.Identity.Host), "Unfinished tasks will be marked interrupted, not replayed. External effects may remain uncertain. Other conversations are not changed."}
		hints = "y: recover · n/esc/enter: cancel"
	}
	m.enqueueConfirmation(&confirmRequest{
		title: title, details: details, hints: hints,
		extras: map[string]func(Model) (Model, tea.Cmd){"enter": decline}, onNo: decline,
		onYes: func(m Model) (Model, tea.Cmd) {
			m.pendingConfirm = nil
			if m.agent != client || m.convID != parent || m.taskSubscriptionGeneration != gen || m.devHost == nil || m.devHost.busy {
				return m, nil
			}
			m.devHost.busy = true
			notice := "Forced resumption: cancelling old execution and transferring to the new server…"
			if offer.dead {
				notice = "Recovering from the exited server: rechecking absence and reconciling interrupted work…"
			}
			m.mainChat().AppendNotice(&Entry{Role: RoleSystem, Content: notice})
			m.refreshViewport()
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				pending := *offer
				r := devHostResult{action: "reopen", fenced: true, origin: client, originID: parent, generation: gen, cutover: &pending}
				dc, err := offer.destination.controls(ctx)
				if err != nil {
					r.err = err
					return r
				}
				defer dc.Close()
				var response *proto.LifecycleControlResponse
				if offer.dead {
					response, err = dc.LifecycleRecover(ctx, &proto.LifecycleRecoverRequest{Target: offer.destination.Identity, Conversation: offer.owner.Conversation, ExpectedOwner: offer.owner, Confirm: true})
				} else if pending.receipt != "" {
					request := pending.request()
					request.Target = request.Destination
					response, err = dc.LifecycleClaim(ctx, request)
				} else {
					sc, dialErr := offer.source.controls(ctx)
					if dialErr != nil {
						r.err = dialErr
						return r
					}
					defer sc.Close()
					response, err = sc.ForceTransfer(ctx, dc, pending.request())
				}
				if response.GetReceipt() != "" {
					pending.receipt = response.Receipt
				}
				if err != nil {
					if status.Code(err) == codes.Unimplemented {
						err = fmt.Errorf("a selected server does not support the forced-cutover protocol; cutover is not confirmed and no terminal-only fallback was attempted: %w", err)
					}
					r.err = err
					return r
				}
				owner := response.GetOwner()
				if response.GetPending() || !protobuf.Equal(response.GetIdentity(), offer.destination.Identity) || owner.GetConversation() != offer.owner.Conversation || owner.GetHost() != offer.destination.Identity.Host || owner.GetEpoch() == "" || owner.GetEpoch() == offer.owner.Epoch {
					r.err = fmt.Errorf("cutover did not return validated new-server ownership")
					return r
				}
				selection := offer.destination
				selection.Conversation, selection.Epoch = owner.Conversation, owner.Epoch
				r.selection = &selection
				r.client, r.err = selection.Dial(ctx)
				return r
			}
		},
	})
	m.refreshViewport()
	return m, nil
}
