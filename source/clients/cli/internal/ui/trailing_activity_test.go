package ui

import (
	"strings"
	"testing"
	"time"

	"cercano/source/clients/cli/internal/theme"
	tea "charm.land/bubbletea/v2"
)

// IsBetweenPhases is false when nothing is streaming — the whole indicator
// only makes sense as a "still working" signal mid-turn.
func TestChatView_IsBetweenPhases_FalseWhenNotStreaming(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetEntriesSlice([]*Entry{
		{Role: RoleUser, Content: "hi"},
		{Role: RoleAssistant, Content: "done"},
	})
	if c.IsBetweenPhases() {
		t.Errorf("not streaming: IsBetweenPhases must be false")
	}
}

// While a tool is in-progress, the tool spinner already covers liveness;
// trailing activity should be silent to avoid two competing indicators.
func TestChatView_IsBetweenPhases_FalseWhileToolInProgress(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	c.SetEntriesSlice([]*Entry{
		{Tool: &ToolEntry{ToolName: "Read", Status: ToolStatusInProgress, StartedAt: time.Now()}},
	})
	if c.IsBetweenPhases() {
		t.Errorf("tool in progress: IsBetweenPhases must be false")
	}
}

// The empty-content streaming assistant is its own (pre-text placeholder)
// indicator. Trailing activity should defer to it.
func TestChatView_IsBetweenPhases_FalseWhileStreamingTextEntry(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	c.SetEntriesSlice([]*Entry{
		{Role: RoleAssistant, Content: "", Streaming: true},
	})
	if c.IsBetweenPhases() {
		t.Errorf("streaming empty assistant: IsBetweenPhases must be false (placeholder covers it)")
	}
}

// A streaming text entry that is still actively receiving tokens (last token
// within the stale threshold) is a live stream — the prose itself is the
// visible activity, so the trailing line must stay silent.
func TestChatView_IsBetweenPhases_FalseWhileStreamFresh(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	c.SetEntriesSlice([]*Entry{
		{Role: RoleAssistant, Content: "Let me write the spec.", Streaming: true},
	})
	now := time.Now()
	c.SetAnimationTime(now)
	c.lastTokenAt = now.Add(-staleStreamThreshold / 2) // token just arrived
	if c.IsBetweenPhases() {
		t.Errorf("fresh stream (token within threshold): IsBetweenPhases must be false")
	}
}

// The dead-zone case this change fixes: a streaming text entry with content
// that has gone quiet past the threshold — the model finished a prose segment
// and is off doing work with nothing flowing. The trailing line must appear.
func TestChatView_IsBetweenPhases_TrueWhileStreamStale(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	c.SetEntriesSlice([]*Entry{
		{Role: RoleAssistant, Content: "Let me write the spec.", Streaming: true},
	})
	now := time.Now()
	c.SetAnimationTime(now)
	c.lastTokenAt = now.Add(-2 * staleStreamThreshold) // stalled well past threshold
	if !c.IsBetweenPhases() {
		t.Errorf("stale stream (no token past threshold): IsBetweenPhases must be true")
	}
}

// The target case: tools completed, no streaming text entry, but still
// streaming. This is the gap the trailing activity line covers.
func TestChatView_IsBetweenPhases_TrueAfterToolsComplete(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	c.SetEntriesSlice([]*Entry{
		{Role: RoleAssistant, Content: "thinking out loud"},
		{Tool: &ToolEntry{ToolName: "Read", Status: ToolStatusComplete, Duration: 5 * time.Millisecond}},
	})
	if !c.IsBetweenPhases() {
		t.Errorf("post-tool, streaming, no new entry: IsBetweenPhases must be true")
	}
}

