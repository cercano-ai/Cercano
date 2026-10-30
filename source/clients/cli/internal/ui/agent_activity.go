package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Activity is derived from existing child lifecycles, never the parent's turn or
// a timer. In particular, terminal/error tabs retained for review aren't live.
type agentActivityLink struct {
	id         string
	start, end int
}

func (m Model) agentActivityRow() (string, []agentActivityLink) {
	if m.contentPageActive() {
		return "", nil
	}
	var live []*chatTab
	for _, id := range m.chatTabs.order {
		tab := m.chatTabs.tabs[id]
		if id != mainChatTabID && tab != nil && !tab.done && !strings.HasPrefix(id, "activity:") {
			live = append(live, tab)
		}
	}
	if len(live) == 0 {
		return "", nil
	}
	text := m.styles.Success.Render(fmt.Sprintf("Agents %d: ", len(live)))
	// Permission gates are session-scoped: there is no reliable child attribution.
	// Don't imply every child is blocked or display a stale reconnect gate as live.
	if c := m.pendingConfirm; c != nil && !c.stale && (c.tool != nil || c.auth != nil) {
		text = m.styles.Info.Render("! Approval required | ") + text
	}
	text = strings.Repeat(" ", min(entryIndent, max(0, m.width))) + text
	var links []agentActivityLink
	shown := len(live)
	if shown > 2 {
		shown = 2
	}
	for i, tab := range live[:shown] {
		if i > 0 {
			text += m.styles.Info.Render(" · ")
		}
		status := "running"
		if tab.taskStatus == "accepted" {
			status = "queued"
		}
		label := strings.Join(strings.Fields(ansi.Strip(tab.title)), " ")
		if label == "" {
			label = "Agent"
		}
		suffix := ""
		if remaining := len(live) - i - 1; remaining > 0 {
			suffix = fmt.Sprintf(" +%d more", remaining)
		}
		budget := m.width - lipgloss.Width(text) - lipgloss.Width(suffix) - 2
		if budget < 4 {
			break
		}
		indicator := "○"
		if status == "running" {
			frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
			indicator = string(frames[m.agentActivityFrame])
		}
		text += m.styles.Info.Render(indicator + " ")
		label = ansi.Truncate(label+" ("+status+")", budget, "…")
		start := lipgloss.Width(text)
		text += m.styles.Info.Underline(true).Render(label)
		links = append(links, agentActivityLink{tab.id, start, lipgloss.Width(text)})
	}
	// The existing task pane tracks TaskNode checklist entries, not these
	// delegated child lifecycles. Keep overflow informational until there is a
	// task-list navigation target for this same population.
	if remaining := len(live) - len(links); remaining > 0 {
		text += m.styles.Info.Render(fmt.Sprintf(" +%d more", remaining))
	}
	return ansi.Truncate(text, max(0, m.width), ""), links
}

func (m Model) agentActivityHeight() int {
	row, _ := m.agentActivityRow()
	if row != "" {
		return 2
	}
	return 0
}

// Use the rendered frame, including spare padding and queued input, as the
// coordinate source just as prompt hit testing does.
func (m Model) agentActivityHit(x, y int) string {
	row, links := m.agentActivityRow()
	if row == "" {
		return ""
	}
	parts, _ := m.composeFrame()
	top := 0
	for _, part := range parts {
		if part == row && top == y {
			for _, link := range links {
				if x >= link.start && x < link.end {
					return link.id
				}
			}
		}
		top += countLines([]string{part})
	}
	return ""
}

// Only running children need repaint ticks. Queued children keep a static
// indicator, and hidden content pages need no activity-row animation.
func (m Model) hasRunningAgentActivity() bool {
	if m.contentPageActive() {
		return false
	}
	for _, id := range m.chatTabs.order {
		tab := m.chatTabs.tabs[id]
		if id != mainChatTabID && !strings.HasPrefix(id, "activity:") && tab != nil && !tab.done && tab.taskStatus != "accepted" {
			return true
		}
	}
	return false
}
