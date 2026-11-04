package consumer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeSkill writes <root>/<name>/SKILL.md with the given frontmatter+body.
func makeSkill(t *testing.T, root, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(md, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return md
}

func skillFile(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + body
}

func TestDiscover_NoSkills(t *testing.T) {
	work, home := t.TempDir(), t.TempDir()
	if got := Discover(work, home); got != nil {
		t.Errorf("expected nil entries, got %+v", got)
	}
	if got := Catalog(Discover(work, home)); got != "" {
		t.Errorf("expected empty catalog, got %q", got)
	}
}

func TestDiscover_EmptyAndRelativeDirs(t *testing.T) {
	// Empty or relative workDir/home must contribute no roots — no implicit
	// process-cwd fallback. Run inside a directory that itself contains
	// skills, so a relative-path fallback would find them.
	base := t.TempDir()
	root := filepath.Join(base, ".agents", "skills")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	makeSkill(t, root, "alpha", skillFile("alpha", "A", "body"))

	t.Chdir(base)
	if got := Discover("", ""); got != nil {
		t.Errorf("empty dirs: expected no skills, got %+v", got)
	}
	if got := Discover(".agents", ""); got != nil {
		t.Errorf("relative workdir: expected no skills, got %+v", got)
	}
	if got := Discover("", "relative-home"); got != nil {
		t.Errorf("relative home: expected no skills, got %+v", got)
	}
	// Absolute discovery of the same tree still works.
	if got := Discover(base, ""); len(got) != 1 {
		t.Errorf("absolute workdir: expected 1 skill, got %+v", got)
	}
}

func TestDiscover_SortedAndComplete(t *testing.T) {
	work, home := t.TempDir(), t.TempDir()
	for _, name := range []string{"zeta", "mango", "apple"} {
		makeSkill(t, filepath.Join(work, ".claude", "skills"), name, skillFile(name, "d-"+name, "b"))
	}
	makeSkill(t, filepath.Join(home, ".agents", "skills"), "kiwi", skillFile("kiwi", "d-kiwi", "b"))
	got := Discover(work, home)
	if len(got) != 4 {
		t.Fatalf("expected 4 skills, got %d: %+v", len(got), got)
	}
	for i, want := range []string{"apple", "kiwi", "mango", "zeta"} {
		if got[i].Name != want {
			t.Errorf("position %d: want %q, got %q", i, want, got[i].Name)
		}
	}
	if got[0].Source != "workspace" || got[1].Source != "home" {
		t.Errorf("unexpected sources: %q / %q", got[0].Source, got[1].Source)
	}
}

func TestDiscover_Precedence(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	home := filepath.Join(base, "h")

	// Same name in all four roots: workspace .agents wins.
	mk := func(root string) {
		makeSkill(t, root, "dup", skillFile("dup", "from "+root, "b"))
	}
	mk(filepath.Join(home, ".claude", "skills"))
	mk(filepath.Join(home, ".agents", "skills"))
	mk(filepath.Join(work, ".claude", "skills"))
	mk(filepath.Join(work, ".agents", "skills"))
	got := Discover(work, home)
	if len(got) != 1 || got[0].Name != "dup" {
		t.Fatalf("expected single dup entry, got %+v", got)
	}
	want := filepath.Join(work, ".agents", "skills", "dup", "SKILL.md")
	if got[0].Path != want || got[0].Source != "workspace" {
		t.Errorf("want path %q (workspace), got %q (%s)", want, got[0].Path, got[0].Source)
	}

	// Workspace roots missing, .agents beats .claude in home.
	work2 := filepath.Join(base, "w2")
	got2 := Discover(work2, home)
	if len(got2) != 1 {
		t.Fatalf("expected 1 entry, got %+v", got2)
	}
	want2 := filepath.Join(home, ".agents", "skills", "dup", "SKILL.md")
	if got2[0].Path != want2 || got2[0].Source != "home" {
		t.Errorf("want home .agents, got %q (%s)", got2[0].Path, got2[0].Source)
	}
}

func TestDiscover_MixedPrecedence_InvalidHigherLoses(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	home := filepath.Join(base, "h")
	// Workspace copy is invalid (description mismatch with required fields):
	// a malformed higher-precedence copy must not shadow nothing — the name
	// still belongs to the workspace copy per "first valid wins"? No: an
	// invalid skill is not discovered at all, so the lower valid one shows.
	makeSkill(t, filepath.Join(work, ".agents", "skills"), "both",
		"---\nname: other\ndescription: mismatched\n---\nbody")
	makeSkill(t, filepath.Join(home, ".agents", "skills"), "both",
		skillFile("both", "valid desc", "body"))
	got := Discover(work, home)
	if len(got) != 1 || got[0].Name != "both" {
		t.Fatalf("expected valid home entry, got %+v", got)
	}
	if got[0].Source != "home" {
		t.Errorf("invalid workspace copy should not shadow valid home copy: %+v", got[0])
	}
}

func TestDiscover_Validation(t *testing.T) {
	work := t.TempDir()
	root := filepath.Join(work, ".agents", "skills")
	cases := []struct {
		name    string
		content string
	}{
		{"Bad_Name", "---\nname: Bad_Name\ndescription: d\n---\nb"},        // dir mismatch + bad chars
		{"UPPER", "---\nname: UPPER\ndescription: d\n---\nb"},              // uppercase
		{"-lead", "---\nname: -lead\ndescription: d\n---\nb"},              // leading hyphen
		{"trail-", "---\nname: trail-\ndescription: d\n---\nb"},            // trailing hyphen
		{"dou--ble", "---\nname: dou--ble\ndescription: d\n---\nb"},        // consecutive hyphen
		{"", "nameless file"},                                              // no frontmatter
		{"not-md-dir", "just a file"},                                      // SKILL.md is a dir
	}
	long := strings.Repeat("d", 65)
	cases = append(cases,
		[]struct{ name, content string }{
			{"toolongname", "---\nname: " + long + "\ndescription: d\n---\nb"},
			{"nodedesc", "---\nname: nodedesc\n---\nb"},
			{"emptydesc", "---\nname: emptydesc\ndescription: \"\"\n---\nb"},
			{"blankdesc", "---\nname: blankdesc\ndescription: \"   \\n  \"\n---\nb"},
		}...)
	over := strings.Repeat("d", MaxDescriptionRunes+1)
	cases = append(cases, struct{ name, content string }{
		"longdesc", "---\nname: longdesc\ndescription: " + over + "\n---\nb",
	})

	for _, c := range cases {
		if c.name != "" {
			makeSkill(t, root, c.name, c.content)
		}
	}
	if got := Discover(work, ""); len(got) != 0 {
		t.Errorf("expected all-invalid tree to yield nothing, got %+v", got)
	}
}

func TestDiscover_MalformedYAML(t *testing.T) {
	work := t.TempDir()
	root := filepath.Join(work, ".claude", "skills")
	makeSkill(t, root, "broken", "---\nname: broken\ndescription: [unclosed\n---\nbody")
	makeSkill(t, root, "tabbed", "---\n\tname: tabbed\ndescription: d\n---\nbody")
	if got := Discover(work, ""); len(got) != 0 {
		t.Errorf("expected malformed YAML skills skipped, got %+v", got)
	}
}

func TestDiscover_MultilineDescription(t *testing.T) {
	work := t.TempDir()
	root := filepath.Join(work, ".agents", "skills")
	content := "---\nname: multiline\ndescription: >\n  A folded\n  multiline description\n---\nbody"
	makeSkill(t, root, "multiline", content)
	got := Discover(work, "")
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %+v", got)
	}
	if !strings.Contains(got[0].Description, "multiline description") {
		t.Errorf("folded description lost: %q", got[0].Description)
	}
}

