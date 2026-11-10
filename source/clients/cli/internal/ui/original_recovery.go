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
	// The wire envelope is retained until the protocol migration. Only the
	// qualified, contained-source coordinator may create a UI recovery offer.
	if o.GetTakeover() || o.GetLegacy() || op.GetLegacyProcessIdentity() == "" || op.GetSource().GetStore() != id.GetStore() || err != nil || ip == nil || !ip.IsLoopback() || o.GetChallenge() == "" || sel.GetTokenFile() == "" || id.GetHost() == "" || id.GetBuild() == "" || id.GetStore() == "" || id.GetContract() == "" || id.GetSchema() <= 0 || !proto.Equal(id, op.GetDestination()) || !proto.Equal(op.GetTarget(), op.GetSource()) || op.GetOwner().GetConversation() != conversation || op.GetOwner().GetEpoch() == "" || op.GetOwner().GetHost() != op.GetSource().GetHost() || op.GetSource().GetHost() == "" || op.GetSource().GetHost() == id.GetHost() || op.GetHandoverId() == "" {
		return fmt.Errorf("invalid contained-source recovery selection")
	}
	return nil
}

func (m Model) offerOriginalRecovery(offer *forcedResumeOffer) (tea.Model, tea.Cmd) {
	o := offer.original
	if err := validateOriginalOffer(o, o.GetOperation().GetOwner().GetConversation()); err != nil || !proto.Equal(o.GetSelection().GetIdentity(), offer.destination.Identity) || o.GetSelection().GetEndpoint() != offer.destination.Endpoint || o.GetSelection().GetTokenFile() != offer.destination.TokenFile {
		m.errMsg = "Recovery offer does not match the selected destination"
		return m, nil
	}
	op := o.Operation
	client, parent, gen := m.agent, m.convID, m.taskSubscriptionGeneration
	decline := func(m Model) (Model, tea.Cmd) { m.pendingConfirm = nil; return m, nil }
	title := "Stop the old host and recover this conversation?"
	mode := "This interrupts the WHOLE old host, including sibling conversations and tasks. Committed history is preserved. Interrupted actions are NOT replayed; external effects may remain uncertain. Consent is single-use for this source epoch and selected destination."
	selectionDescription := fmt.Sprintf("Conversation %s: source %s, epoch %s, operation %s → selected destination %s.", op.Owner.Conversation, op.Source.Host, op.Owner.Epoch, op.HandoverId, op.Destination.Host)
	hints := "y: stop whole old host and recover · n/esc/enter: cancel"
	m.enqueueConfirmation(&confirmRequest{title: title, details: []string{
		selectionDescription,
		mode, "Recovery stops the attributed source and interrupts conversation work. A stuck drain or uncertain containment is refused. The server revalidates identities and exact ownership before reassignment. Consent applies only to this attempt."}, hints: hints, extras: map[string]func(Model) (Model, tea.Cmd){"enter": decline}, onNo: decline, onYes: func(m Model) (Model, tea.Cmd) {
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
			request := &pb.LifecycleControlRequest{Target: offer.destination.Identity, Owner: &pb.LifecycleOwner{Conversation: op.Owner.Conversation}, Requester: offer.requester, RecoveryChallenge: o.Challenge, ConfirmOriginalRecovery: true}
			response, err := dc.LifecycleResume(ctx, request)
			if err != nil {
				result.err = err
				return result
			}
			if response.GetConfirmationRequired() {
				result.err = fmt.Errorf("recovery requires a fresh reopen and confirmation")
				return result
			}
			recovered := response.GetOriginalRecovery()
			owner := response.GetOwner()
			if !response.GetCurrentOwner() || !response.GetReady() || response.GetPending() || !proto.Equal(response.GetIdentity(), op.Destination) || !proto.Equal(recovered.GetOperation(), op) || !proto.Equal(recovered.GetSelection(), o.Selection) || owner.GetConversation() != op.Owner.Conversation || owner.GetHost() != op.Destination.Host || owner.GetEpoch() == "" || owner.GetEpoch() == op.Owner.Epoch || response.GetOperationId() != op.HandoverId {
				result.err = fmt.Errorf("recovery returned unvalidated current ownership")
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
