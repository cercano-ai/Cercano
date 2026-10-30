package ui

import (
	"cercano/source/clients/cli/internal/theme"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"strings"
	"testing"
	"time"

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
	for _, width := range []int{0, 1, 2, 10, 24, 40, 80} {
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
			if m.agentActivityShown || m.mainChat().Height() != before+2 {
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
			wantH -= 2
		}
		if m.mainChat().Height() != wantH {
			t.Fatalf("%s height=%d want=%d", status, m.mainChat().Height(), wantH)
		}
		if m.input.Value() != "draft" {
			t.Fatal("subscription lost draft")
		}
	}
}

func TestAgentActivityGutterAndCountStyle(t *testing.T) {
	m := activityModel(t)
	for _, palette := range []theme.Palette{theme.Cracker(), builtinPaletteForTest("daylight")} {
		m.styles = theme.NewStyles(palette)
		if m.styles.Success.Render("Agents 2: ") == m.styles.Info.Render("Agents 2: ") {
			t.Fatal("styling regression requires distinguishable green and blue output")
		}
		row, links := m.agentActivityRow()
		want := strings.Repeat(" ", entryIndent) + m.styles.Success.Render("Agents 2: ") +
			m.styles.Info.Render("⠋ ") + m.styles.Info.Underline(true).Render("Research (running)") +
			m.styles.Info.Render(" · ") + m.styles.Info.Render("⠋ ") + m.styles.Info.Underline(true).Render("Tests (running)")
		if row != want {
			t.Errorf("activity must use chat gutter, green count, and blue agent details: got %q want %q", row, want)
		}
		if len(links) != 2 || links[0].start != entryIndent+lipgloss.Width("Agents 2: ⠋ ") {
			t.Errorf("links must include gutter offset: %+v", links)
		}
		m.pendingConfirm = &confirmRequest{tool: &pendingToolCall{Name: "Edit"}}
		approvalRow, approvalLinks := m.agentActivityRow()
		approvalPrefix := m.styles.Info.Render("! Approval required | ")
		approvalWant := strings.Repeat(" ", entryIndent) + approvalPrefix + strings.TrimPrefix(want, strings.Repeat(" ", entryIndent))
		if approvalRow != approvalWant {
			t.Errorf("approval must stay blue with only the count green: got %q want %q", approvalRow, approvalWant)
		}
		if len(approvalLinks) != len(links) {
			t.Fatalf("approval changed links: %+v", approvalLinks)
		}
		for i, link := range links {
			shift := lipgloss.Width(approvalPrefix)
			if got := approvalLinks[i]; got.id != link.id || got.start != link.start+shift || got.end != link.end+shift {
				t.Errorf("approval link offset changed: %+v", got)
			}
		}
		m.pendingConfirm = nil
	}
}

func TestAgentActivityBeforeRecap(t *testing.T) {
	for _, width := range []int{24, 40, 100} {
		for _, queued := range []bool{false, true} {
			m := activityModel(t)
			if queued {
				m.mainChat().Enqueue("queued input wraps at narrow widths", nil)
			}
			m = send(t, m, tea.WindowSizeMsg{Width: width, Height: 30})
			m.recap = "RECAPSENTINEL summary wraps at narrow widths"
			m.input.SetValue("DRAFTSENTINEL")
			m.relayout()
			parts, _ := m.composeFrame()
			row, links := m.agentActivityRow()
			activityIdx, activityY := -1, 0
			for i, part := range parts {
				if part == row {
					activityIdx = i
					break
				}
				activityY += countLines([]string{part})
			}
			if activityIdx < 2 || parts[activityIdx-1] != "" || parts[activityIdx-2] != m.renderViewportWithScrollbar() || activityIdx+1 >= len(parts) || parts[activityIdx+1] != m.renderRecap() {
				t.Errorf("width %d: activity must have one spacer above and directly precede recap (index %d)", width, activityIdx)
			}
			for x := 0; x < width; x++ {
				want := ""
				for _, link := range links {
					if x >= link.start && x < link.end {
						want = link.id
					}
				}
				if got := m.agentActivityHit(x, activityY); got != want {
					t.Fatalf("x=%d y=%d hit=%q want=%q", x, activityY, got, want)
				}
				if got := m.agentActivityHit(x, activityY-1); got != "" {
					t.Fatalf("spacer hit %q", got)
				}
				if got := m.agentActivityHit(x, activityY+1); got != "" {
					t.Fatalf("off-row hit %q", got)
				}
			}
			lines := strings.Split(ansi.Strip(m.View().Content), "\n")
			if lines[activityY-1] != "" {
				t.Fatal("activity spacer missing from rendered frame")
			}
			if lines[activityY] != ansi.Strip(row) {
				t.Fatal("activity hit coordinates do not match rendered frame")
			}
			if want := ansi.Strip(m.recapLines()[0]); lines[activityY+1] != want {
				t.Fatalf("recap must immediately follow activity: next row=%q want=%q", lines[activityY+1], want)
			}
			if len(lines) > m.height {
				t.Fatalf("frame overflows: %d > %d", len(lines), m.height)
			}
			if !strings.Contains(lines[m.promptTop()], "DRAFTSENTINEL") {
				t.Fatal("prompt coordinates drifted")
			}
		}
	}
}

