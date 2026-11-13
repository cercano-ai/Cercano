package ui

import "strings"

// Reuse the former activity-list animation in the tab strip. The frame is owned
// by the shared progress tick, never by rendering or hit testing.
var workingAgentFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func (tab *chatTab) isWorkingAgent() bool {
	return tab != nil && tab.id != mainChatTabID && !strings.HasPrefix(tab.id, "activity:") && !tab.done && !tab.errored && tab.taskStatus != "accepted"
}

// Queued children and retained terminal tabs are static. Content pages hide the
// strip, so it must not keep the shared animation loop alive there.
func (m Model) hasWorkingAgentTabs() bool {
	if m.contentPageActive() {
		return false
	}
	for _, id := range m.chatTabs.order {
		if m.chatTabs.tabs[id].isWorkingAgent() {
			return true
		}
	}
	return false
}
