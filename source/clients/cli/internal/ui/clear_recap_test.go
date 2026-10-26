package ui

import (
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/pkg/agentclient"
	tea "charm.land/bubbletea/v2"
)

func TestClearRemovesRecapAndRestoresViewport(t *testing.T) {
	for _, tc := range []struct {
		name    string
		recap   string
		wrapped bool
	}{
		{name: "normal", recap: "wired the badge"},
		{name: "unavailable", recap: recapDisplay(agentclient.ConversationInfo{TurnCount: recapUnavailableMinTurns})},
		{name: "wrapped", recap: "refactored the telemetry collector to guard Emit against a closed channel, added a race regression test, and rebuilt the server", wrapped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(nil, false)
			m = m.SeedAssistantMarkdown("old conversation reply\n")
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 30})
			oldID := m.convID
			baseline := m.mainChat().Height()
			m = send(t, m, recapLoadedMsg{convID: oldID, recap: tc.recap})
			if n := len(m.recapLines()); n == 0 || (tc.wrapped && n < 2) {
				t.Fatalf("invalid recap setup: %d rendered lines", n)
			}
			if m.mainChat().Height() >= baseline {
				t.Fatal("recap did not reserve viewport space")
			}

			// /clear is a local switch, not a request to delete stored history.
			// It must work without an agent and schedule no backend mutation.
			next, cmd := m.runSlash("/clear")
			if cmd != nil {
				t.Fatal("clear unexpectedly scheduled a backend command")
			}
			m = next.(Model)
			if m.convID == oldID || m.convRef.id != m.convID {
				t.Fatal("clear did not switch conversation identity")
			}
			if m.recap != "" || len(m.recapLines()) != 0 || len(m.mainChat().Entries()) != 0 {
				t.Fatal("clear left old recap or transcript visible")
			}
			if got := m.mainChat().Height(); got != baseline {
				t.Fatalf("clear viewport height = %d, want baseline %d", got, baseline)
			}

			// A pre-clear poll cannot bring the old recap back.
			m = send(t, m, recapLoadedMsg{convID: oldID, recap: tc.recap})
			if m.recap != "" || m.mainChat().Height() != baseline {
				t.Fatal("late old recap restored text or changed layout")
			}

			// Once the new conversation has a recap, neither a successful old
			// poll nor an empty/error result may overwrite it or relayout.
			m = send(t, m, recapLoadedMsg{convID: m.convID, recap: "new recap"})
			newHeight := m.mainChat().Height()
			for _, stale := range []string{tc.recap, ""} {
				m = send(t, m, recapLoadedMsg{convID: oldID, recap: stale})
				if m.recap != "new recap" || m.mainChat().Height() != newHeight {
					t.Fatal("stale result overwrote new recap or changed layout")
				}
			}
			m = send(t, m, recapLoadedMsg{convID: m.convID, recap: ""})
			if m.recap != "" || m.mainChat().Height() != baseline {
				t.Fatal("matching empty result did not clear recap and restore layout")
			}
		})
	}
}

func TestFetchRecapCarriesConversationIdentity(t *testing.T) {
	if msg := fetchRecap(nil, "")().(recapLoadedMsg); msg.convID != "" || msg.recap != "" {
		t.Fatalf("empty conversation result = %+v", msg)
	}
	metadata := filepath.Join(t.TempDir(), "conversation.json")
	client := devResumeClient(t, metadata)
	// The missing metadata causes the RPC error path.
	if msg := fetchRecap(client, "old")().(recapLoadedMsg); msg.convID != "old" || msg.recap != "" {
		t.Fatalf("failed fetch lost identity: %+v", msg)
	}
	const stored = `{"id":"old","recap":"saved recap"}`
	if err := os.WriteFile(metadata, []byte(stored), 0600); err != nil {
		t.Fatal(err)
	}
	if msg := fetchRecap(client, "old")().(recapLoadedMsg); msg.convID != "old" || msg.recap != "saved recap" {
		t.Fatalf("successful fetch = %+v", msg)
	}
}
