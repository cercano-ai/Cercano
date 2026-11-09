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
	original        *proto.LifecycleOriginalRecovery
	destination     DevHostSelection
	owner           *proto.LifecycleOwner
	dead            bool // retained for old fixture helpers; normal recovery is automatic
	source          *proto.LifecycleIdentity
	requester       *proto.LifecycleIdentity
	operation       string
	processIdentity string
	blocked         string
}

func (o *forcedResumeOffer) Error() string {
	return "legacy source requires explicit whole-host interruption confirmation"
}
func (m Model) offerForcedResume(offer *forcedResumeOffer) (tea.Model, tea.Cmd) {
	if offer.original != nil {
		return m.offerOriginalRecovery(offer)
	}
	client, parent, gen := m.agent, m.convID, m.taskSubscriptionGeneration
	decline := func(m Model) (Model, tea.Cmd) { m.pendingConfirm = nil; return m, nil }
	title := "Interrupt the entire legacy source host?"
	details := []string{fmt.Sprintf("Stop source host %s (build %s, store %s) to resume conversation %s on %s?", offer.owner.Host, offer.source.GetBuild(), offer.source.GetStore(), offer.owner.Conversation, offer.destination.Identity.Host), "Sibling conversation and task inventory is incomplete. ALL conversations and running tasks on this source host may be interrupted, not just the selected conversation. External effects may remain uncertain.", "The server will revalidate the exact source process, owner epoch, destination and operation before any stop. " + offer.blocked}
	hints := "y: interrupt whole source host · n/esc/enter: cancel"
	m.enqueueConfirmation(&confirmRequest{
		title: title, details: details, hints: hints,
		extras: map[string]func(Model) (Model, tea.Cmd){"enter": decline}, onNo: decline,
		onYes: func(m Model) (Model, tea.Cmd) {
			m.pendingConfirm = nil
			if m.agent != client || m.convID != parent || m.taskSubscriptionGeneration != gen || m.devHost == nil || m.devHost.busy {
				return m, nil
			}
			m.devHost.busy = true
			notice := "Rechecking confirmed legacy host identity before whole-host interruption…"
			m.mainChat().AppendNotice(&Entry{Role: RoleSystem, Content: notice})
			m.refreshViewport()
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				r := devHostResult{action: "reopen", fenced: true, origin: client, originID: parent, generation: gen}
				dc, err := offer.destination.controls(ctx)
				if err != nil {
					r.err = err
					return r
				}
				defer dc.Close()
				response, err := dc.LifecycleResume(ctx, &proto.LifecycleControlRequest{Target: offer.destination.Identity, Source: offer.source, Owner: offer.owner, HandoverId: offer.operation, DestinationEndpoint: offer.destination.Endpoint, Requester: offer.requester, ConfirmLegacyStop: true, LegacyProcessIdentity: offer.processIdentity})
				if err != nil {
					r.err = err
					return r
				}
				owner := response.GetOwner()
				if !response.GetReady() || response.GetConfirmationRequired() || response.GetPending() || !protobuf.Equal(response.GetIdentity(), offer.destination.Identity) || owner.GetConversation() != offer.owner.Conversation || owner.GetHost() != offer.destination.Identity.Host || owner.GetEpoch() == "" || owner.GetEpoch() == offer.owner.Epoch {
					r.err = fmt.Errorf("cutover did not return validated new-server ownership")
					return r
				}
				selection := offer.destination
				selection.LaunchOperation, selection.OperationFile = "", ""
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
