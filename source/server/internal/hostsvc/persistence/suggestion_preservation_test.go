package persistence

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeSuggestionPreservesCompletePrompt(t *testing.T) {
	for _, suggestion := range []string{
		strings.Repeat("x", 79),
		strings.Repeat("x", 80),
		"Run the focused regression tests for suggestion generation, transport, and CLI acceptance before checkpointing.",
		strings.Repeat("x", 79) + "界", // the old byte cap splits this rune
		strings.Repeat("界", 40),       // fewer than 80 characters, more than 80 bytes
	} {
		t.Run(suggestion, func(t *testing.T) {
			got := SanitizeSuggestion(suggestion)
			if got != suggestion || !utf8.ValidString(got) {
				t.Fatalf("got %q (%d bytes, valid UTF-8 %v), want complete %q (%d bytes)", got, len(got), utf8.ValidString(got), suggestion, len(suggestion))
			}
		})
	}
}
