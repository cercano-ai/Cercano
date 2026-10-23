package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/agent"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/profilemigrate"
)

// stubMigrationOptions keeps tests from probing or stopping a real agent.
func stubMigrationOptions(t *testing.T) *int {
	t.Helper()
	calls := 0
	prev := migrationOptions
	migrationOptions = func() profilemigrate.Options {
		calls++
		return profilemigrate.Options{
			LegacyAgentAlive: func(context.Context) (bool, error) { return false, nil },
		}
	}
	t.Cleanup(func() { migrationOptions = prev })
	return &calls
}

func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "")
	t.Setenv("CERCANO_PORT", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func TestProfileMigrationRunsForProfileCommands(t *testing.T) {
	for _, args := range [][]string{
		{"cercano"}, {"cercano", "--mcp"}, {"cercano", "--server"}, {"cercano", "agent"},
		{"cercano", "run", "hi"}, {"cercano", "setup"}, {"cercano", "reset"},
		{"cercano", "enterprise", "status"},
		{"cercano", "stats"}, {"cercano", "logs"}, {"cercano", "export"},
		// These commands ignore or misread a help flag and go on to touch
		// profile data, so they must migrate first.
		{"cercano", "agent", "--help"}, {"cercano", "agent", "-h"},
		{"cercano", "stats", "-h"}, {"cercano", "stats", "--help"},
		{"cercano", "setup", "-help"},                // setup only knows -h and --help
		{"cercano", "run", "-help"},                  // a prompt, not a flag
		{"cercano", "run", "--conv", "-h", "prompt"}, // -h is the conversation ID
		{"cercano", "enterprise", "-h"},              // "unknown enterprise command" path
		{"cercano", "enterprise", "status", "--help"},
		{"cercano", "logs", "--tail", "-h"}, // -h is --tail's value
		{"cercano", "export", "--help"},     // only "export trajectory" parses flags
		{"cercano", "--grpc-addr", "-h"},    // -h is --grpc-addr's value
		{"cercano", "unknown", "--help"},    // flag parsing stops at "unknown"
	} {
		if !needsProfileMigration(args) {
			t.Errorf("needsProfileMigration(%v) = false, want true", args)
		}
	}
	// Workers are spawned by an agent that already migrated; the model host
	// is machine-wide; restart-after-upgrade leaves the move to the agent the
	// Homebrew coordinator relaunches; the help forms below print usage
	// before the command touches anything.
	for _, args := range [][]string{
		{"cercano", "worker"}, {"cercano", "model-host"}, {"cercano", "version"},
		{"cercano", "--version"}, {"cercano", "protocols", "sync"}, {"cercano", "skills", "sync"},
		{"cercano", "restart-after-upgrade"}, {"cercano", "restart-after-upgrade", "--timeout", "1m"},
		{"cercano", "--help"}, {"cercano", "-h"}, {"cercano", "-help"}, {"cercano", "--mcp", "--help"},
		{"cercano", "setup", "--help"}, {"cercano", "setup", "-h"}, {"cercano", "setup", "--install-engine", "-h"},
		{"cercano", "run", "--help"}, {"cercano", "run", "-h"}, {"cercano", "run", "--conv", "abc", "-h"},
		{"cercano", "reset", "-h"}, {"cercano", "reset", "--help"}, {"cercano", "reset", "--setup", "-help"},
		{"cercano", "logs", "--help"}, {"cercano", "logs", "--crashes", "-h"},
		{"cercano", "export", "trajectory", "--help"}, {"cercano", "export", "trajectory", "-h"},
		{"cercano", "enterprise"}, {"cercano", "enterprise", "--help"}, {"cercano", "enterprise", "help"},
	} {
		if needsProfileMigration(args) {
			t.Errorf("needsProfileMigration(%v) = true, want false", args)
		}
	}
}

// The startup helper moves legacy data before the agent reads config or
// permissions, so both are found at their profile paths.
func TestStartupMigratesBeforeProfileFilesAreRead(t *testing.T) {
	tempHome(t)
	calls := stubMigrationOptions(t)
	root := paths.ConfigRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("port: \"50444\"\nllama_server:\n    extra_args: [--legacy]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "permissions.yaml"), []byte("mode: strict\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := migrateProfileData([]string{"cercano", "agent"}); err != nil {
		t.Fatalf("migrateProfileData: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("migration options built %d times, want 1", *calls)
	}

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Port != "50444" {
		t.Errorf("port = %q: the agent would not see the legacy config", cfg.Port)
	}
	if len(cfg.LlamaServer.ExtraArgs) != 1 || cfg.LlamaServer.ExtraArgs[0] != "--legacy" {
		t.Errorf("llama_server.extra_args = %v", cfg.LlamaServer.ExtraArgs)
	}
	if _, err := os.Stat(paths.MachineConfigPath()); err != nil {
		t.Errorf("machine.yaml: %v", err)
	}
	store, err := agent.LoadPermissionStore(profileFile("permissions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if store.Mode() != agent.ModeStrict {
		t.Errorf("permission mode = %q: the agent would not see the legacy permissions", store.Mode())
	}
}

func TestWorkerAndModelHostSkipMigration(t *testing.T) {
	tempHome(t)
	calls := stubMigrationOptions(t)
	for _, cmd := range []string{"worker", "model-host"} {
		if err := migrateProfileData([]string{"cercano", cmd}); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}
	if *calls != 0 {
		t.Errorf("migration ran %d times", *calls)
	}
	if _, err := os.Stat(paths.ProfilesRoot()); !os.IsNotExist(err) {
		t.Errorf("profiles root touched: %v", err)
	}
}

// main must migrate right after the startup environment check and before it
// dispatches any command, so no command can open a profile file first.
func TestMainMigratesBeforeDispatch(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var mainFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
			mainFn = fn
		}
	}
	if mainFn == nil {
		t.Fatal("func main not found")
	}
	var calls []string
	for _, stmt := range mainFn.Body.List {
		expr, ok := stmt.(*ast.ExprStmt)
		if !ok {
			break
		}
		call, ok := expr.X.(*ast.CallExpr)
		if !ok {
			break
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok {
			break
		}
		calls = append(calls, ident.Name)
	}
	if len(calls) < 2 || calls[0] != "requireStartupEnvironment" || calls[1] != "requireProfileMigration" {
		t.Fatalf("main begins with %v, want [requireStartupEnvironment requireProfileMigration ...]", calls)
	}
}

func TestRestartAfterUpgradeDoesNotMigrate(t *testing.T) {
	tempHome(t)
	calls := stubMigrationOptions(t)
	legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("port: \"50777\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"cercano", "restart-after-upgrade"}, {"cercano", "setup", "--help"}, {"cercano", "--help"}} {
		if err := migrateProfileData(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if *calls != 0 {
		t.Errorf("migration ran %d times", *calls)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy config moved: %v", err)
	}
	if _, err := os.Stat(paths.ProfilesRoot()); !os.IsNotExist(err) {
		t.Errorf("profiles root touched: %v", err)
	}
}

// Before migration there is no profile config, so restart-after-upgrade
// addresses the pre-profile agent where it actually listens.
func TestUpgradeRestartPortFallsBackToLegacyAgent(t *testing.T) {
	tempHome(t)
	if port, err := upgradeRestartPort(); err != nil || port != "50052" {
		t.Errorf("fresh: port = %q, %v; want the default", port, err)
	}
	legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("port: \"50777\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if port, err := upgradeRestartPort(); err != nil || port != "50777" {
		t.Errorf("legacy config: port = %q, %v; want 50777", port, err)
	}
	t.Setenv("CERCANO_PORT", "50888")
	if port, err := upgradeRestartPort(); err != nil || port != "50888" {
		t.Errorf("CERCANO_PORT: port = %q, %v; want 50888", port, err)
	}
	t.Setenv("CERCANO_PORT", "")
	profileCfg := config.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(profileCfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profileCfg, []byte("port: \"50999\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if port, err := upgradeRestartPort(); err != nil || port != "50999" {
		t.Errorf("profile config: port = %q, %v; want 50999", port, err)
	}
}
