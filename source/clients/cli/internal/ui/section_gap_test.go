package ui

import (
	"fmt"
	"strings"
	"testing"
)

func sectionGapPlain(s string) string {
	lines := strings.Split(plain(s), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.Join(lines, "\n")
}

func TestAssistantMarkdown_QuoteTrailingGap(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, quote := range []string{"> quote", "> quote\nlazy continuation", "> > nested", "intro\n> quote"} {
			for _, following := range []string{"after", "## heading", "- item", "> next quote", "```text\ncode\n```", "| A | B |\n|---|---|\n| a | b |"} {
				for _, ending := range []string{"", "\n", "\n\n"} {
					t.Run(fmt.Sprintf("stream=%v/%s/%s/%q", streaming, quote, following, ending), func(t *testing.T) {
						cv := newMdChatView()
						e := &Entry{Role: RoleAssistant, Streaming: streaming, Content: quote + "\n\n" + following + ending}
						for frame := 0; frame < 2; frame++ {
							out := sectionGapPlain(cv.renderAssistantMarkdown(e, 40))
							marker := "quote"
							if strings.Contains(quote, "continuation") {
								marker = "continuation"
							}
							if strings.Contains(quote, "nested") {
								marker = "nested"
							}
							idx := strings.Index(out, marker)
							if idx < 0 {
								t.Fatalf("quote missing: %q", out)
							}
							rest := out[idx+len(marker):]
							if !strings.HasPrefix(rest, "\n\n") || strings.HasPrefix(rest, "\n\n\n") {
								t.Fatalf("want exactly one blank line after quote: %q", out)
							}
							indentedText := "  quote"
							if strings.Contains(quote, "nested") {
								indentedText = "    nested"
							}
							if !strings.Contains(out, "\n"+indentedText) {
								t.Fatalf("indent lost: %q", out)
							}
						}
					})
				}
			}
		}
	}
}

func TestAssistantMarkdown_QuoteTerminalAndGrowingTail(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		cv := newMdChatView()
		e := &Entry{Role: RoleAssistant, Streaming: streaming}
		for _, ending := range []string{"", "\n", "\n\n"} {
			e.Content = "> quote" + ending
			out := sectionGapPlain(cv.renderAssistantMarkdown(e, 40))
			if strings.HasSuffix(out, "\n") {
				t.Fatalf("terminal blank added: %q", out)
			}
		}
		e.Content += "after"
		out := sectionGapPlain(cv.renderAssistantMarkdown(e, 40))
		if !strings.Contains(out, "  quote\n\nafter") {
			t.Fatalf("growing tail missing separation: %q", out)
		}
	}
}
