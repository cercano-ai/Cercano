package ui

import (
	"testing"

	"cercano/source/server/pkg/agentclient"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func TestConfigPendingConfirmOwnsTyping(t *testing.T) {
	for _, kind := range []string{"tool", "auth", "plan"} {
		for _, key := range "yncd" {
			t.Run(kind+"/"+string(key), func(t *testing.T) {
				m := New(nil, false)
				m.width, m.height = 100, 30
				m.relayout()
				d := newCatalogTestDashboard(runtimeDashboardSnapshot{Config: &agentclient.Config{}, Catalog: agentclient.RuntimeModelCatalog{Models: []agentclient.RuntimeModel{{ID: "llama_server:catalog:qwen", Runtime: "llama_server", Source: "catalog", DisplayName: "Qwen"}}}})
				d.mode = dashboardModeModels
				d.focus = runtimeFocusActions
				d.catalogSearch = textinput.New()
				m.content = d
				m.configSurface = &configSurface{active: configTabModels, focused: true}
				var gate *confirmRequest
				switch kind {
				case "auth":
					gate = authenticationConfirm(agentclient.AuthenticationRequired{})
				case "plan":
					gate = toolConfirm(&pendingToolCall{Name: "request_plan_approval"})
				default:
					gate = toolConfirm(&pendingToolCall{Name: "edit_file", ToolUseID: "pending-tool", Args: `{"path":"a.go"}`})
				}
				yes, no := 0, 0
				gate.onYes = func(m Model) (Model, tea.Cmd) { yes++; m.pendingConfirm = nil; return m, nil }
				gate.onNo = func(m Model) (Model, tea.Cmd) { no++; m.pendingConfirm = nil; return m, nil }
				m.pendingConfirm = gate
				queued := &confirmRequest{}
				m.confirmQueue = []*confirmRequest{queued}
				entries := len(m.mainChat().Entries())
				press := func(msg tea.KeyPressMsg) { next, _ := m.Update(msg); m = next.(Model) }
				press(tea.KeyPressMsg{Code: key, Text: string(key)})
				if got := d.catalogSearch.Value(); got != string(key) {
					t.Errorf("config filter = %q, want %q", got, string(key))
				}
				if len(m.mainChat().Entries()) != entries {
					t.Error("config key appended confirmation details or status")
				}
				if m.pendingConfirm != gate || yes != 0 || no != 0 || m.composeToolUseID != "" {
					t.Fatalf("config typing changed confirmation: pending=%v yes=%d no=%d", m.pendingConfirm == gate, yes, no)
				}
				press(tea.KeyPressMsg{Code: tea.KeyEscape})
				if m.configSurface == nil || !m.configSurface.focused {
					t.Fatal("Escape must return to tab strip")
				}
				press(tea.KeyPressMsg{Code: tea.KeyRight})
				if m.configSurface.active != configTabMcp {
					t.Fatal("Right must navigate config tabs")
				}
				press(tea.KeyPressMsg{Code: tea.KeyEscape})
				if m.configSurface != nil || m.content != nil {
					t.Fatal("Escape must close config")
				}
				if m.pendingConfirm != gate || len(m.confirmQueue) != 1 || yes != 0 || no != 0 {
					t.Fatal("navigation/close resolved background confirmation")
				}
				press(tea.KeyPressMsg{Code: 'y', Text: "y"})
				if yes != 1 || no != 0 || m.pendingConfirm != queued {
					t.Fatal("confirmation must resume normally after config closes")
				}
			})
		}
	}
}

// Ctrl+C remains an explicit global shortcut, not a background auth rejection.
func TestConfigPendingAuthPreservesGlobalCtrlC(t *testing.T) {
	m := New(nil, false)
	m.content = newCatalogTestDashboard(runtimeDashboardSnapshot{Config: &agentclient.Config{}})
	m.configSurface = &configSurface{active: configTabModels, focused: true}
	gate := authenticationConfirm(agentclient.AuthenticationRequired{})
	m.pendingConfirm = gate
	next, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	got := next.(Model)
	if !got.ctrlCArmed || got.pendingConfirm != gate || got.configSurface == nil {
		t.Fatal("Ctrl+C must retain its global quit-arm behavior without rejecting authentication")
	}
}