// The trailing line renders BELOW the last entry when IsBetweenPhases.
// The line should include an indicator (spinner/activity text), and the
// scrollback output should contain something distinct from just the
// entries.
func TestChatView_TrailingActivityAppears(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	entries := []*Entry{
		{Role: RoleUser, Content: "do it"},
		{Tool: &ToolEntry{ToolName: "Read", ArgsSummary: "a.go", Status: ToolStatusComplete, Duration: 5 * time.Millisecond, Folded: true}},
	}
	c.SetEntriesSlice(entries)
	c.SetEntries(entries)
	got := stripAnsiCSI(strings.Join(c.PlainLines(), "\n"))
	// The trailing activity uses turnStatusLine which includes either
	// "thinking" (the default) or the current activity. Verify SOMETHING
	// indicating work appears AFTER the tool entry.
	if !strings.Contains(got, "thinking") {
		t.Errorf("expected 'thinking' activity in trailing line, got:\n%s", got)
	}
}

// When not streaming, the trailing line must NOT appear — it would
// misleadingly imply work is still happening.
func TestChatView_TrailingActivityHiddenWhenNotStreaming(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(false)
	entries := []*Entry{
		{Role: RoleUser, Content: "do it"},
		{Tool: &ToolEntry{ToolName: "Read", Status: ToolStatusComplete, Duration: 5 * time.Millisecond, Folded: true}},
	}
	c.SetEntriesSlice(entries)
	c.SetEntries(entries)
	got := stripAnsiCSI(strings.Join(c.PlainLines(), "\n"))
	if strings.Contains(got, "thinking") {
		t.Errorf("trailing 'thinking' line should not appear when not streaming, got:\n%s", got)
	}
}

func TestChatView_TrailingActivityReservePreventsMidTurnBounce(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	entries := []*Entry{
		{Role: RoleUser, Content: "do it"},
		{Role: RoleAssistant, Content: "First I will inspect the repo.", Streaming: true},
	}
	c.SetEntriesSlice(entries)

	stale := time.Now()
	c.SetAnimationTime(stale)
	c.lastTokenAt = stale.Add(-2 * staleStreamThreshold)
	c.SetEntries(entries)
	if !c.IsBetweenPhases() {
		t.Fatalf("setup: stale streaming entry should show trailing activity")
	}
	shownLines := c.TotalLineCount()
	shown := stripAnsiCSI(strings.Join(c.PlainLines(), "\n"))
	if !strings.Contains(shown, "thinking") {
		t.Fatalf("setup: trailing activity should be visible, got:\n%s", shown)
	}

	fresh := stale.Add(time.Second)
	c.SetAnimationTime(fresh)
	c.lastTokenAt = fresh
	c.SetEntries(entries)
	if c.IsBetweenPhases() {
		t.Fatalf("setup: fresh token should hide trailing activity")
	}
	hiddenLiveLines := c.TotalLineCount()
	if hiddenLiveLines < shownLines {
		t.Fatalf("mid-turn hidden activity shrank transcript: got %d lines, want at least %d", hiddenLiveLines, shownLines)
	}
	hiddenLive := stripAnsiCSI(strings.Join(c.PlainLines(), "\n"))
	if strings.Contains(hiddenLive, "thinking") {
		t.Fatalf("hidden live reserve should be blank, not activity text, got:\n%s", hiddenLive)
	}

	entries = append(entries, &Entry{Tool: &ToolEntry{ToolName: "Read", Status: ToolStatusInProgress, StartedAt: fresh}})
	c.SetEntriesSlice(entries)
	c.SetEntries(entries)
	withNewContent := c.PlainLines()
	if len(withNewContent) == 0 || strings.TrimSpace(withNewContent[len(withNewContent)-1]) == "" {
		t.Fatalf("new visible content should consume the reserve instead of leaving a blank tail, got:%q", withNewContent)
	}

	entries[1].Streaming = false
	entries[2].Tool.Status = ToolStatusComplete
	c.SetStreaming(false)
	c.SetEntries(entries)
	settledLines := c.PlainLines()
	if len(settledLines) == 0 || strings.TrimSpace(settledLines[len(settledLines)-1]) == "" {
		t.Fatalf("completed turn should not retain a reserved blank tail, got:%q", settledLines)
	}
	settled := stripAnsiCSI(strings.Join(settledLines, "\n"))
	if strings.Contains(settled, "thinking") {
		t.Fatalf("completed turn should not retain trailing activity text, got:\n%s", settled)
	}
}

