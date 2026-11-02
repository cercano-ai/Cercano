package ui

import (
	"context"
	"fmt"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
)

type conversationAttachmentMsg struct {
	attached   bool
	parent     string
	generation uint64
	update     agentclient.ConversationUpdate
	next       tea.Cmd
}
type observedMainTurn struct {
	view       chatView
	mirrors    []*Entry
	sequence   uint64
	done       bool
	epoch      string
	generation uint64
}

func attachMainConversationCmd(ctx context.Context, client *agentclient.Client, parent string, generation uint64) tea.Cmd {
	return attachMainConversationWithTakeoverCmd(ctx, client, parent, generation, "")
}
func attachMainConversationWithTakeoverCmd(ctx context.Context, client *agentclient.Client, parent string, generation uint64, expected string) tea.Cmd {
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		var updates <-chan agentclient.ConversationUpdate
		var err error
		if expected == "" {
			updates, err = client.AttachConversation(ctx, parent)
		} else {
			updates, err = client.TakeoverConversation(ctx, parent, expected)
		}
		if err != nil {
			return conversationAttachmentMsg{parent: parent, generation: generation, update: agentclient.ConversationUpdate{Err: err}}
		}
		var wait tea.Cmd
		wait = func() tea.Msg {
			select {
			case update, ok := <-updates:
				if !ok {
					return nil
				}
				return conversationAttachmentMsg{parent: parent, generation: generation, update: update, next: wait}
			case <-ctx.Done():
				return nil
			}
		}
		return conversationAttachmentMsg{parent: parent, generation: generation, attached: true, next: wait}
	}
}

// Background turns own separate transcript state. Their mirrored entries are
// never marked as the foreground streaming target, so delayed deltas cannot
// append to a user's newer reply (or vice versa).
func (m *Model) mirrorObservedTurn(turn *observedMainTurn) {
	for index, entry := range turn.view.entries {
		copy := *entry
		copy.Streaming = false
		if index < len(turn.mirrors) {
			*turn.mirrors[index] = copy
		} else {
			turn.mirrors = append(turn.mirrors, &copy)
			m.mainChat().AppendEntry(&copy)
		}
	}
}
func (m *Model) applyConversationAttachment(msg conversationAttachmentMsg) tea.Cmd {
	if msg.parent != m.convID || msg.generation != m.taskSubscriptionGeneration {
		return nil
	}
	if msg.attached {
		return tea.Batch(msg.next, subscribeTasksCmd(m.taskSubscriptionContext, m.agent, msg.parent, msg.generation))
	}
	if msg.update.Err != nil {
		if target := agentclient.TakeoverTarget(msg.update.Err); target != "" {
			client, parent, generation := m.agent, m.convID, m.taskSubscriptionGeneration
			decline := func(m Model) (Model, tea.Cmd) {
				m.pendingConfirm = nil
				if m.agent == client && m.convID == parent && m.taskSubscriptionGeneration == generation {
					m.taskAttachmentDeclined = true
				}
				return m, nil
			}
			m.enqueueConfirmation(&confirmRequest{
				title:   "This conversation is open in another client. Take over here?",
				details: []string{"The previous client will lose control. Running work will continue on its current host."},
				hints:   "y: take over · n/esc/enter: cancel",
				extras:  map[string]func(Model) (Model, tea.Cmd){"enter": decline},
				onYes: func(m Model) (Model, tea.Cmd) {
					m.pendingConfirm = nil
					if m.agent != client || m.convID != parent || m.taskSubscriptionGeneration != generation {
						return m, nil
					}
					ctx := m.taskSubscriptionContext
					if ctx == nil {
						return m, nil
					}
					return m, attachMainConversationWithTakeoverCmd(ctx, client, parent, generation, target)
				},
				onNo: decline,
			})
			return nil
		}
		m.mainChat().AppendEntry(&Entry{Role: RoleSystem, Content: "Main chat updates disconnected: " + msg.update.Err.Error()})
		return nil
	}
	response := msg.update.Response
	if response == nil {
		return msg.next
	}
	if m.backgroundTurns == nil {
		m.backgroundTurns = make(map[string]*observedMainTurn)
		m.backgroundPrompts = make(map[string]bool)
	}
	for _, turn := range m.backgroundTurns {
		if !turn.done && (turn.epoch != response.TurnEpoch || turn.generation < response.TurnGeneration) {
			turn.done = true
			turn.view.SetStreaming(false)
			turn.view.resolveStaleInProgressTools()
			m.mirrorObservedTurn(turn)
			turn.view = chatView{}
			turn.mirrors = nil
		}
	}
	// Only locally initiated foreground messages already arrive through our
	// request stream. Inherited foreground output and approvals need this observer.
	if msg.update.Local || msg.update.Message == nil {
		return msg.next
	}

	if response.PersistedTurnId != "" {
		for _, saved := range m.resumeTurns {
			if saved.ID == response.PersistedTurnId {
				key := fmt.Sprintf("%s/%d", response.TurnEpoch, response.TurnGeneration)
				if turn := m.backgroundTurns[key]; turn != nil {
					remove := make(map[*Entry]bool)
					for _, entry := range turn.mirrors {
						remove[entry] = true
					}
					entries := m.mainChat().entries[:0]
					for _, entry := range m.mainChat().entries {
						if !remove[entry] {
							entries = append(entries, entry)
						}
					}
					m.mainChat().SetEntries(entries)
					turn.mirrors = nil
					turn.done = true
				}
				return msg.next
			}
		}
	}
	event := streamMsgToEvent(*msg.update.Message)
	if permission, ok := event.(permissionRequiredMsg); ok {
		key := response.TurnEpoch + "/" + permission.id
		if m.backgroundPrompts[key] || (m.agent != nil && !m.agent.IsCurrentConversationEvent(m.convID, response)) {
			return msg.next
		}
		m.backgroundPrompts[key] = true
		m.enqueueConfirmation(toolConfirm(&pendingToolCall{Background: true, ToolUseID: permission.id, Name: permission.name, Args: permission.argsJSON, Permission: permission.tier, Destructive: permission.destructive}))
		return msg.next
	}
	if response.EventSequence == 0 {
		return msg.next
	}
	key := fmt.Sprintf("%s/%d", response.TurnEpoch, response.TurnGeneration)
	turn := m.backgroundTurns[key]
	if turn == nil {
		turn = &observedMainTurn{view: newChatView(m.styles, m.palette, m.root, m.home, m.mainChat().Width(), m.mainChat().Height()), epoch: response.TurnEpoch, generation: response.TurnGeneration}
		m.backgroundTurns[key] = turn
	}
	if response.EventSequence <= turn.sequence {
		return msg.next
	}
	turn.sequence = response.EventSequence
	switch event.(type) {
	case subAgentEventMsg, taskChangeMsg, chatStatusMsg:
		// Child lifecycle has its own durable subscription. A background route must
		// not overwrite foreground model telemetry.
		return msg.next
	case chatDoneMsg:
		turn.done = true
	}
	turn.view.Apply(event)
	if turn.done {
		turn.view.SetStreaming(false)
		turn.view.resolveStaleInProgressTools()
	}
	m.mirrorObservedTurn(turn)
	if turn.done {
		turn.view = chatView{}
		turn.mirrors = nil
	}
	m.refreshViewport()
	return tea.Batch(msg.next, m.ensureAnimTick())
}
