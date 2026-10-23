package paths_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"cercano/source/server/internal/conversation"
	llamaengine "cercano/source/server/internal/engine/llamaserver"
	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/failurelog"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/routinglog"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/web"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/setupstate"
)

// isolate points HOME at a fresh directory, selects profile, and clears every
// environment override so each path function falls through to its default.
func isolate(t *testing.T, profile string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("CERCANO_PROFILE", profile)
	for _, k := range []string{"CERCANO_CONVERSATIONS_DB", "CERCANO_WIZARD_STATE", "CERCANO_ANOMALY_LOG"} {
		t.Setenv(k, "")
	}
	return home
}

func TestProfileOwnedFilesLiveInProfileDir(t *testing.T) {
	home := isolate(t, "work")
	profileDir := filepath.Join(home, ".config", "cercano", "profiles", "work")

	conversations, err := conversation.DefaultPath()
	if err != nil {
		t.Fatalf("conversation.DefaultPath: %v", err)
	}
	enterpriseDir, err := enterprise.DefaultStateDir()
	if err != nil {
		t.Fatalf("enterprise.DefaultStateDir: %v", err)
	}
	cfgDir := filepath.Dir(config.DefaultPath())
	rows := []struct {
		name, got, want string
	}{
		{"config.yaml", config.DefaultPath(), "config.yaml"},
		{"mcp.yaml dir (mcp_host.LoadConfig receives the config dir)", filepath.Join(cfgDir, "mcp.yaml"), "mcp.yaml"},
		{"conversations.db", conversations, "conversations.db"},
		{"wizard_state.yaml", setupstate.StatePath(), "wizard_state.yaml"},
		{"failures.jsonl", failurelog.DefaultPath(), "failures.jsonl"},
		{"turn-routing.jsonl", routinglog.DefaultPath(), "turn-routing.jsonl"},
		{"stream-anomalies.jsonl", llm.AnomalyLogPath(), "stream-anomalies.jsonl"},
		{"scripts/", web.DefaultScriptDir(), "scripts"},
		{"crash.log (agentclient)", agentclient.DefaultCrashLogPath(), "crash.log"},
		{"crash.log (llamaserver engine)", llamaengine.DefaultCrashLogPath(), "crash.log"},
		{"watchdog-audit.db", server.DefaultWatchdogAuditPath(), "watchdog-audit.db"},
		{"enterprise/", enterpriseDir, "enterprise"},
	}
	for _, r := range rows {
		want := filepath.Join(profileDir, r.want)
		if r.got != want {
			t.Errorf("%s: got %q, want %q", r.name, r.got, want)
		}
	}
}

func TestEnvOverridesStillWin(t *testing.T) {
	home := isolate(t, "work")
	db := filepath.Join(home, "elsewhere", "conv.db")
	t.Setenv("CERCANO_CONVERSATIONS_DB", db)
	if got, err := conversation.DefaultPath(); err != nil || got != db {
		t.Errorf("conversation.DefaultPath() = %q, %v; want %q", got, err, db)
	}
	wizard := filepath.Join(home, "elsewhere", "wizard.yaml")
	t.Setenv("CERCANO_WIZARD_STATE", wizard)
	if got := setupstate.StatePath(); got != wizard {
		t.Errorf("setupstate.StatePath() = %q, want %q", got, wizard)
	}
	anomalies := filepath.Join(home, "elsewhere", "anomalies.jsonl")
	t.Setenv("CERCANO_ANOMALY_LOG", anomalies)
	if got := llm.AnomalyLogPath(); got != anomalies {
		t.Errorf("llm.AnomalyLogPath() = %q, want %q", got, anomalies)
	}
	t.Setenv("CERCANO_PORT", "50123")
	cfg, err := config.Load(config.DefaultPath())
	if err != nil || cfg.Port != "50123" {
		t.Errorf("config.Load port = %q, %v; want 50123", cfg.Port, err)
	}
}

func TestInvalidProfileFallsBackButLoadReportsIt(t *testing.T) {
	home := isolate(t, "../escape")
	want := filepath.Join(home, ".config", "cercano", "profiles", "default", "config.yaml")
	if got := config.DefaultPath(); got != want {
		t.Errorf("config.DefaultPath() = %q, want default profile %q", got, want)
	}
	if _, err := config.Load(config.DefaultPath()); err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Errorf("config.Load error = %v, want the profile validation error", err)
	}
	if _, err := conversation.DefaultPath(); err == nil {
		t.Error("conversation.DefaultPath() error = nil, want the profile validation error")
	}
}