// Advancing animation time must create the activity row even when no new
// content arrives to trigger a transcript rebuild.
func TestChatView_QuietStreamCreatesTrailingActivity(t *testing.T) {
	p := theme.Cracker()
	c := newChatView(theme.NewStyles(p), p, "", "", 100, 20)
	c.SetStreaming(true)
	entries := []*Entry{{Role: RoleAssistant, Content: "Inspecting the repo.", Streaming: true}}
	c.SetEntriesSlice(entries)
	now := time.Now()
	c.SetAnimationTime(now)
	c.lastTokenAt = now
	c.SetEntries(entries)
	if strings.Contains(stripAnsiCSI(c.View()), "thinking") {
		t.Fatal("fresh stream shows activity")
	}
	c.SetAnimationTime(now.Add(staleStreamThreshold + time.Millisecond))
	quiet := stripAnsiCSI(c.View())
	t.Logf("quiet: between=%t activity=%t", c.IsBetweenPhases(), strings.Contains(quiet, "thinking"))
	// Control probe: a queue-triggered rebuild must not be necessary.
	c.Enqueue("next prompt", nil)
	c.SetEntries(entries)
	if !strings.Contains(stripAnsiCSI(c.View()), "thinking") {
		t.Fatal("queue-triggered rebuild lacks activity")
	}
	if !strings.Contains(quiet, "thinking") {
		t.Fatal("quiet stream lacks activity until layout rebuild")
	}
}

func TestQuietStreamAnimationTickLifecycle(t *testing.T) {
	for _, child := range []bool{false, true} {
		name := "main"
		if child {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			m := New(nil, false)
			m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
			if child {
				m.applySubAgentEvent(subAgentEventMsg{id: "child", kind: "started"})
				m.chatTabs.active = "child"
			} else {
				m.streaming = true
			}
			c := m.activeChat()
			c.SetStreaming(true)
			now := time.Now()
			c.SetAnimationTime(now)
			c.SetEntriesSlice([]*Entry{{Role: RoleAssistant, Content: "Inspecting the repo.", Streaming: true}})
			c.lastTokenAt = now
			m.refreshViewport()
			if m.ensureAnimTick() == nil || !m.animTickActive {
				t.Fatal("tick did not start")
			}
			if m.ensureAnimTick() != nil {
				t.Fatal("duplicate tick")
			}
			// Deliver the scheduled tick with deterministic time past the threshold.
			next, cmd := m.Update(progressAnimTickMsg(now.Add(staleStreamThreshold + time.Millisecond)))
			m = next.(Model)
			if cmd == nil || !m.animTickActive {
				t.Fatal("quiet active chat lost tick")
			}
			if !strings.Contains(stripAnsiCSI(m.View().Content), "thinking") {
				t.Fatal("tick did not reveal quiet-stream activity")
			}
			if !strings.Contains(stripAnsiCSI(c.View()), "thinking") {
				t.Fatal("activity missing from chat")
			}
			// Queue changes must neither be needed for activity nor cancel it.
			c.Enqueue("next prompt", nil)
			m.relayout()
			if !strings.Contains(stripAnsiCSI(m.View().Content), "thinking") {
				t.Fatal("enqueue hid activity")
			}
			c.DrainNext()
			m.relayout()
			if !strings.Contains(stripAnsiCSI(m.View().Content), "thinking") {
				t.Fatal("draining queue hid activity")
			}
			// Fresh text must remove the old activity unit without waiting for a
			// separate full-transcript refresh, while retaining reserved tail rows.
			c.lastTokenAt = now.Add(staleStreamThreshold + time.Millisecond)
			if strings.Contains(stripAnsiCSI(c.View()), "thinking") {
				t.Fatal("fresh stream retained stale activity")
			}
			c.SetStreaming(false)
			c.entries[0].Streaming = false
			if child {
				m.closeSubAgentTab("child")
			} else {
				m.streaming = false
			}
			next, cmd = m.Update(progressAnimTickMsg(now.Add(time.Second)))
			m = next.(Model)
			if cmd != nil || m.animTickActive {
				t.Fatal("idle chat retained tick")
			}
		})
	}
}
