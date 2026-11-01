package ui

import (
	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedStartupParity(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	ordinary := New(nil, false).Init()().(tea.BatchMsg)
	pinned := New(nil, false).WithDevHost(DevHostSelection{Conversation: "saved"}, "test").Init()().(tea.BatchMsg)
	if len(pinned) != len(ordinary)+1 {
		t.Fatalf("pinned commands=%d ordinary=%d", len(pinned), len(ordinary))
	}
}
func TestResumeClearsPreviousTitle(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	m := New(&agentclient.Client{}, false)
	m.sessionTitle = "old"
	next, _ := m.Update(resumeRequestedMsg{ConversationID: "new"})
	if got := next.(Model).sessionTitle; got != "" {
		t.Fatalf("title=%q", got)
	}
}

func TestResumeMetadataFencesAndEmptyTitle(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	client := &agentclient.Client{}
	m := New(client, false)
	m.convID = "saved"
	m.resumeGen = 4
	m.titleRevision = 2
	m.sessionTitle = "renamed"
	base := resumeMetadataMsg{client: client, convID: "saved", gen: 4, titleRevision: 2, info: agentclient.ConversationInfo{Title: "persisted"}}
	for _, kind := range []string{"generation", "client", "conversation", "rename"} {
		msg := base
		switch kind {
		case "generation":
			msg.gen--
		case "client":
			msg.client = &agentclient.Client{}
		case "conversation":
			msg.convID = "other"
		case "rename":
			msg.titleRevision--
		}
		next, _ := m.Update(msg)
		if next.(Model).sessionTitle != "renamed" {
			t.Fatalf("accepted stale %s", kind)
		}
	}
	next, _ := m.Update(base)
	m = next.(Model)
	if m.sessionTitle != "persisted" {
		t.Fatal("lost metadata title")
	}
	base.info.Title = ""
	next, _ = m.Update(base)
	if next.(Model).sessionTitle != "" {
		t.Fatal("empty title did not clear")
	}
}

func TestPinnedRebuildPreservesTitleAndFencesHydration(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	m := New(nil, false).WithDevHost(DevHostSelection{Conversation: "saved"}, "test")
	m.sessionTitle = "picker title"
	m.resumeGen = 8
	next, _ := m.applyDevHost(devHostResult{action: "reopen", client: &agentclient.Client{}})
	n := next.(Model)
	if n.sessionTitle != m.sessionTitle || n.resumeGen <= m.resumeGen {
		t.Fatal("lost title or hydration fence")
	}
}

// Exercise the actual command/RPC/reducer chain, not manually fabricated tail
// events. The provider is test-owned gRPC with disk metadata and no inference.
func TestOrdinaryResumeCommandTransport(t *testing.T) {
	for _, entry := range []string{"picker", "slash"} {
		t.Run(entry, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
			metadata := filepath.Join(root, "conversation.json")
			if err := os.WriteFile(metadata, []byte(`{"id":"saved","title":"Saved title"}`), 0600); err != nil {
				t.Fatal(err)
			}
			client := devResumeClient(t, metadata)
			m := New(client, false)
			var command tea.Cmd
			if entry == "picker" {
				next, cmd := m.Update(resumeRequestedMsg{ConversationID: "saved"})
				m = next.(Model)
				command = cmd
			} else {
				next, cmd := m.runSlash("/resume saved")
				m = next.(Model)
				command = cmd
			}
			var consume func(tea.Msg)
			consume = func(msg tea.Msg) {
				switch msg := msg.(type) {
				case tea.BatchMsg:
					for _, cmd := range msg {
						if cmd != nil {
							consume(cmd())
						}
					}
				case resumeMetadataMsg:
					next, _ := m.Update(msg)
					m = next.(Model)
				case resumeViewportStreamMsg:
					m, _ = m.applyProgressiveResumeEvent(msg)
					if !msg.done && msg.ch != nil {
						consume(waitProgressiveResumeCmd(msg.ch)())
					}
				}
			}
			consume(command())
			if m.sessionTitle != "Saved title" || len(m.inputHistory) != 1 || m.resumeHydrating || m.resumeBackfilling {
				t.Fatalf("incomplete hydration: title=%q history=%v hydrating=%v", m.sessionTitle, m.inputHistory, m.resumeHydrating)
			}
		})
	}
}
