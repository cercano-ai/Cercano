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
	"cercano/source/server/internal/crashlog"
	llamaengine "cercano/source/server/internal/engine/llamaserver"
	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/failurelog"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/routinglog"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/telemetry"
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
// pkg/paths. Each entry names the file, the exact literal, how many times it
// appears there, and why the path is not profile-owned. The count keeps an
// entry from silently covering a new use of the same literal in that file.
// Paths are relative to the repository's source/ dir.
type allowedLiteral struct {
	file, literal string
	count         int
	reason        string
}

var hardcodedPathAllowlist = []allowedLiteral{
	// Machine-wide state under ~/.cercano (models, run, bin): shared by every
	// profile and by the machine-wide model host.
	{"server/internal/modelhost/registry.go", ".cercano", 1, "~/.cercano/run/modelhost: machine-wide model host registry; RunDir must error without a home dir"},
	{"server/internal/localruntime/llamaserver/provider.go", ".cercano", 1, "~/.cercano/models: machine-wide model store"},
	{"server/internal/localruntime/llamaserver/orphans.go", ".cercano", 1, "~/.cercano/run/llamaserver: machine-wide runtime pid records"},
	{"server/internal/localruntime/mistralrs/catalog_records.go", ".cercano", 1, "~/.cercano/models: machine-wide model store"},
	{"server/internal/localruntime/mistralrs/provider.go", ".cercano", 1, "~/.cercano/bin/mistralrs: machine-wide runtime binary"},
	{"server/internal/localruntime/mistralrs/orphans.go", ".cercano", 1, "~/.cercano/run/mistralrs: machine-wide runtime pid records"},
	{"server/pkg/config/config.go", "~/.cercano/models", 2, "default llama_server and mistralrs model_dirs: machine-wide model store"},
	{"server/internal/server/server.go", "~/.cercano/models", 1, "fallback download dir: machine-wide model store"},
	// Client-only UI preferences are machine-wide by design.
	{"clients/cli/internal/uiconfig/uiconfig.go", ".config", 1, "ui.yaml and themes/: machine-wide CLI preferences"},
	// Project-local .cercano directories live in a repository, not under HOME.
	{"server/internal/capabilities/builtins/deepinfra_infer.go", ".cercano/deepinfra", 1, "<workdir>/.cercano/deepinfra: default artifact dir, resolved against the work directory"},
	{"server/internal/context/context.go", ".cercano", 1, "<project>/.cercano: project context directory"},
	{"server/internal/document/writer.go", ".cercano", 1, "<file dir>/.cercano/backups: project-local document backups"},
	{"server/internal/gitflow/config.go", ".cercano", 1, "<repo>/.cercano/gitflow.yaml: project config"},
	{"server/internal/projectconfig/config.go", ".cercano", 1, "<workdir>/.cercano/config.yaml: project config"},
	{"clients/cli/internal/slash/context.go", ".cercano", 1, "<cwd>/.cercano/context.md: project context file"},
}

// suspiciousPathLiteral reports whether a string literal can build a path
// under ~/.config or ~/.cercano: a bare ".config"/".cercano" element, one
// with more path after it (".config/cercano"), one ending a joined path
// (home + "/.cercano"), or a literal rooted at $HOME, ~ or a home-joined
// "/.config/", "/.cercano/".
func suspiciousPathLiteral(s string) bool {
	for _, dir := range []string{".config", ".cercano"} {
		if s == dir || strings.HasPrefix(s, dir+"/") || strings.HasSuffix(s, "/"+dir) {
			return true
		}
		for _, root := range []string{"$HOME/", "~/", "/"} {
			if strings.HasPrefix(s, root+dir+"/") || s == root+dir {
				return true
			}
		}
	}
	return false
}