// allowedLiteral is a home-relative path literal that is allowed outside
// pkg/paths. Each entry names the file, the exact literal, and why the path
// is not profile-owned. Paths are relative to the repository's source/ dir.
type allowedLiteral struct{ file, literal, reason string }

var hardcodedPathAllowlist = []allowedLiteral{
	// Machine-wide state under ~/.cercano (models, run, bin): shared by every
	// profile and by the machine-wide model host.
	{"server/internal/modelhost/registry.go", ".cercano", "~/.cercano/run/modelhost: machine-wide model host registry; RunDir must error without a home dir"},
	{"server/internal/localruntime/llamaserver/provider.go", ".cercano", "~/.cercano/models: machine-wide model store"},
	{"server/internal/localruntime/llamaserver/orphans.go", ".cercano", "~/.cercano/run/llamaserver: machine-wide runtime pid records"},
	{"server/internal/localruntime/mistralrs/catalog_records.go", ".cercano", "~/.cercano/models: machine-wide model store"},
	{"server/internal/localruntime/mistralrs/provider.go", ".cercano", "~/.cercano/bin/mistralrs: machine-wide runtime binary"},
	{"server/internal/localruntime/mistralrs/orphans.go", ".cercano", "~/.cercano/run/mistralrs: machine-wide runtime pid records"},
	{"server/pkg/config/config.go", "~/.cercano/models", "default llama_server.model_dirs: machine-wide model store"},
	{"server/internal/server/server.go", "~/.cercano/models", "fallback download dir: machine-wide model store"},
	// Client-only UI preferences are machine-wide by design.
	{"clients/cli/internal/uiconfig/uiconfig.go", ".config", "ui.yaml and themes/: machine-wide CLI preferences"},
	// Project-local .cercano directories live in a repository, not under HOME.
	{"server/internal/context/context.go", ".cercano", "<project>/.cercano: project context directory"},
	{"server/internal/document/writer.go", ".cercano", "<file dir>/.cercano/backups: project-local document backups"},
	{"server/internal/gitflow/config.go", ".cercano", "<repo>/.cercano/gitflow.yaml: project config"},
	{"server/internal/projectconfig/config.go", ".cercano", "<workdir>/.cercano/config.yaml: project config"},
	{"clients/cli/internal/slash/context.go", ".cercano", "<cwd>/.cercano/context.md: project context file"},
}

// suspiciousPathLiteral reports whether a string literal builds a path under
// ~/.config or ~/.cercano: a bare ".config"/".cercano" path element, or a
// literal rooted at $HOME, ~ or a home-joined "/.config", "/.cercano".
func suspiciousPathLiteral(s string) bool {
	if s == ".config" || s == ".cercano" {
		return true
	}
	for _, p := range []string{"$HOME/.config", "$HOME/.cercano", "~/.config/", "~/.cercano/", "/.config/", "/.cercano/"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// TestNoHardcodedConfigPaths stops new ad hoc paths under ~/.config/cercano or
// ~/.cercano: profile-owned files must come from pkg/paths so they follow the
// active profile. Machine-wide and project-local paths are allowlisted above.
func TestNoHardcodedConfigPaths(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[[2]string]bool{}
	for _, a := range hardcodedPathAllowlist {
		allowed[[2]string{a.file, a.literal}] = true
	}
	used := map[[2]string]bool{}
	var violations []string
	for _, module := range []string{"server", filepath.Join("clients", "cli")} {
		root := filepath.Join(sourceRoot, module)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(sourceRoot, path)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				name := d.Name()
				if rel == "server/pkg/paths" || name == "testdata" || name == "vendor" || name == "node_modules" || (strings.HasPrefix(name, ".") && path != root) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil || !suspiciousPathLiteral(s) {
					return true
				}
				key := [2]string{rel, s}
				if allowed[key] {
					used[key] = true
					return true
				}
				violations = append(violations, fmt.Sprintf("%s:%d: %q", rel, fset.Position(lit.Pos()).Line, s))
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	sort.Strings(violations)
	for _, v := range violations {
		t.Errorf("hardcoded home path (use pkg/paths, or allowlist a machine-wide path with a reason): %s", v)
	}
	for _, a := range hardcodedPathAllowlist {
		if !used[[2]string{a.file, a.literal}] {
			t.Errorf("stale allowlist entry: %s %q no longer appears; remove it", a.file, a.literal)
		}
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "server", "pkg", "paths", "paths.go")); err != nil {
		t.Fatalf("source root %s looks wrong: %v", sourceRoot, err)
	}
}
