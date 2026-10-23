package ui

import (
	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func TestUpdateReadinessPreservesDecisionsAndDraft(t *testing.T) {
	for _, auth := range []bool{false, true} {
		m := New(nil, false)
		m.input.SetValue("unsent draft")
		pending := &confirmRequest{title: "pending permission"}
		if auth {
			pending.auth = &agentclient.AuthenticationRequired{}
		}
		m.pendingConfirm = pending
		token := strings.Repeat("a", 32)
		type nextEvent struct{}
		next := func() tea.Msg { return nextEvent{} }
		for _, phase := range []string{"DRAINING", "IDLE", "RELEASED"} {
			updated, cmd := m.Update(updateReadinessMsg{notice: agentclient.UpdateReadiness{OperationToken: token, Phase: phase}, next: next})
			m = updated.(Model)
			if m.pendingConfirm != pending || m.input.Value() != "unsent draft" || m.tuiUpdateAvailable {
				t.Fatal("notice displaced local state")
			}
			if cmd == nil {
				t.Fatal("lost subscription")
			}
			if _, ok := cmd().(nextEvent); !ok {
				t.Fatal("notice triggered action")
			}
			if phase != "RELEASED" && !strings.Contains(m.renderUpdateReadiness(), "exit voluntary") {
				t.Fatal("missing non-authoritative display")
			}
		}
		if m.renderUpdateReadiness() != "" {
			t.Fatal("release not cleared")
		}
		updated, _ := m.Update(updateReadinessMsg{notice: agentclient.UpdateReadiness{OperationToken: token, Phase: "IDLE"}})
		m = updated.(Model)
		updated, _ = m.Update(updateReadinessMsg{notice: agentclient.UpdateReadiness{OperationToken: strings.Repeat("b", 32), Phase: "RELEASED"}})
		m = updated.(Model)
		if m.updateReadiness.OperationToken != token {
			t.Fatal("wrong owner cleared")
		}
		updated, cmd := m.Update(updateReadinessMsg{})
		m = updated.(Model)
		if m.renderUpdateReadiness() != "" || cmd != nil || m.pendingConfirm != pending || m.input.Value() != "unsent draft" {
			t.Fatal("disconnect altered local state")
		}
	}
}
