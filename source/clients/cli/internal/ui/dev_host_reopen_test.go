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

// Reconnect can attach A while B remains the default for explicit reopens.
// Startup hydration must read A directly, not launch a second resume on B.
func TestDevHostStartupHydratesAttachedHostNotDefault(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	b := DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "B"}, Conversation: "fresh"}
	a := DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "A"}, Conversation: "existing"}
	m := New(nil, false).WithDevHost(b, "test")
	result, _ := m.applyDevHost(devHostResult{action: "reconnect", selection: &a, client: &agentclient.Client{}})
	attached := result.(Model)
	hydrated, cmd := attached.Update(devHostHydrateMsg{})
	n := hydrated.(Model)
	if cmd == nil || !n.resumeHydrating || n.resumeGen != attached.resumeGen+1 || n.convID != "existing" || n.devHost.busy {
		t.Fatalf("startup did not hydrate attached A: conv=%s hydrating=%v gen=%d busy=%v", n.convID, n.resumeHydrating, n.resumeGen, n.devHost.busy)
	}
	if n.devHost.selection.Identity.Host != "A" || n.devHost.defaultSelection.Identity.Host != "B" {
		t.Fatal("hydration changed destination identity")
	}
	// A user history-picker action remains an explicit reopen on default B.
	reopened, cmd := attached.beginProgressiveResume("existing")
	if cmd == nil || !reopened.devHost.busy || reopened.resumeGen != attached.resumeGen {
		t.Fatal("explicit reopen bypassed lifecycle destination routing")
	}
}

func TestDevHostStartupDoesNotHydrateDetachedOwner(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	m := New(&agentclient.Client{}, false).WithDevHost(DevHostSelection{Conversation: "retained"}, "test")
	m.devHost.detached = true
	result, _ := m.Update(devHostHydrateMsg{})
	if result.(Model).resumeHydrating || result.(Model).resumeGen != m.resumeGen {
		t.Fatal("startup hydration attached a detached owner")
	}
}
