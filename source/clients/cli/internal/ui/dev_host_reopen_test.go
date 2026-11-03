package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"path/filepath"
	"testing"
)

func TestDevHostOrdinaryReopenStartsCommand(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	m := New(&agentclient.Client{}, false).WithDevHost(DevHostSelection{Conversation: "new"}, "test")
	_, cmd := m.applyResume("original")
	if cmd == nil {
		t.Fatal("ordinary reopen rejected instead of starting bounded lifecycle command")
	}
}

func TestResumeKeepsDefaultHostForFreshConversation(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	b := DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "B"}, Conversation: "fresh"}
	a := DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "A"}, Conversation: "existing"}
	m := New(nil, false).WithDevHost(b, "test")
	result, _ := m.applyDevHost(devHostResult{action: "reopen", selection: &a, client: &agentclient.Client{}})
	n := result.(Model)
	if n.devHost.selection.Identity.Host != "A" || n.devHost.defaultSelection.Identity.Host != "B" {
		t.Fatal("resume replaced fresh-conversation default")
	}
}
