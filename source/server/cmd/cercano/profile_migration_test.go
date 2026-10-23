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
		{"cercano", "enterprise", "status"}, {"cercano", "restart-after-upgrade"},
		{"cercano", "stats"}, {"cercano", "logs"}, {"cercano", "export"},
	} {
		if !needsProfileMigration(args) {
			t.Errorf("needsProfileMigration(%v) = false, want true", args)
		}
	}
	// Workers are spawned by an agent that already migrated; the model host
	// is machine-wide; the rest print static information.
	for _, args := range [][]string{
		{"cercano", "worker"}, {"cercano", "model-host"}, {"cercano", "version"},
		{"cercano", "--version"}, {"cercano", "protocols", "sync"}, {"cercano", "skills", "sync"},
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
