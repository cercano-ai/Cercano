package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func activityModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.applySubAgentEvent(subAgentEventMsg{id: "one", title: "Research", kind: "started"})
	m.applySubAgentEvent(subAgentEventMsg{id: "two", title: "Tests", kind: "started"})
	m.refreshViewport()
	return m
}

func TestAgentActivityRendering(t *testing.T) {
	m := New(nil, false)
	if row, _ := m.agentActivityRow(); row != "" {
		t.Fatal(row)
	}
	m = activityModel(t)
	m.chatTabs.tabs["one"].applyTaskLifecycle("task1", "accepted")
	m.applySubAgentEvent(subAgentEventMsg{id: "three", title: "Review", kind: "started"})
	row, links := m.agentActivityRow()
	for _, want := range []string{"Agents 3", "Research (queued)", "Tests (running)", "+1 more"} {
		if !strings.Contains(ansi.Strip(row), want) {
			t.Errorf("row %q lacks %q", row, want)
		}
	}
	if len(links) != 2 {
		t.Fatalf("links: %v", links)
	}
	// Parent cancellation/idle must not hide independently running children.
	m.streaming = false
	if row2, _ := m.agentActivityRow(); row2 != row {
		t.Fatal("parent changed activity")
	}
	m.pendingConfirm = &confirmRequest{tool: &pendingToolCall{Name: "Edit"}}
	row, _ = m.agentActivityRow()
	if !strings.Contains(row, "! Approval required") {
		t.Fatal(row)
	}
	m.pendingConfirm.stale = true
	row, _ = m.agentActivityRow()
	if strings.Contains(row, "Approval required") {
		t.Fatal("stale approval: " + row)
	}
	m.pendingConfirm = nil
	for _, width := range []int{1, 10, 24, 40, 80} {
		m.width = width
		row, links = m.agentActivityRow()
		if lipgloss.Width(row) > width || strings.Contains(row, "\n") {
			t.Fatalf("width %d: %q", width, row)
		}
		for _, link := range links {
			if link.end > width {
				t.Fatalf("invisible hit target: %+v", link)
			}
		}
	}
}

func TestAgentActivityLifecycleAndLayout(t *testing.T) {
	for _, status := range []string{"completed", "failed", "stopped", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			m := activityModel(t)
			m.input.SetValue("keep my draft")
			before := m.mainChat().Height()
			for _, id := range []string{"one", "two"} {
				m.chatTabs.tabs[id].applyTaskLifecycle("task-"+id, status)
			}
			m.refreshViewport()
			if row, _ := m.agentActivityRow(); row != "" {
				t.Fatal("terminal activity: " + row)
			}
			if m.agentActivityShown || m.mainChat().Height() != before+1 {
				t.Fatal("activity row not reclaimed while terminal tabs remain")
			}
			if m.input.Value() != "keep my draft" {
				t.Fatal("draft lost")
			}
		})
	}
	m := activityModel(t)
	m.applySubAgentEvent(subAgentEventMsg{id: "one", kind: "done"})
	m.applySubAgentEvent(subAgentEventMsg{id: "two", kind: "error"})
	m.refreshViewport()
	if row, _ := m.agentActivityRow(); row != "" {
		t.Fatal(row)
	}
	// A dismissed live child doesn't leave a phantom activity indicator.
	m = activityModel(t)
	m.closeSubAgentTab("one")
	m.closeSubAgentTab("two")
	m.refreshViewport()
	if m.agentActivityShown {
		t.Fatal("closed tabs still active")
	}
}

func TestAgentActivityClickAndPromptIntegration(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {40, 20}, {80, 24}} {
		m := activityModel(t)
		m = send(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.input.SetValue("DRAFTSENTINEL")
		m.relayout()
		frame := ansi.Strip(m.View().Content)
		lines := strings.Split(frame, "\n")
		activityY, promptY := -1, -1
		for y, line := range lines {
			if strings.Contains(line, "Agents 2:") {
				activityY = y
			}
			if strings.Contains(line, "DRAFTSENTINEL") {
				promptY = y
			}
		}
		if activityY < 0 || promptY <= activityY || promptY != m.promptTop() {
			t.Fatalf("activity=%d prompt=%d hit=%d", activityY, promptY, m.promptTop())
		}
		if len(lines) > size[1] {
			t.Fatalf("overflow %d > %d, body=%d splash=%v frame=%q", len(lines), size[1], m.mainChat().Height(), m.splashShown, frame)
		}
		_, links := m.agentActivityRow()
		for _, link := range links {
			m.switchChatTab(mainChatTabID)
			m = send(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: link.start, Y: activityY})
			if m.chatTabs.active != link.id {
				t.Fatalf("click %v selected %q", link, m.chatTabs.active)
			}
			if m.input.Value() != "DRAFTSENTINEL" {
				t.Fatal("click changed draft")
			}
		}
		if id := m.agentActivityHit(size[0]-1, activityY+1); id != "" {
			t.Fatal("off-row hit: " + id)
		}
	}
}

func TestAgentActivitySubscriptionRelayout(t *testing.T) {
	m := New(nil, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.convID = "owner"
	m.ensureTaskSubscription()
	m.input.SetValue("draft")
	initialH := m.mainChat().Height()
	for _, status := range []string{"accepted", "running", "failed"} {
		event := &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: "task", ChildId: "child", Status: status}}}
		m = send(t, m, taskSubscriptionMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.TaskUpdate{Event: event}})
		row, _ := m.agentActivityRow()
		live := status != "failed"
		if m.agentActivityShown != live {
			t.Fatalf("%s activity layout flag=%v row=%q", status, m.agentActivityShown, row)
		}
		wantH := initialH - 2
		if live {
			wantH--
		}
		if m.mainChat().Height() != wantH {
			t.Fatalf("%s height=%d want=%d", status, m.mainChat().Height(), wantH)
		}
		if m.input.Value() != "draft" {
			t.Fatal("subscription lost draft")
		}
	}
}
