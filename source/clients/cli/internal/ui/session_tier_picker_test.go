package ui

import (
	"fmt"
	"strings"
	"testing"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func tierPickerModel() Model {
	m := New(nil, false)
	m.width = 120
	m.height = 36
	m.cloudModel = "gpt-6-astra"
	m.activeCloudProfile = "openai"
	m.primarySlotModel = "gpt-6-astra"
	m.sessionModelConv = m.convID
	m.sessionModelState = agentclient.SessionModelStatus{Effective: &agentclient.SessionModelRoute{Profile: "openai", Model: "gpt-6-astra"}, Tiers: []agentclient.SessionModelTier{{Tier: "premium", Model: "gpt-6-astra"}, {Tier: "standard", Model: "gpt-6.1-sol"}, {Tier: "economy", Model: "gpt-6-luna"}}}
	m.sessionTierPicker = &sessionTierPicker{convID: m.convID, status: m.sessionModelState}
	return m
}

func TestSessionTierPickerMouseAndKeys(t *testing.T) {
	m := tierPickerModel()
	left, right, ok := m.cloudChipRange()
	if !ok || left >= right {
		t.Fatal("chip hit range missing")
	}
	m.sessionTierPicker = nil
	next, cmd := m.Update(tea.MouseClickMsg{X: left, Y: 0, Button: tea.MouseLeft})
	m = next.(Model)
	if !m.activeSessionTierPicker() || cmd == nil {
		t.Fatal("chip not clickable")
	}
	next, _ = m.Update(sessionModelMsg{convID: m.convID, serial: m.sessionModelSerial, quiet: true, status: tierPickerModel().sessionModelState})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	box, x, y := m.sessionTierBox()
	if !strings.Contains(ansi.Strip(box), "› [standard : gpt-6.1-sol]") {
		t.Fatal("arrow selection missing", ansi.Strip(box))
	}
	next, cmd = m.Update(tea.MouseClickMsg{X: x + 3, Y: y + 3, Button: tea.MouseLeft})
	m = next.(Model)
	if cmd == nil || !m.sessionModelPending || m.sessionTierPicker.cursor != 2 {
		t.Fatal("economy click did not request change")
	}
	if !strings.Contains(ansi.Strip(m.renderHeader()), "1:gpt-6-astra") {
		t.Fatal("optimistic header changed before acknowledgement")
	}
	// A failure preserves the existing effective model and leaves a usable picker.
	next, _ = m.Update(sessionModelMsg{convID: m.convID, serial: m.sessionModelSerial, err: fmt.Errorf("rejected")})
	m = next.(Model)
	if m.sessionModelPending || m.sessionTierPicker.err != "rejected" || m.sessionModelState.Effective.Model != "gpt-6-astra" {
		t.Fatal("failure not retained")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.activeSessionTierPicker() {
		t.Fatal("Escape did not close")
	}
}

func TestSessionTierAcknowledgementAndStaleReplies(t *testing.T) {
	m := tierPickerModel()
	_ = m.requestSessionModel("tier", "openai", "standard", false)
	state := m.sessionModelState
	route := agentclient.SessionModelRoute{Profile: "openai", Model: "gpt-6.1-sol"}
	state.Effective = &route
	state.Override = &route
	next, _ := m.Update(sessionModelMsg{convID: m.convID, serial: m.sessionModelSerial, status: state})
	m = next.(Model)
	if m.activeSessionTierPicker() || !strings.Contains(ansi.Strip(m.renderHeader()), "1:gpt-6-astra") {
		t.Fatal("successful override should close picker without changing configured slot")
	}
	// Both conversation and request generation protect against old responses.
	old := m.sessionModelSerial
	_ = m.requestSessionModel("status", "", "", true)
	for _, reply := range []sessionModelMsg{{convID: m.convID, serial: old, status: tierPickerModel().sessionModelState}, {convID: "other", serial: m.sessionModelSerial, status: tierPickerModel().sessionModelState}} {
		next, _ = m.Update(reply)
		m = next.(Model)
		if m.sessionModelState.Effective.Model != "gpt-6.1-sol" {
			t.Fatal("stale reply replaced override")
		}
	}
}

func TestSessionTierUnavailableAndCustomModels(t *testing.T) {
	m := tierPickerModel()
	m.sessionTierPicker.status.Tiers[0].Model = ""
	if m.selectSessionTier() != nil || m.sessionTierPicker.err == "" {
		t.Fatal("unavailable model sent")
	}
	m.sessionTierPicker.status.Effective = &agentclient.SessionModelRoute{Profile: "openai", Model: "custom"}
	m.sessionTierKey("tab")
	if m.sessionTierPicker.cursor != 1 {
		t.Fatal("Tab failed")
	}
	m.sessionTierKey("shift+tab")
	if m.sessionTierPicker.cursor != 0 {
		t.Fatal("Shift-Tab failed")
	}
	m.sessionTierClick(0, 30)
	if m.activeSessionTierPicker() {
		t.Fatal("outside click not dismissed")
	}
}

func TestSessionTierPendingRetainsQueuedTurn(t *testing.T) {
	m := tierPickerModel()
	m.mainChat().Enqueue("next", nil)
	_ = m.requestSessionModel("tier", "openai", "standard", false)
	if _, ok := m.drainQueuedChat(); ok || len(m.mainChat().Queued()) != 1 {
		t.Fatal("queued turn raced override acknowledgement")
	}
	next, _ := m.Update(sessionModelMsg{convID: m.convID, serial: m.sessionModelSerial, err: fmt.Errorf("rejected")})
	m = next.(Model)
	if len(m.mainChat().Queued()) != 1 {
		t.Fatal("failed override dispatched queued turn")
	}
}

func TestSessionTierNewConversationDoesNotDisplayOldOverrideWhileLoading(t *testing.T) {
	m := tierPickerModel()
	m.sessionModelState.Effective = &agentclient.SessionModelRoute{Profile: "previous", Model: "old-session-only"}
	m.convID = "new-conversation"
	_ = m.requestSessionModel("status", "", "", true)
	if strings.Contains(ansi.Strip(m.renderHeader()), "old-session-only") {
		t.Fatal("old override leaked into new session while loading")
	}
}
