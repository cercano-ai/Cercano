package ui

import (
	"context"
	"encoding/json"
	"strings"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
)

type taskSubscriptionMsg struct {
	parent     string
	generation uint64
	update     agentclient.TaskUpdate
	next       tea.Cmd
}

func (m *Model) ensureTaskSubscription() tea.Cmd {
	if m.taskSubscriptionParent == m.convID && m.taskSubscriptionClient == m.agent {
		return nil
	}
	if m.taskSubscriptionCancel != nil {
		m.taskSubscriptionCancel()
		m.taskSubscriptionCancel = nil
	}
	m.taskSubscriptionClient = m.agent
	m.taskSubscriptionParent = m.convID
	m.taskSubscriptionGeneration++
	m.taskProgressSequences = make(map[string]int64)
	m.backgroundTurns = nil
	m.backgroundPrompts = nil
	if m.agent == nil || m.convID == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.taskSubscriptionCancel = cancel
	m.taskSubscriptionContext = ctx
	parent, generation, client := m.convID, m.taskSubscriptionGeneration, m.agent
	taskCmd := func() tea.Msg {
		updates, err := client.SubscribeTasks(ctx, parent)
		if err != nil {
			return taskSubscriptionMsg{parent: parent, generation: generation, update: agentclient.TaskUpdate{Err: err}}
		}
		var wait tea.Cmd
		wait = func() tea.Msg {
			select {
			case update, ok := <-updates:
				if !ok {
					return nil
				}
				return taskSubscriptionMsg{parent: parent, generation: generation, update: update, next: wait}
			case <-ctx.Done():
				return nil
			}
		}
		return wait()
	}
	return tea.Batch(taskCmd, attachMainConversationCmd(ctx, client, parent, generation))
}
func (m *Model) applyTaskSubscription(msg taskSubscriptionMsg) tea.Cmd {
	if msg.parent != m.convID || msg.generation != m.taskSubscriptionGeneration {
		return nil
	}
	if msg.update.Err != nil {
		m.mainChat().AppendEntry(&Entry{Role: RoleSystem, Content: "Task updates disconnected: " + msg.update.Err.Error()})
		return nil
	}
	event := msg.update.Event
	if event == nil {
		return msg.next
	}
	m.ensureChatTabs()
	if task := event.GetTask(); task != nil && !m.chatTabs.closed[task.ChildId] {
		m.ensureSubAgentTab(task.ChildId, "", "", nil)
		tab := m.chatTabs.tabs[task.ChildId]
		fresh := false
		if _, seen := m.taskProgressSequences[task.TaskId]; !seen {
			fresh = true
			// A fresh subscription replays durable progress from zero. Replace any
			// transcript restored through the legacy path instead of appending it twice.
			// The durable replay re-applies the kickoff card from the dispatch_start
			// record below, so keep the card instead of dropping it to a raw prompt:
			// without it the tab loses its formatted front matter (route/model/tier/
			// tools) every time a subscription is (re)established.
			tab.view = newChatView(m.styles, m.palette, m.root, m.home, tab.view.Width(), tab.view.Height())
			kind, title := "Sub-agent", tab.title
			if tab.startup != nil {
				kind, title = tab.startup.Kind, tab.startup.Title
			}
			startup := &subAgentStartEntry{Kind: kind, Title: title, Task: strings.TrimSpace(task.Task)}
			tab.startup = startup
			tab.view.AppendEntry(&Entry{SubAgentStart: startup})
			m.taskProgressSequences[task.TaskId] = 0
		}
		previous := tab.taskStatus
		changed := tab.applyTaskLifecycle(task.TaskId, task.Status)
		if (fresh || changed) && task.Error != "" {
			tab.view.AppendEntry(&Entry{Role: RoleSystem, Content: "Task " + task.Status + ": " + task.Error})
		}
		tab.view.SetStreaming(!tab.done)
		tab.view.rebuild()
		// Regression probe task_terminal_retire_test.go: an independent task that
		// completes while no parent turn is running must retire its own tab now,
		// matching dispatch children that retire on their own "done" event,
		// instead of lingering until the next cleanupFinishedSubAgentTabs sweep.
		// retireFinishedSubAgentTab spares restored and user-dismissed tabs, and
		// failed/stopped/interrupted tabs stay for post-mortem review.
		if task.Status == "completed" && previous != "completed" {
			m.retireFinishedSubAgentTab(task.ChildId)
		}
	}
	if progress := event.GetProgress(); progress != nil && !m.chatTabs.closed[progress.ChildId] {
		tab := m.chatTabs.tabs[progress.ChildId]
		if tab == nil || tab.taskID != progress.TaskId || progress.Sequence <= m.taskProgressSequences[progress.TaskId] {
			return msg.next
		}
		m.taskProgressSequences[progress.TaskId] = progress.Sequence
		switch progress.Kind {
		case "model_response":
			var response struct{ Blocks []struct{ Type, Text string } }
			if json.Unmarshal([]byte(progress.PayloadJson), &response) == nil {
				for _, block := range response.Blocks {
					if block.Type == "text" && block.Text != "" {
						tab.view.Apply(chatAssistantMsg{text: block.Text})
					}
				}
			}
		case "tool_call":
			var call struct {
				ID   string `json:"tool_use_id"`
				Name string `json:"tool_name"`
				Args string `json:"args"`
			}
			if json.Unmarshal([]byte(progress.PayloadJson), &call) == nil {
				tab.view.Apply(toolEntryStartMsg{id: call.ID, name: call.Name})
				tab.view.Apply(toolEntryStopMsg{id: call.ID, argsSummary: call.Args})
				tab.view.Apply(toolEntryExecStartMsg{id: call.ID})
			}
		case "tool_result":
			var result struct {
				ID      string `json:"tool_use_id"`
				Content string `json:"content"`
				IsError bool   `json:"is_error"`
			}
			if json.Unmarshal([]byte(progress.PayloadJson), &result) == nil {
				tab.view.Apply(toolEntryExecCompleteMsg{id: result.ID, summary: result.Content, isError: result.IsError})
			}
		case "note":
			var note struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(progress.PayloadJson), &note) == nil && note.Text != "" {
				tab.view.AppendEntry(&Entry{Role: RoleSystem, Content: note.Text})
			}
		case "dispatch_start":
			applyTaskDispatchStartCard(tab, progress.ChildId, progress.PayloadJson)
		}
		tab.applyTaskLifecycle(tab.taskID, tab.taskStatus)
		tab.view.rebuild()
	}
	return msg.next
}

