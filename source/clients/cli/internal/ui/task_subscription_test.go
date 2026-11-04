package ui

import (
	"strings"
	"testing"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
)

func TestTaskSubscriptionProgressSurvivesCancelAndDeduplicates(t *testing.T) {
	m := New(nil, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.convID = "owner"
	m.ensureTaskSubscription()
	deliver := func(event *proto.TaskSubscriptionEvent) {
		m.applyTaskSubscription(taskSubscriptionMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.TaskUpdate{Event: event}})
	}
	snapshot := func(status string) *proto.TaskSubscriptionEvent {
		return &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: "task", ChildId: "child", Status: status, Task: "work"}}}
	}
	progress := func(seq int64, text string) *proto.TaskSubscriptionEvent {
		return &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Progress{Progress: &proto.TaskProgressRecord{TaskId: "task", ChildId: "child", Sequence: seq, Kind: "model_response", PayloadJson: `{"blocks":[{"type":"text","text":"` + text + `"}]}`}}}
	}
	deliver(snapshot("running"))
	deliver(progress(1, "before"))
	m.cancelStream = func() {}
	m.streaming = true
	m.cancelCurrentStreamWithNotice(false)
	tab := m.chatTabs.tabs["child"]
	if tab == nil || tab.done {
		t.Fatal("parent cancellation killed task tab")
	}
	deliver(progress(2, "after"))
	count := len(tab.view.entries)
	deliver(progress(2, "after"))
	if len(tab.view.entries) != count {
		t.Fatal("duplicate replay added another entry")
	}
	found := false
	for _, entry := range tab.view.entries {
		if strings.Contains(entry.Content, "after") {
			found = true
		}
	}
	if !found {
		t.Fatal("post-cancellation progress missing")
	}
	deliver(snapshot("completed"))
	if !tab.done || tab.view.streaming {
		t.Fatal("terminal status not applied")
	}
	m.closeSubAgentTab("child")
	deliver(snapshot("completed"))
	deliver(progress(3, "late"))
	if m.chatTabs.tabs["child"] != nil {
		t.Fatal("dismissed tab resurrected")
	}
}
func TestTaskSubscriptionChatSwitchCancelsAndFencesOldEvents(t *testing.T) {
	m := New(nil, false)
	m.convID = "owner"
	m.ensureTaskSubscription()
	oldGeneration := m.taskSubscriptionGeneration
	canceled := false
	m.taskSubscriptionCancel = func() { canceled = true }
	m.convID = "other"
	m.ensureTaskSubscription()
	if !canceled {
		t.Fatal("chat switch retained old subscription")
	}
	event := &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: "secret", ChildId: "foreign", Status: "running"}}}
	m.applyTaskSubscription(taskSubscriptionMsg{parent: "owner", generation: oldGeneration, update: agentclient.TaskUpdate{Event: event}})
	if m.chatTabs.tabs["foreign"] != nil {
		t.Fatal("old chat update crossed scope")
	}
	// Switching back does not authorize queued events from the earlier subscription.
	m.convID = "owner"
	m.ensureTaskSubscription()
	m.applyTaskSubscription(taskSubscriptionMsg{parent: "owner", generation: oldGeneration, update: agentclient.TaskUpdate{Event: event}})
	if m.chatTabs.tabs["foreign"] != nil {
		t.Fatal("stale subscription generation accepted")
	}
}

func TestTaskUpdatesPreserveTypedInput(t *testing.T) {
	m := New(nil, false)
	m.convID = "owner"
	m.ensureTaskSubscription()
	m.input.SetValue("next request still being typed")
	snapshot := &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: "task", ChildId: "child", Status: "completed"}}}
	m.applyTaskSubscription(taskSubscriptionMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.TaskUpdate{Event: snapshot}})
	if m.input.Value() != "next request still being typed" || m.streaming {
		t.Fatal("completion changed input or claimed the main stream")
	}
}

func TestTaskSubscriptionRestoresNestedTitles(t *testing.T) {
	m := New(nil, false)
	m.convID = "owner"
	m.ensureTaskSubscription()
	// Simulate an already populated chat with the next top-level ordinal 35.
	m.ensureChatTabs()
	m.chatTabs.childCounts[mainChatTabID] = 34
	deliver := func(id, child, parent string) {
		m.applyTaskSubscription(taskSubscriptionMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.TaskUpdate{Event: &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: id, ChildId: child, ParentId: parent, Status: "running", Task: "work"}}}}})
	}
	deliver("parent-task", "parent-child", "owner")
	deliver("leaf-task", "leaf-child", "parent-child")
	deliver("deep-task", "deep-child", "leaf-child")
	for id, want := range map[string]string{"parent-child": "sub 35", "leaf-child": "sub 35.1", "deep-child": "sub 35.1.1"} {
		tab := m.chatTabs.tabs[id]
		if tab == nil || tab.title != want {
			t.Fatalf("%s: %+v; want %s", id, tab, want)
		}
	}
	deliver("leaf-task", "leaf-child", "parent-child")
	if m.chatTabs.childCounts["parent-child"] != 1 {
		t.Fatal("replayed snapshot changed hierarchy numbering")
	}
}
