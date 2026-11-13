package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_LoadFromProject(t *testing.T) {
	dir := t.TempDir()
	cercanoDir := filepath.Join(dir, ".cercano")
	os.MkdirAll(cercanoDir, 0755)
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("# Project Context\n\nThis is a test project."), 0644)

	loader := NewLoader()
	ctx, err := loader.Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if ctx != "# Project Context\n\nThis is a test project." {
		t.Errorf("unexpected context: %q", ctx)
	}
}

func TestLoader_CachesContext(t *testing.T) {
	dir := t.TempDir()
	cercanoDir := filepath.Join(dir, ".cercano")
	os.MkdirAll(cercanoDir, 0755)
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("original"), 0644)

	loader := NewLoader()
	ctx1, _ := loader.Load(dir)

	// Change the file — cached version should still be returned
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("modified"), 0644)
	ctx2, _ := loader.Load(dir)

	if ctx1 != ctx2 {
		t.Errorf("expected cached result, got different: %q vs %q", ctx1, ctx2)
	}
}

func TestLoader_NoContextFile(t *testing.T) {
	dir := t.TempDir()

	loader := NewLoader()
	ctx, err := loader.Load(dir)
	if err != nil {
		t.Fatalf("Load should not error for missing file: %v", err)
	}
	if ctx != "" {
		t.Errorf("expected empty context, got %q", ctx)
	}
}

func TestLoader_IsInitialized(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader()

	if loader.IsInitialized(dir) {
		t.Error("expected not initialized for empty dir")
	}

	cercanoDir := filepath.Join(dir, ".cercano")
	os.MkdirAll(cercanoDir, 0755)
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("context"), 0644)

	if !loader.IsInitialized(dir) {
		t.Error("expected initialized after writing context.md")
	}
}

func TestLoader_Invalidate(t *testing.T) {
	dir := t.TempDir()
	cercanoDir := filepath.Join(dir, ".cercano")
	os.MkdirAll(cercanoDir, 0755)
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("original"), 0644)

	loader := NewLoader()
	loader.Load(dir)

	// Update file and invalidate cache
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("updated"), 0644)
	loader.Invalidate(dir)

	ctx, _ := loader.Load(dir)
	if ctx != "updated" {
		t.Errorf("expected updated context after invalidate, got %q", ctx)
	}
}

// newTestLoader builds a loader with skill home-discovery disabled so
// PrependContext tests stay hermetic regardless of the developer's real
// ~/.claude or ~/.agents contents.
func newTestLoader() *Loader {
	l := NewLoader()
	l.homeDirOverride = func() string { return "" }
	return l
}

func TestLoader_PrependContext(t *testing.T) {
	dir := t.TempDir()
	cercanoDir := filepath.Join(dir, ".cercano")
	os.MkdirAll(cercanoDir, 0755)
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("Key struct: Foo has field bar at offset 0x10"), 0644)

	loader := newTestLoader()
	result := loader.PrependContext(dir, "Explain this code")
	if result != "Project Context:\nKey struct: Foo has field bar at offset 0x10\n\n---\n\nExplain this code" {
		t.Errorf("unexpected prepend result: %q", result)
	}
}

func TestLoader_PrependContext_NoContext(t *testing.T) {
	dir := t.TempDir()
	loader := newTestLoader()
	result := loader.PrependContext(dir, "Explain this code")
	if result != "Explain this code" {
		t.Errorf("expected unchanged prompt, got %q", result)
	}
}

func TestLoader_PrependContext_EmptyProjectDir(t *testing.T) {
	loader := newTestLoader()
	result := loader.PrependContext("", "Explain this code")
	if result != "Explain this code" {
		t.Errorf("expected unchanged prompt, got %q", result)
	}
}