// applyTaskDispatchStartCard restores the task child tab's formatted kickoff
// card from the durable dispatch_start record replayed by SubscribeTasks. The
// live dispatch path populates the card from its ephemeral "started" stream
// event; that event is not part of the durable replay, so the same metadata
// (route/provider/model/tier/granted tools, plus the task prompt) is rebuilt
// from the Recorder's DispatchStartEvent JSON.
//
// The update is idempotent: non-empty durable values only fill empty card
// fields, so a redelivered record updates the existing card in place instead
// of duplicating the header, and a task with no dispatch_start record at all
// (accepted but not yet started) keeps an unpopulated card — no metadata is
// fabricated.
func applyTaskDispatchStartCard(tab *chatTab, childID, payload string) {
	if tab == nil {
		return
	}
	var start struct {
		Task         string   `json:"task"`
		Provider     string   `json:"provider"`
		Model        string   `json:"model"`
		Tier         string   `json:"tier"`
		IsCloud      bool     `json:"is_cloud"`
		GrantedTools []string `json:"granted_tools"`
	}
	if json.Unmarshal([]byte(payload), &start) != nil {
		return
	}
	if tab.startup == nil {
		if strings.HasPrefix(tab.id, "activity:") {
			// Activity tabs render a plain system header line, not a card.
			return
		}
		// Progress replayed before any card existed: prepend one so the
		// kickoff stays at the top of the transcript.
		tab.startup = &subAgentStartEntry{Kind: "Sub-agent", Title: tab.title, Task: strings.TrimSpace(start.Task)}
		view := tab.view
		view.SetEntriesSlice(append([]*Entry{{SubAgentStart: tab.startup}}, view.Entries()...))
	} else if tab.startup.Task == "" {
		tab.startup.Task = strings.TrimSpace(start.Task)
	}
	card := tab.startup
	if v := strings.TrimSpace(start.Provider); v != "" {
		card.Provider = v
	}
	if v := strings.TrimSpace(start.Model); v != "" {
		card.Model = v
	}
	if v := strings.TrimSpace(start.Tier); v != "" {
		card.Tier = v
	}
	// is_cloud is authoritative in the persisted record; false means the
	// dispatch ran on the local route.
	card.Route = "open"
	if start.IsCloud {
		card.Route = "cloud"
	}
	if len(card.Tools) == 0 && len(start.GrantedTools) > 0 {
		card.Tools = append([]string(nil), start.GrantedTools...)
	}
	if card.ConvID == "" {
		card.ConvID = childID
	}
}