func TestDiscover_OversizeFileIgnored(t *testing.T) {
	work := t.TempDir()
	root := filepath.Join(work, ".agents", "skills")
	// Valid small skill, plus one whose SKILL.md exceeds MaxFileRead.
	makeSkill(t, root, "ok", skillFile("ok", "d", "b"))
	big := makeSkill(t, root, "big", skillFile("big", strings.Repeat("x", MaxFileRead), "b"))
	if fi, err := os.Stat(big); err != nil || fi.Size() <= MaxFileRead {
		t.Fatalf("test setup: big file size %d err %v", fi.Size(), err)
	}
	got := Discover(work, "")
	if len(got) != 1 || got[0].Name != "ok" {
		t.Errorf("oversize skill must be ignored entirely, got %+v", got)
	}
}

func TestDiscover_BinaryAndNonUTF8Ignored(t *testing.T) {
	work := t.TempDir()
	root := filepath.Join(work, ".agents", "skills")
	makeSkill(t, root, "nul", "---\nname: nul\ndescription: d\x00\n---\nbody")
	makeSkill(t, root, "latin", "---\nname: latin\ndescription: caf\xe9\n---\nbody")
	if got := Discover(work, ""); len(got) != 0 {
		t.Errorf("expected binary/non-UTF8 skills skipped, got %+v", got)
	}
}

