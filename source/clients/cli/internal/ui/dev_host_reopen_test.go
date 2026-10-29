package ui

import (
	"cercano/source/server/pkg/agentclient"
	"path/filepath"
	"testing"
)

func TestDevHostOrdinaryReopenStartsCommand(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	m := New(&agentclient.Client{}, false).WithDevHost(DevHostSelection{Conversation: "new"}, "test")
	_, cmd := m.beginProgressiveResume("original")
	if cmd == nil {
		t.Fatal("ordinary reopen rejected instead of starting bounded lifecycle command")
	}
}
