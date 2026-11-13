package ui

import (
	"strings"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func workingTabsModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.applySubAgentEvent(subAgentEventMsg{id: "one", title: "Research", kind: "started"})
	m.applySubAgentEvent(subAgentEventMsg{id: "two", title: "Tests", kind: "started"})
	m.refreshViewport()
	return m
}

func TestWorkingAgentTabAnimation(t *testing.T) {
	m := workingTabsModel(t)
	m.chatTabs.tabs["one"].applyTaskLifecycle("task-one", "accepted")
	m.streaming = false // independent children animate even with an idle parent
	var previous string
	var segments []tabStripSegment
	for i, ms := range []int64{800, 880, 960} {
		next, cmd := m.Update(progressAnimTickMsg(time.UnixMilli(ms)))
		m = next.(Model)
		if cmd == nil || !m.animTickActive {
			t.Fatal("working tab did not keep tick alive")
		}
		items := m.chatTabItems()
		if items[0].Label != "main" || items[1].Label != "Research ○" {
			t.Fatalf("idle/queued changed: %v", items)
		}
		want := "Tests " + string(workingAgentFrames[i])
		if items[2].Label != want {
			t.Fatalf("working label %q want %q", items[2].Label, want)
		}
		strip := m.renderChatTabStrip()
		if strip == previous || !strings.Contains(ansi.Strip(m.View().Content), want) {
			t.Fatal("working tab did not repaint")
		}
		if m.renderChatTabStrip() != strip {
			t.Fatal("render advanced the frame")
		}
		now := tabStripSegments(items)
		if i > 0 && (now[2] != segments[2] || lipgloss.Width(strip) != lipgloss.Width(previous)) {
			t.Fatal("animation moved tab hit regions")
		}
		for _, seg := range now {
			id, close, ok := tabStripHitAtX(items, seg.start)
			if !ok || id != seg.id || close {
				t.Fatal("bad tab hit region")
			}
			if seg.closeStart > 0 {
				id, close, ok = tabStripHitAtX(items, seg.closeStart)
				if !ok || !close || id != seg.id {
					t.Fatal("bad close hit region")
				}
			}
		}
		segments, previous = now, strip
	}
	// Selecting a different tab must not stop the background child's indication.
	m.switchChatTab("one")
	if !m.hasWorkingAgentTabs() {
		t.Fatal("selection hid working agent")
	}
}

func TestWorkingAgentTabsStaticStates(t *testing.T) {
	for _, status := range []string{"accepted", "completed", "failed", "stopped", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			m := workingTabsModel(t)
			for _, id := range []string{"one", "two"} {
				m.chatTabs.tabs[id].applyTaskLifecycle("task-"+id, status)
			}
			before := m.renderChatTabStrip()
			next, cmd := m.Update(progressAnimTickMsg(time.UnixMilli(880)))
			m = next.(Model)
			if cmd != nil || m.animTickActive || m.renderChatTabStrip() != before {
				t.Fatal("static tabs animated")
			}
			label := m.chatTabItems()[1].Label
			want := "Research"
			if status == "accepted" {
				want += " ○"
			}
			if status == "failed" {
				want += " !"
			}
			if label != want {
				t.Fatalf("label=%q want=%q", label, want)
			}
		})
	}
	// Session-scoped permission gates still use the existing confirmation UI;
	// they must not claim every independently working child is blocked.
	m := workingTabsModel(t)
	m.pendingConfirm = &confirmRequest{tool: &pendingToolCall{Name: "Edit"}}
	if !m.hasWorkingAgentTabs() || m.chatTabItems()[1].Label != "Research ⠋" {
		t.Fatal("approval changed child attribution")
	}
	// Activity/tool tabs are not child agents and retain their static marker.
	m.closeSubAgentTab("one")
	m.closeSubAgentTab("two")
	m.ensureSubAgentTab("activity:tool", "", "Tool", nil)
	if m.hasWorkingAgentTabs() {
		t.Fatal("activity tab owns agent ticks")
	}
	if got := m.chatTabItems()[1].Label; got != "Tool •" {
		t.Fatal(got)
	}
}

func TestWorkingAgentTabTickLifecycle(t *testing.T) {
	m := workingTabsModel(t)
	m.content = &queuedPageStub{body: "page"}
	next, cmd := m.Update(progressAnimTickMsg(time.UnixMilli(800)))
	m = next.(Model)
	if cmd != nil || m.animTickActive {
		t.Fatal("hidden tabs kept ticks alive")
	}
	m.content = nil
	next, cmd = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(Model)
	if cmd == nil || !m.animTickActive {
		t.Fatal("visible tabs did not restart tick")
	}
	if m.ensureAnimTick() != nil {
		t.Fatal("duplicate tick scheduled")
	}
	for _, id := range []string{"one", "two"} {
		m.chatTabs.tabs[id].applyTaskLifecycle("task-"+id, "accepted")
	}
	next, cmd = m.Update(progressAnimTickMsg(time.UnixMilli(880)))
	m = next.(Model)
	if cmd != nil || m.animTickActive {
		t.Fatal("queued-only tabs kept ticks alive")
	}
	m.convID = "owner"
	m.ensureTaskSubscription()
	event := &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: "task-two", ChildId: "two", Status: "running"}}}
	next, cmd = m.Update(taskSubscriptionMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.TaskUpdate{Event: event}})
	m = next.(Model)
	if cmd == nil || !m.animTickActive {
		t.Fatal("durable running event did not restart tick")
	}
	m.closeSubAgentTab("one")
	m.closeSubAgentTab("two")
	next, cmd = m.Update(progressAnimTickMsg(time.UnixMilli(960)))
	m = next.(Model)
	if cmd != nil || m.animTickActive {
		t.Fatal("closed tabs left phantom tick")
	}
}

func TestWorkingAgentTabsReclaimActivityLayout(t *testing.T) {
	for _, size := range [][2]int{{100, 30}, {40, 20}, {80, 24}} {
		m := workingTabsModel(t)
		m = send(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.input.SetValue("DRAFTSENTINEL")
		m.recap = "Summary of ongoing work"
		m.relayout()
		liveHeight := m.mainChat().Height()
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if len(lines) > m.height {
			t.Fatalf("frame overflow %d > %d", len(lines), m.height)
		}
		if strings.Contains(strings.Join(lines, "\n"), "Agents 2:") {
			t.Fatal("activity list remains")
		}
		if !strings.Contains(lines[m.promptTop()], "DRAFTSENTINEL") {
			t.Fatal("prompt coordinates drifted")
		}
		if !strings.HasPrefix(m.renderRecap(), "\n") || !strings.Contains(ansi.Strip(m.renderRecap()), m.recap) {
			t.Fatal("recap lost spacer or summary")
		}
		for _, id := range []string{"one", "two"} {
			m.chatTabs.tabs[id].applyTaskLifecycle("task-"+id, "completed")
		}
		m.refreshViewport()
		if m.mainChat().Height() != liveHeight {
			t.Fatal("working agents still reserve extra rows")
		}
		m.recap = ""
		m.relayout()
		if got, want := m.mainChat().Height(), liveHeight+2; got != want {
			t.Fatalf("recap reclamation got=%d want=%d", got, want)
		}
		m.closeSubAgentTab("one")
		m.closeSubAgentTab("two")
		m.refreshViewport()
		if got, want := m.mainChat().Height(), liveHeight+4; got != want {
			t.Fatalf("only tab chrome should reserve rows: got=%d want=%d", got, want)
		}
	}
}