func TestDiscover_SymlinksSkipped(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	home := filepath.Join(base, "h")

	// Symlinked root directory: never followed.
	realRoot := filepath.Join(base, "real-skills")
	makeSkill(t, realRoot, "real", skillFile("real", "d", "b"))
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, filepath.Join(home, ".claude", "skills")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	// Symlinked skill directory inside a real root.
	if err := os.MkdirAll(filepath.Join(work, ".agents", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, filepath.Join(work, ".agents", "skills", "linked")); err != nil {
		t.Fatal(err)
	}

	// Symlinked SKILL.md inside a real skill dir.
	other := filepath.Join(base, "other", "SKILL.md")
	if err := os.MkdirAll(filepath.Join(base, "other"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte(skillFile("linkedmd", "d", "b")), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(work, ".agents", "skills", "linkedmd")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, filepath.Join(target, "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	got := Discover(work, home)
	if len(got) != 0 {
		t.Errorf("symlinked roots/entries/files must be skipped, got %+v", got)
	}
}

func TestDiscover_Limits(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	home := filepath.Join(base, "h")
	// More than MaxSkills valid skills spread across roots; catalog must cap.
	for i := 0; i < MaxSkills+10; i++ {
		name := "s" + strings.Repeat("x", i) // unique lowercase names
		root := filepath.Join(work, ".agents", "skills")
		if i >= 5 {
			root = filepath.Join(home, ".claude", "skills")
		}
		makeSkill(t, root, name, skillFile(name, "d", "b"))
	}
	got := Discover(work, home)
	if len(got) != MaxSkills {
		t.Errorf("expected catalog capped at %d entries, got %d", MaxSkills, len(got))
	}
}

func TestDiscover_RootEntriesBounded(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	root := filepath.Join(work, ".agents", "skills")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	// Plain directories beyond the enumeration cap must not be examined.
	for i := 0; i < MaxRootEntries+10; i++ {
		if err := os.MkdirAll(filepath.Join(root, strings.Repeat("d", 30)+string(rune('a'+i%26))+string(rune('a'+i/26))), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got := Discover(work, "")
	if len(got) != 0 {
		t.Errorf("expected no skills (no SKILL.md anywhere), got %+v", got)
	}
}

func TestDiscover_TwoWorkDirsIsolated(t *testing.T) {
	base := t.TempDir()
	w1, w2 := filepath.Join(base, "one"), filepath.Join(base, "two")
	makeSkill(t, filepath.Join(w1, ".agents", "skills"), "first", skillFile("first", "d1", "b"))
	makeSkill(t, filepath.Join(w2, ".agents", "skills"), "second", skillFile("second", "d2", "b"))
	home := filepath.Join(base, "home")
	if got := Discover(w1, home); len(got) != 1 || got[0].Name != "first" {
		t.Errorf("w1 isolation: %+v", got)
	}
	if got := Discover(w2, home); len(got) != 1 || got[0].Name != "second" {
		t.Errorf("w2 isolation: %+v", got)
	}
	// Repeated discovery is stateless.
	if got := Discover(w1, home); len(got) != 1 || got[0].Name != "first" {
		t.Errorf("repeat discovery drifted: %+v", got)
	}
}

func TestCatalog_NoBodyLeakage(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	body := "SECRET-BODY-INSTRUCTIONS-RUN-RM-RF"
	desc := "metadata only"
	makeSkill(t, filepath.Join(work, ".agents", "skills"), "alpha", skillFile("alpha", desc, body))
	got := Discover(work, "")
	cat := Catalog(got)
	if strings.Contains(cat, body) {
		t.Error("catalog leaked skill body text")
	}
	if strings.Contains(cat, "SECRET-BODY") {
		t.Error("catalog leaked body fragment")
	}
	for _, want := range []string{"alpha", desc, "UNTRUSTED", "OPTIONAL", "read_file", "Supporting resources referenced by a skill", "registers no tools", "NOT included"} {
		if !strings.Contains(cat, want) {
			t.Errorf("catalog missing required content %q", want)
		}
	}
}

func TestCatalog_JSONEscapesUntrustedFields(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	// Hostile-ish description: quotes, backslashes, angle brackets, newlines.
	desc := "desc with \"quotes\", <script>alert(1)</script>, back\\slash"
	content := "---\nname: sneaky\ndescription: >\n  " + strings.ReplaceAll(desc, "\n", "\n  ") + "\n---\nbody"
	makeSkill(t, filepath.Join(work, ".agents", "skills"), "sneaky", content)
	got := Discover(work, "")
	if len(got) != 1 {
		t.Fatalf("expected skill discovered, got %+v", got)
	}
	cat := Catalog(got)
	var parsed []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
	}
	if err := json.Unmarshal([]byte(cat[strings.Index(cat, "["):]), &parsed); err != nil {
		t.Fatalf("catalog JSON invalid: %v\n%s", err, cat)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 parsed entry, got %d", len(parsed))
	}
	if parsed[0].Name != "sneaky" || parsed[0].Description != got[0].Description || parsed[0].Path != got[0].Path {
		t.Errorf("JSON round-trip mismatch: %+v vs %+v", parsed[0], got[0])
	}
	if strings.Contains(cat, "<script>") {
		t.Errorf("catalog contains raw structural characters:\n%s", cat)
	}
}

func TestCatalog_ByteBudget(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "w")
	root := filepath.Join(work, ".agents", "skills")
	bigDesc := strings.Repeat("d", MaxDescriptionRunes)
	for i := 0; i < MaxSkills; i++ {
		name := "e" + strings.Repeat("x", i)
		makeSkill(t, root, name, skillFile(name, bigDesc, "b"))
	}
	cat := Catalog(Discover(work, ""))
	if len(cat) > MaxCatalogBytes+len(bigDesc)+512 {
		t.Errorf("catalog exceeded budget: %d bytes", len(cat))
	}
	if !strings.HasPrefix(cat, catalogPreamble) {
		t.Error("catalog missing preamble")
	}
	// Small budget sanity: catalog of one small skill is well under it.
	small := Catalog([]Entry{{Name: "a", Description: "d", Path: "/tmp/a/SKILL.md", Source: "workspace"}})
	if len(small) > MaxCatalogBytes {
		t.Errorf("small catalog over budget: %d", len(small))
	}
}

func TestValidName(t *testing.T) {
	valid := []string{"a", "z9", "repo-doc-sync", strings.Repeat("x", 64)}
	for _, v := range valid {
		if !validName(v) {
			t.Errorf("expected valid: %q", v)
		}
	}
	invalid := []string{"", "A", "a_b", "-a", "a-", "a--b", strings.Repeat("x", 65), "a b", "café"}
	for _, v := range invalid {
		if validName(v) {
			t.Errorf("expected invalid: %q", v)
		}
	}
}