// writeSkillMaterial creates <root>/<name>/SKILL.md with valid frontmatter.
func writeSkillMaterial(t *testing.T, root, name, desc, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoader_PrependContext_SkillsWithoutContextFile(t *testing.T) {
	dir := t.TempDir()
	writeSkillMaterial(t, filepath.Join(dir, ".agents", "skills"), "test-skill", "Test skill guidance", "SECRET-BODY")

	loader := newTestLoader()
	result := loader.PrependContext(dir, "Do the task")
	if !strings.Contains(result, "Agent Skills (metadata catalog)") {
		t.Errorf("skills catalog missing without context.md:\n%s", result)
	}
	if strings.Contains(result, "SECRET-BODY") {
		t.Error("skill body leaked into prompt")
	}
	if strings.Contains(result, "Project Context:") {
		t.Error("unexpected context section")
	}
	if strings.HasSuffix(result, "Do the task") && strings.Contains(result, "\n\n---\n\n") {
		// prompt must still come after the catalog
	} else {
		t.Errorf("prompt not preserved after catalog:\n%s", result)
	}
}

func TestLoader_PrependContext_HomeSkillsOnly(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	writeSkillMaterial(t, filepath.Join(home, ".claude", "skills"), "home-skill", "Home skill guidance", "body")

	loader := NewLoader()
	loader.homeDirOverride = func() string { return home }
	result := loader.PrependContext(dir, "Do the task")
	if !strings.Contains(result, "home-skill") {
		t.Errorf("home-only skills missing from prompt:\n%s", result)
	}
	if strings.Contains(result, "Project Context:") {
		t.Error("unexpected context section")
	}
}

func TestLoader_PrependContext_ContextAndSkills(t *testing.T) {
	dir := t.TempDir()
	cercanoDir := filepath.Join(dir, ".cercano")
	os.MkdirAll(cercanoDir, 0755)
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("Existing project facts"), 0644)
	writeSkillMaterial(t, filepath.Join(dir, ".claude", "skills"), "ws-skill", "WS guidance", "body")

	loader := newTestLoader()
	result := loader.PrependContext(dir, "Do the task")
	if !strings.Contains(result, "Project Context:\nExisting project facts") {
		t.Errorf("existing context lost:\n%s", result)
	}
	if !strings.Contains(result, "ws-skill") {
		t.Errorf("skills section missing alongside context:\n%s", result)
	}
	if !strings.HasSuffix(result, "Do the task") {
		t.Errorf("prompt not last:\n%s", result)
	}
}

func TestLoader_PrependContext_SkillsTwoWorkdirsIsolated(t *testing.T) {
	base := t.TempDir()
	w1, w2 := filepath.Join(base, "one"), filepath.Join(base, "two")
	writeSkillMaterial(t, filepath.Join(w1, ".agents", "skills"), "alpha-skill", "A", "b")
	writeSkillMaterial(t, filepath.Join(w2, ".claude", "skills"), "beta-skill", "B", "b")

	loader := newTestLoader()
	r1 := loader.PrependContext(w1, "P1")
	if strings.Contains(r1, "beta-skill") || !strings.Contains(r1, "alpha-skill") {
		t.Errorf("workdir 1 leaked: %s", r1)
	}
	r2 := loader.PrependContext(w2, "P2")
	if strings.Contains(r2, "alpha-skill") || !strings.Contains(r2, "beta-skill") {
		t.Errorf("workdir 2 leaked: %s", r2)
	}
}

func TestLoader_PrependContext_EmptyProjectDir_WithRealHome(t *testing.T) {
	// An empty project dir must return the prompt unchanged even though the
	// production home lookup is consulted for skill roots.
	loader := NewLoader()
	if result := loader.PrependContext("", "Explain this code"); result != "Explain this code" {
		t.Errorf("expected unchanged prompt, got %q", result)
	}
}

func TestLoader_NudgeNeeded(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader()

	// No context.md, first call — should nudge
	if !loader.NudgeNeeded(dir) {
		t.Error("expected nudge needed for uninitialized project")
	}

	// Second call — already nudged
	if loader.NudgeNeeded(dir) {
		t.Error("expected no nudge on second call")
	}
}

func TestLoader_NudgeNeeded_Initialized(t *testing.T) {
	dir := t.TempDir()
	cercanoDir := filepath.Join(dir, ".cercano")
	os.MkdirAll(cercanoDir, 0755)
	os.WriteFile(filepath.Join(cercanoDir, "context.md"), []byte("context"), 0644)

	loader := NewLoader()
	if loader.NudgeNeeded(dir) {
		t.Error("expected no nudge for initialized project")
	}
}

func TestLoader_NudgeNeeded_EmptyDir(t *testing.T) {
	loader := NewLoader()
	if loader.NudgeNeeded("") {
		t.Error("expected no nudge for empty project dir")
	}
}
