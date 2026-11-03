package ui

import (
	"context"
	"fmt"
	"time"

	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	protobuf "google.golang.org/protobuf/proto"
)

type forcedResumeOffer struct {
	destination DevHostSelection
	owner       *proto.LifecycleOwner
	dead        bool
}

func (o *forcedResumeOffer) Error() string {
	return "conversation belongs to an exited server; recovery requires confirmation"
}
func (m Model) offerForcedResume(offer *forcedResumeOffer) (tea.Model, tea.Cmd) {
	client, parent, gen := m.agent, m.convID, m.taskSubscriptionGeneration
	decline := func(m Model) (Model, tea.Cmd) { m.pendingConfirm = nil; return m, nil }
	title := "Recover conversation from the exited server?"
	details := []string{fmt.Sprintf("Server %s has been verified absent. Recover conversation %s on %s?", offer.owner.Host, offer.owner.Conversation, offer.destination.Identity.Host), "Unfinished tasks will be marked interrupted, not replayed. External effects may remain uncertain. Other conversations are not changed."}
	hints := "y: recover · n/esc/enter: cancel"
	m.enqueueConfirmation(&confirmRequest{
		title: title, details: details, hints: hints,
		extras: map[string]func(Model) (Model, tea.Cmd){"enter": decline}, onNo: decline,
		onYes: func(m Model) (Model, tea.Cmd) {
			m.pendingConfirm = nil
			if m.agent != client || m.convID != parent || m.taskSubscriptionGeneration != gen || m.devHost == nil || m.devHost.busy {
				return m, nil
			}
			m.devHost.busy = true
			notice := "Recovering from the exited server: rechecking absence and reconciling interrupted work…"
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
				response, err := dc.LifecycleRecover(ctx, &proto.LifecycleRecoverRequest{Target: offer.destination.Identity, Conversation: offer.owner.Conversation, ExpectedOwner: offer.owner, Confirm: true})
				if err != nil {
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
