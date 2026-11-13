package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestNextPromptSuggestionPreviewAndAcceptance(t *testing.T) {
	suggestion := "Run the focused regression tests for suggestion generation, transport, and CLI acceptance before checkpointing. 界"
	for _, width := range []int{20, 80, 160} {
		m := New(nil, false)
		m = send(t, m, tea.WindowSizeMsg{Width: width, Height: 24})
		m = send(t, m, nextPromptSuggestionMsg{suggestion: suggestion})
		if m.nextPromptSuggestion != suggestion || m.input.Suggestion != suggestion || m.input.Value() != "" {
			t.Fatal("receiving suggestion must preserve complete ghost text without typing it")
		}
		view := ansi.Strip(m.input.View())
		if lipgloss.Width(view) > m.input.width {
			t.Fatalf("preview exceeds input width: %q", view)
		}
		if m.input.textWidth() < lipgloss.Width(suggestion) {
			if !strings.Contains(view, "…") {
				t.Errorf("clipped preview must indicate hidden text: %q", view)
			}
		} else if !strings.Contains(view, suggestion) {
			t.Errorf("fitting preview must show complete suggestion: %q", view)
		}
		m = send(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.input.Value() != suggestion {
			t.Fatalf("Tab accepted %q, want complete %q", m.input.Value(), suggestion)
		}
		if m.nextPromptSuggestion != "" || m.input.Suggestion != "" {
			t.Fatal("Tab must clear the ghost after accepting")
		}
	}
}
