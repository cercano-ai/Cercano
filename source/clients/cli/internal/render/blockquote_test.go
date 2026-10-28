package render

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"cercano/source/clients/cli/internal/theme"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

func quoteLines(t *testing.T, out string, width, indent int) []string {
	t.Helper()
	plain := ansi.Strip(out)
	if strings.ContainsAny(plain, "│┃▎>") {
		t.Fatalf("quote markers remain: %q", plain)
	}
	var lines []string
	for _, line := range strings.Split(plain, "\n") {
		if ansi.StringWidth(line) > width {
			t.Fatalf("line exceeds width %d: %q", width, line)
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, strings.Repeat(" ", indent)) {
			t.Fatalf("lost %d-column indent: %q", indent, line)
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	if len(lines) == 0 {
		t.Fatal("quote content lost")
	}
	return lines
}

func TestMarkdown_BlockquoteWrapping(t *testing.T) {
	for _, depth := range []int{1, 2, 3} {
		// Include one content column, long words, and widths that used to trigger
		// the parent's second wrap (and lose continuation indentation).
		for _, width := range []int{2*depth + 1, 2*depth + 2, 8, 12, 24} {
			t.Run(fmt.Sprintf("depth%d/width%d", depth, width), func(t *testing.T) {
				source := strings.Repeat("> ", depth) + "aa bb cc dd ee ff superlongword"
				md := NewMarkdown(theme.MarkdownStyle(theme.Cracker()))
				out := md.Render(source, width)
				lines := quoteLines(t, out, width, 2*depth)
				if len(lines) < 2 {
					t.Fatalf("expected wrapped quote: %q", out)
				}
				if strings.ReplaceAll(strings.Join(lines, ""), " ", "") != "aabbccddeeffsuperlongword" {
					t.Fatalf("lost or duplicated content: %q", lines)
				}
				if live := md.RenderLive(source, width); live != out {
					t.Fatal("live and cached quote rendering differ")
				}
				if cached := md.Render(source, width); cached != out {
					t.Fatal("cached quote rendering differs")
				}
			})
		}
	}
}

func TestMarkdown_BlockquoteNestedAndParagraphs(t *testing.T) {
	out := NewMarkdown(theme.MarkdownStyle(theme.Cracker())).Render("> outer\n>\n> > nested words that wrap across lines\n>\n> last\n\nafter", 16)
	plain := ansi.Strip(out)
	for _, line := range strings.Split(plain, "\n") {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		indent := 4
		switch text {
		case "outer", "last":
			indent = 2
		case "after":
			indent = 0
		}
		got := len(line) - len(strings.TrimLeft(line, " "))
		if got != indent {
			t.Fatalf("indent=%d, want %d: %q", got, indent, line)
		}
		if ansi.StringWidth(line) > 16 {
			t.Fatalf("overwide line: %q", line)
		}
	}
}

func TestMarkdown_BlockquoteInlineStyles(t *testing.T) {
	out := NewMarkdown(theme.MarkdownStyle(theme.Cracker())).Render("> normal **bold** *emphasis* `code` [link](https://example.com)", 80)
	quoteLines(t, out, 80, 2)
	for word, want := range map[string]string{
		"normal":   "38;2;234;130;18;48;2;33;29;25",
		"bold":     "38;2;255;184;77;48;2;33;29;25;1",
		"emphasis": "38;2;234;130;18;48;2;33;29;25;3",
		"code":     "38;2;183;166;224;48;2;33;29;25",
		"link":     "38;2;46;168;188;48;2;33;29;25",
	} {
		re := regexp.MustCompile(`\x1b\[([0-9;]*)m` + word)
		match := re.FindStringSubmatch(out)
		if len(match) < 2 || match[1] != want {
			t.Errorf("%s style=%q, want %q", word, match, want)
		}
	}
	// The quote background must not leak into the following ordinary paragraph.
	following := NewMarkdown(theme.MarkdownStyle(theme.Cracker())).Render("> quote\n\nafter", 40)
	if !strings.Contains(following, "\x1b[38;2;234;130;18mafter") {
		t.Fatalf("quote style leaked: %q", following)
	}
}

func TestMarkdown_BlockquoteNoColor(t *testing.T) {
	out := NewMarkdown(theme.MarkdownStyle(theme.Cracker())).Render("> aa bb cc dd ee ff", 8)
	// Bubble Tea's output profile performs color downsampling/removal; quotes
	// need no colored rail or background to remain recognizable in plain text.
	var buf bytes.Buffer
	writer := colorprofile.Writer{Forward: &buf, Profile: colorprofile.NoTTY}
	if _, err := writer.WriteString(out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("escape remains in no-color output: %q", buf.String())
	}
	lines := quoteLines(t, buf.String(), 8, 2)
	if strings.Join(lines, " ") != "aa bb cc dd ee ff" {
		t.Fatalf("no-color content changed: %q", lines)
	}
}
