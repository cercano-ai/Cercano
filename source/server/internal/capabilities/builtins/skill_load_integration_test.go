package builtins

// Integration coverage for skill loading: an external-style skill fixture is
// discovered via the real consumer.Discover, its metadata catalog is checked,
// and the discovered SKILL.md (plus a referenced supporting file) is loaded
// through the existing registered read_file capability.
//
// This is a deterministic filesystem integration test — it proves discovery
// and read-tool plumbing work end to end, not live-model behavior.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/skills/consumer"
)

// writeSkill creates an external-style skill directory with a SKILL.md that
// has valid frontmatter, a body containing relative-reference guidance, and a
// supporting file under references/.
func writeSkill(t *testing.T, skillDir string) (skillMDPath, refPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	skillMDPath = filepath.Join(skillDir, "SKILL.md")
	body := `# Review checklist

Use this checklist when reviewing a change.

## Steps

1. Check correctness of the diff.
2. Check tests exist for new behavior.

For the detailed per-file rubric, read the relative reference
references/checklist.md with the file-read tool before finalizing a verdict.
`
	md := "---\nname: review-checklist\ndescription: Checklist for reviewing code changes with a detailed rubric reference\n---\n" + body
	if err := os.WriteFile(skillMDPath, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	refPath = filepath.Join(skillDir, "references", "checklist.md")
	ref := "# Rubric\n\n- No unhandled error paths.\n- Public API changes documented.\n"
	if err := os.WriteFile(refPath, []byte(ref), 0o644); err != nil {
		t.Fatal(err)
	}
	return skillMDPath, refPath
}

func TestSkillDiscoverThenLoadWithReadFile(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir() // separate, so only the workspace root matches

	_, wantRefPath := writeSkill(t, filepath.Join(workDir, ".agents", "skills", "review-checklist"))

	// 1. Real discovery over the workspace roots.
	entries := consumer.Discover(workDir, homeDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 discovered entry, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Name != "review-checklist" {
		t.Errorf("name = %q", e.Name)
	}
	if e.Source != "workspace" {
		t.Errorf("source = %q, want workspace", e.Source)
	}
	if !filepath.IsAbs(e.Path) {
		t.Errorf("entry path not absolute: %q", e.Path)
	}
	if e.Description == "" || !strings.Contains(e.Description, "reviewing code changes") {
		t.Errorf("description missing/ wrong: %q", e.Description)
	}

	// 2. Catalog contains metadata (name/description/path), not the body.
	catalog := consumer.Catalog(entries)
	if catalog == "" {
		t.Fatal("catalog empty")
	}
	for _, want := range []string{`"name"`, "review-checklist", "reviewing code changes", "SKILL.md"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog missing %q", want)
		}
	}
	if strings.Contains(catalog, "detailed per-file rubric") {
		t.Error("catalog leaked skill body text; it must contain metadata only")
	}

	// 3. Load the discovered SKILL.md through the actual read_file capability.
	cap := ReadFile()
	if cap.Name() != "read_file" || cap.Tier() != capabilities.TierR {
		t.Fatalf("unexpected capability identity: %q %q", cap.Name(), cap.Tier())
	}
	args, _ := json.Marshal(map[string]any{"path": e.Path})
	res, err := cap.Execute(context.Background(), &capabilities.Call{WorkDir: workDir, Args: args, Emit: func(string) {}})
	if err != nil {
		t.Fatalf("read_file on discovered SKILL.md: %v", err)
	}
	if !strings.Contains(res.Text, "# Review checklist") {
		t.Errorf("read_file body missing heading: %q", res.Text)
	}
	if !strings.Contains(res.Text, "references/checklist.md") {
		t.Errorf("read_file body missing relative reference guidance: %q", res.Text)
	}

	// 4. Read the referenced supporting file through the same capability,
	// using an absolute path derived from the skill directory.
	relArgs, _ := json.Marshal(map[string]any{"path": wantRefPath})
	refRes, err := cap.Execute(context.Background(), &capabilities.Call{WorkDir: workDir, Args: relArgs, Emit: func(string) {}})
	if err != nil {
		t.Fatalf("read_file on referenced file: %v", err)
	}
	if !strings.Contains(refRes.Text, "Public API changes documented") {
		t.Errorf("referenced file content wrong: %q", refRes.Text)
	}
}
