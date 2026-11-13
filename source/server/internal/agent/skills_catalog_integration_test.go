package agent

// Integration tests for the agent-skills consumer wiring: the real project
// context loader (internal/context, which wraps the internal/skills/consumer
// discovery) must cause the actual prompt handed to the provider to carry the
// skills metadata catalog — without context.md, with user-only skills, and
// without skill bodies leaking. This proves the agent wiring, which the
// context package's own unit tests cannot.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	projectctx "cercano/source/server/internal/context"
)

func writeSkillFile(t *testing.T, root, name, desc, body string) {
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

// TestAgent_SkillsCatalogReachesPrompt proves the full wiring: Request.WorkDir
// → Agent.loadHistory → real context.Loader.PrependContext → skills consumer
// discovery → provider receives the catalog in the user prompt.
func TestAgent_SkillsCatalogReachesPrompt(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	writeSkillFile(t, filepath.Join(workspace, ".agents", "skills"), "ws-skill", "Workspace skill guidance", "WS-SECRET-BODY")
	writeSkillFile(t, filepath.Join(home, ".claude", "skills"), "home-skill", "Home skill guidance", "HOME-SECRET-BODY")

	loader := projectctx.NewLoaderWithHomeDir(func() string { return home })
	provider := &mockModelProviderFunc{name: "capturing", response: &Response{Output: "done"}}
	router := &mockRouter{intent: IntentChat, provider: provider}
	a := NewAgent(router, &mockCoordinator{}, WithContextLoader(loader))

	res, err := a.ProcessRequest(context.Background(), &Request{Input: "Do the task", WorkDir: workspace})
	if err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}
	if res.Output != "done" {
		t.Fatalf("unexpected output: %q", res.Output)
	}

	if provider.capturedReq == nil {
		t.Fatal("provider never received a request")
	}
	prompt := provider.capturedReq.Input

	if !strings.Contains(prompt, "Agent Skills (metadata catalog)") {
		t.Errorf("prompt missing skills catalog header:\n%s", prompt)
	}
	for _, want := range []string{"ws-skill", "Workspace skill guidance", "home-skill", "Home skill guidance", "UNTRUSTED"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, leak := range []string{"WS-SECRET-BODY", "HOME-SECRET-BODY"} {
		if strings.Contains(prompt, leak) {
			t.Errorf("skill body %q leaked into prompt:\n%s", leak, prompt)
		}
	}
	if !strings.HasSuffix(prompt, "Do the task") {
		t.Errorf("prompt must end with the user's input:\n%s", prompt)
	}
}

// TestAgent_SkillsNoCatalogWithoutLoader preserves existing behavior: with no
// context loader attached, nothing is added.
func TestAgent_SkillsNoCatalogWithoutLoader(t *testing.T) {
	workspace := t.TempDir()
	writeSkillFile(t, filepath.Join(workspace, ".agents", "skills"), "ws-skill", "WS guidance", "body")

	provider := &mockModelProviderFunc{name: "capturing", response: &Response{Output: "done"}}
	router := &mockRouter{intent: IntentChat, provider: provider}
	a := NewAgent(router, &mockCoordinator{})

	if _, err := a.ProcessRequest(context.Background(), &Request{Input: "Do the task", WorkDir: workspace}); err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}
	if provider.capturedReq.Input != "Do the task" {
		t.Errorf("prompt unexpectedly modified without loader: %q", provider.capturedReq.Input)
	}
}

// TestAgent_SkillsEmptyWorkDir: an empty WorkDir must produce an unchanged
// prompt even when the loader is attached (no implicit process-cwd skills).
func TestAgent_SkillsEmptyWorkDir(t *testing.T) {
	home := t.TempDir()
	writeSkillFile(t, filepath.Join(home, ".agents", "skills"), "home-skill", "H guidance", "body")
	t.Chdir(home) // if cwd leaked into discovery this would find home-skill

	loader := projectctx.NewLoaderWithHomeDir(func() string { return home })
	provider := &mockModelProviderFunc{name: "capturing", response: &Response{Output: "done"}}
	router := &mockRouter{intent: IntentChat, provider: provider}
	a := NewAgent(router, &mockCoordinator{}, WithContextLoader(loader))

	if _, err := a.ProcessRequest(context.Background(), &Request{Input: "Do the task"}); err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}
	if provider.capturedReq.Input != "Do the task" {
		t.Errorf("empty WorkDir must leave prompt unchanged, got: %q", provider.capturedReq.Input)
	}
}
