package theme

import "testing"

func TestMarkdownStyleUsesPaletteColors(t *testing.T) {
	sc := MarkdownStyle(Cracker())
	if sc.Document.Color == nil || *sc.Document.Color != "#ea8212" {
		t.Fatalf("Document.Color = %v, want #ea8212", sc.Document.Color)
	}
	if sc.Code.Color == nil || *sc.Code.Color != "#b7a6e0" {
		t.Fatalf("Code.Color = %v, want #b7a6e0", sc.Code.Color)
	}
	if sc.Emph.Color == nil || *sc.Emph.Color != "#ea8212" {
		t.Fatalf("Emph.Color = %v, want #ea8212", sc.Emph.Color)
	}
	if sc.BlockQuote.Color == nil || *sc.BlockQuote.Color != "#ea8212" {
		t.Fatalf("BlockQuote.Color = %v, want #ea8212", sc.BlockQuote.Color)
	}
	if sc.Table.Color == nil || *sc.Table.Color != "#ea8212" {
		t.Fatalf("Table.Color = %v, want #ea8212", sc.Table.Color)
	}
	if sc.CodeBlock.BackgroundColor == nil || *sc.CodeBlock.BackgroundColor != "#1a1a1a" {
		t.Fatalf("CodeBlock.BackgroundColor = %v, want #1a1a1a", sc.CodeBlock.BackgroundColor)
	}
}

func TestCrackerMarkdownStyle_KeyColorsSet(t *testing.T) {
	sc := MarkdownStyle(Cracker())
	if sc.Heading.Color == nil || *sc.Heading.Color != "#ffb84d" {
		t.Fatalf("heading color = %v, want #ffb84d", sc.Heading.Color)
	}
	// Inline code uses the darker desaturated teal — distinct from links and
	// calmer than the vivid chrome cyan (see palette.go).
	if sc.Code.Color == nil || *sc.Code.Color != "#b7a6e0" {
		t.Fatalf("inline code color = %v, want #b7a6e0", sc.Code.Color)
	}
	// Links keep the brighter muted cyan — a different named color from code.
	if sc.Link.Color == nil || *sc.Link.Color != "#2ea8bc" {
		t.Fatalf("link color = %v, want #2ea8bc", sc.Link.Color)
	}
	if sc.Link.Underline == nil || !*sc.Link.Underline {
		t.Fatalf("link should be underlined")
	}
	if sc.Document.Margin == nil || *sc.Document.Margin != 0 {
		t.Fatalf("document margin = %v, want 0 (we add our own indent)", sc.Document.Margin)
	}
}

func TestMarkdownStyleBlockquoteSubtleAcrossThemes(t *testing.T) {
	for _, theme := range BuiltinThemes() {
		t.Run(theme.Name, func(t *testing.T) {
			p := theme.Palette
			sc := MarkdownStyle(p)
			if sc.BlockQuote.Indent == nil || *sc.BlockQuote.Indent != 2 || sc.BlockQuote.IndentToken == nil || *sc.BlockQuote.IndentToken != " " {
				t.Fatal("blockquote must use two single-space indent columns")
			}
			if sc.BlockQuote.Italic == nil || *sc.BlockQuote.Italic {
				t.Fatal("blockquote must not force italics")
			}
			if *sc.BlockQuote.Color != Hex(p.Primary) {
				t.Fatal("quote must retain normal foreground")
			}
			bg := Lerp(p.BgDeep, p.Primary, 0.035)
			if sc.BlockQuote.BackgroundColor == nil || *sc.BlockQuote.BackgroundColor != Hex(bg) {
				t.Fatal("quote must use a palette-derived subtle background")
			}
			base, tint := RGB(p.BgDeep), RGB(bg)
			for i := range base {
				delta := int(base[i]) - int(tint[i])
				if delta < -9 || delta > 9 {
					t.Fatalf("tint too strong: base=%v tint=%v", base, tint)
				}
			}
			if contrastRatio(p.Primary, bg) < contrastRatio(p.Primary, p.BgDeep)*0.9 {
				t.Fatal("quote tint significantly reduces foreground contrast")
			}
		})
	}
}