// TestSuspiciousPathLiteralCoverage pins which literal forms the guard
// catches, so a narrowed matcher cannot silently let ad hoc paths through.
func TestSuspiciousPathLiteralCoverage(t *testing.T) {
	flagged := []string{
		".config", ".cercano", // filepath.Join(home, ".config", ...)
		".config/cercano", ".config/cercano/x.db", ".cercano/enterprise", ".cercano/deepinfra", // filepath.Join(home, ".config/cercano", ...)
		"/.config", "/.cercano", // home + "/.config"
		"/.config/cercano/x", "/.cercano/trace-openai-stream", // home + "/.cercano/..."
		"$HOME/.config/cercano/conversations.db", "$HOME/.cercano", // os.ExpandEnv
		"~/.config/cercano/config.yaml", "~/.cercano/models", // ~ expansion
	}
	for _, s := range flagged {
		if !suspiciousPathLiteral(s) {
			t.Errorf("suspiciousPathLiteral(%q) = false, want true", s)
		}
	}
	clean := []string{
		"", "config.yaml", "cercano", "config", ".configure", ".cercanorc", "my.config",
		"profiles", "logs/dispatch.log", "a .cercano dir", "x/.configs", "validation skipped",
	}
	for _, s := range clean {
		if suspiciousPathLiteral(s) {
			t.Errorf("suspiciousPathLiteral(%q) = true, want false", s)
		}
	}
}

// TestNoHardcodedConfigPaths stops new ad hoc paths under ~/.config/cercano or
// ~/.cercano: profile-owned files must come from pkg/paths so they follow the
// active profile. Machine-wide and project-local paths are allowlisted above.
func TestNoHardcodedConfigPaths(t *testing.T) {
	sourceRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ file, literal string }
	allowed := map[key]int{}
	for _, a := range hardcodedPathAllowlist {
		allowed[key{a.file, a.literal}] = a.count
	}
	hits := map[key][]string{}
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
				k := key{rel, s}
				hits[k] = append(hits[k], fmt.Sprintf("%s:%d", rel, fset.Position(lit.Pos()).Line))
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	var violations []string
	for k, at := range hits {
		if want, ok := allowed[k]; ok {
			if len(at) != want {
				violations = append(violations, fmt.Sprintf("%q allowlisted %d time(s) in %s but found %d: %v", k.literal, want, k.file, len(at), at))
			}
			continue
		}
		for _, pos := range at {
			violations = append(violations, fmt.Sprintf("%s: %q", pos, k.literal))
		}
	}
	sort.Strings(violations)
	for _, v := range violations {
		t.Errorf("hardcoded home path (use pkg/paths, or allowlist a machine-wide or project-local path with a reason): %s", v)
	}
	for _, a := range hardcodedPathAllowlist {
		if len(hits[key{a.file, a.literal}]) == 0 {
			t.Errorf("stale allowlist entry: %s %q no longer appears; remove it", a.file, a.literal)
		}
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "server", "pkg", "paths", "paths.go")); err != nil {
		t.Fatalf("source root %s looks wrong: %v", sourceRoot, err)
	}
}

// Profile directories hold conversations, credentials state and logs; every
// creator makes them private (0700), as the agent registry does.
func TestProfileDirCreatorsUsePrivateMode(t *testing.T) {
	creators := map[string]func(t *testing.T, profileDir string) error{
		"config.Save": func(t *testing.T, _ string) error { return config.Save(config.Defaults(), config.DefaultPath()) },
		"conversation.DefaultPath": func(t *testing.T, _ string) error {
			_, err := conversation.DefaultPath()
			return err
		},
		"failurelog.NewWriter": func(t *testing.T, _ string) error {
			w, err := failurelog.NewWriter("")
			if w != nil {
				w.Close()
			}
			return err
		},
		"routinglog.NewWriter": func(t *testing.T, _ string) error {
			w, err := routinglog.NewWriter("")
			if w != nil {
				w.Close()
			}
			return err
		},
		"setupstate.Save": func(t *testing.T, _ string) error { return setupstate.Save(setupstate.State{}) },
		"crashlog.NewWriter": func(t *testing.T, dir string) error {
			w, err := crashlog.NewWriter(filepath.Join(dir, "crash.log"), "test")
			if w != nil {
				w.Close()
			}
			return err
		},
		"telemetry.NewSQLiteStore": func(t *testing.T, dir string) error {
			s, err := telemetry.NewSQLiteStore(filepath.Join(dir, "telemetry.db"))
			if s != nil {
				s.Close()
			}
			return err
		},
		"web.EnsureSearchScript": func(t *testing.T, _ string) error {
			_, err := web.EnsureSearchScript("")
			return err
		},
	}
	for name, create := range creators {
		t.Run(name, func(t *testing.T) {
			home := isolate(t, "work")
			profileDir := filepath.Join(home, ".config", "cercano", "profiles", "work")
			if err := create(t, profileDir); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			fi, err := os.Stat(profileDir)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm() != 0o700 {
				t.Errorf("%s created the profile directory with mode %v, want 0700", name, fi.Mode().Perm())
			}
		})
	}
}