func TestAgentActivityRecapHeight(t *testing.T) {
	m := activityModel(t)
	before := m.mainChat().Height()
	m.recap = "A summary that wraps over multiple lines at narrow widths"
	m.relayout()
	if got, want := m.mainChat().Height(), before-len(m.recapLines()); got != want {
		t.Fatalf("activity recap height=%d want=%d", got, want)
	}
	withActivity := m.mainChat().Height()
	for _, id := range []string{"one", "two"} {
		m.chatTabs.tabs[id].applyTaskLifecycle("task-"+id, "completed")
	}
	m.refreshViewport()
	// Reclaim activity's two rows, but restore recap's standalone spacer.
	if got, want := m.mainChat().Height(), withActivity+1; got != want {
		t.Fatalf("standalone recap height=%d want=%d", got, want)
	}
	if !strings.HasPrefix(m.renderRecap(), "\n") {
		t.Fatal("standalone recap lost its spacer")
	}
}

func TestAgentActivityTickAndQueuedIndicator(t *testing.T) {
	m := activityModel(t)
	// Leave the parent idle and all child transcripts quiet: only the row
	// should own animation, even while the main tab is selected.
	m.chatTabs.tabs["one"].applyTaskLifecycle("task-one", "accepted")
	m.switchChatTab(mainChatTabID)
	m.streaming = false
	var previous string
	for _, ms := range []int64{800, 880, 960} {
		next, cmd := m.Update(progressAnimTickMsg(time.UnixMilli(ms)))
		m = next.(Model)
		if cmd == nil || !m.animTickActive {
			t.Fatal("running child did not keep ticks alive")
		}
		row, links := m.agentActivityRow()
		frameLines := strings.Split(ansi.Strip(m.View().Content), "\n")
		activityY := -1
		for y, line := range frameLines {
			if line == ansi.Strip(row) {
				activityY = y
			}
		}
		if activityY < 0 {
			t.Fatal("animated row missing from frame")
		}
		for _, link := range links {
			if m.agentActivityHit(link.start, activityY) != link.id || m.agentActivityHit(link.start-1, activityY) != "" {
				t.Fatalf("tick changed name/indicator hit coordinates: %+v", link)
			}
		}
		if !strings.Contains(ansi.Strip(row), "○ Research (queued)") {
			t.Fatal(row)
		}
		if row == previous {
			t.Fatal("running indicator did not animate")
		}
		for _, link := range links {
			if got := ansi.Cut(row, link.start, link.end); !strings.Contains(got, m.styles.Info.Underline(true).Render(map[string]string{"one": "Research (queued)", "two": "Tests (running)"}[link.id])) {
				t.Fatalf("name is not blue and underlined within click range: %q", got)
			}
		}
		previous = row
	}
	m.chatTabs.tabs["two"].applyTaskLifecycle("task-two", "accepted")
	before, _ := m.agentActivityRow()
	next, cmd := m.Update(progressAnimTickMsg(time.UnixMilli(1040)))
	m = next.(Model)
	if cmd != nil || m.animTickActive {
		t.Fatal("queued-only children kept animation alive")
	}
	after, _ := m.agentActivityRow()
	if before != after || strings.Count(ansi.Strip(after), "○ ") != 2 {
		t.Fatal("queued indicators are not static")
	}
	// A durable running event must restart an idle tick loop.
	m.convID = "owner"
	m.ensureTaskSubscription()
	event := &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: "task-two", ChildId: "two", Status: "running"}}}
	next, cmd = m.Update(taskSubscriptionMsg{parent: "owner", generation: m.taskSubscriptionGeneration, update: agentclient.TaskUpdate{Event: event}})
	m = next.(Model)
	if cmd == nil || !m.animTickActive {
		t.Fatal("durable running event did not restart animation")
	}
}

func TestAgentActivityNarrowNamesAndTheme(t *testing.T) {
	m := activityModel(t)
	m.chatTabs.tabs["one"].title = "界面 👩‍💻 very long name"
	for _, palette := range []theme.Palette{theme.Cracker(), builtinPaletteForTest("daylight")} {
		m.styles = theme.NewStyles(palette)
		for width := 0; width <= 100; width++ {
			m.width = width
			row, links := m.agentActivityRow()
			if lipgloss.Width(row) > width || strings.Contains(row, "\n") {
				t.Fatalf("width %d: %q", width, row)
			}
			for _, link := range links {
				if link.start < 2 || link.end > width || link.end <= link.start {
					t.Fatalf("width %d: %+v", width, link)
				}
				name := ansi.Cut(row, link.start, link.end)
				if strings.ContainsAny(ansi.Strip(name), "⠋○") {
					t.Fatalf("indicator included in name hit range: %q", name)
				}
			}
		}
	}
}
