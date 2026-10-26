package ui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"cercano/source/clients/cli/internal/uiconfig"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
)

// Only reducers run: inert client identities never dial or execute commands.
func TestDevHostReconnectFencesQueuedSubscriptions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(dir, "ui.yaml"))
	t.Setenv("CERCANO_THEMES_DIR", filepath.Join(dir, "themes"))
	for _, kind := range []string{"task", "task error", "attachment", "attachment error"} {
		t.Run(kind, func(t *testing.T) {
			a := New(&agentclient.Client{}, false).WithDevHost(DevHostSelection{Conversation: "same"}, "test")
			a.ensureTaskSubscription()
			defer a.taskSubscriptionCancel()
			generation := a.taskSubscriptionGeneration
			event := &proto.TaskSubscriptionEvent{Payload: &proto.TaskSubscriptionEvent_Task{Task: &proto.TaskLifecycleSnapshot{TaskId: "old", ChildId: "old-child", Status: "running"}}}
			var saved tea.Msg
			switch kind {
			case "task":
				saved = taskSubscriptionMsg{parent: "same", generation: generation, update: agentclient.TaskUpdate{Event: event}}
			case "task error":
				saved = taskSubscriptionMsg{parent: "same", generation: generation, update: agentclient.TaskUpdate{Err: errors.New("old A disconnected")}}
			case "attachment":
				saved = conversationAttachmentMsg{parent: "same", generation: generation, update: agentclient.ConversationUpdate{Response: &proto.StreamProcessResponse{TurnEpoch: "old"}}}
			case "attachment error":
				saved = conversationAttachmentMsg{parent: "same", generation: generation, update: agentclient.ConversationUpdate{Err: errors.New("old A disconnected")}}
			}
			next, _ := a.Update(devHostResult{action: "reconnect", client: &agentclient.Client{}})
			b := next.(Model)
			b.Init() // construct startup commands, without RPCs
			b.ensureTaskSubscription()
			defer b.taskSubscriptionCancel()
			entries := len(b.mainChat().entries)
			state := b.connState
			turns := b.backgroundTurns
			after, _ := b.Update(saved)
			b = after.(Model)
			if len(b.mainChat().entries) != entries || b.chatTabs.tabs["old-child"] != nil || b.connState != state || !reflect.DeepEqual(turns, b.backgroundTurns) {
				t.Fatal("queued A message changed B UI/disconnect state")
			}
		})
	}
}

func TestDevHostPinnedConfigFreeze(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ui.yaml")
	t.Setenv("CERCANO_UI_CONFIG", path)
	t.Setenv("CERCANO_THEMES_DIR", filepath.Join(dir, "themes"))
	t.Setenv("XDG_CONFIG_HOME", dir)
	for _, pinned := range []bool{true, false} {
		for _, entry := range []string{"shortcut", "direct theme", "config entry"} {
			t.Run(map[bool]string{true: "pinned/", false: "normal/"}[pinned]+entry, func(t *testing.T) {
				before := []byte("# preserve exact shared preferences\ntheme: cr4k3r_j4x\n")
				if err := os.WriteFile(path, before, 0600); err != nil {
					t.Fatal(err)
				}
				m := New(nil, false)
				if pinned {
					m = m.WithDevHost(DevHostSelection{Conversation: "same"}, "test")
				}
				switch entry {
				case "shortcut":
					next, _ := m.Update(tea.KeyPressMsg{Code: 'm', Mod: tea.ModSuper})
					m = next.(Model)
				case "config entry":
					m.openConfigSurface(configTabModels)
				case "direct theme":
					next, _ := m.Update(settingsThemeMsg{working: m.theme, persistName: "changed"})
					m = next.(Model)
				}
				if entry != "direct theme" && (m.configSurface != nil) == pinned {
					t.Error("incorrect config entry availability")
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if pinned && string(after) != string(before) {
					t.Error("pinned reducer changed shared UI config")
				}
				if !pinned && entry == "direct theme" && uiconfig.LoadActiveTheme() != "changed" {
					t.Error("normal theme persistence disabled")
				}
			})
		}
	}
}
