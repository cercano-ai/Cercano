package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/profilemigrate"
)

func TestMigrationMovesLegacyConfigBeforeLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "")
	t.Setenv("CERCANO_PORT", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	prev := migrationOptions
	migrationOptions = func() profilemigrate.Options {
		return profilemigrate.Options{LegacyAgentAlive: func(context.Context) (bool, error) { return false, nil }}
	}
	t.Cleanup(func() { migrationOptions = prev })

	root := paths.ConfigRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("port: \"50455\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateProfileData(); err != nil {
		t.Fatalf("migrateProfileData: %v", err)
	}
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "50455" {
		t.Errorf("port = %q: the CLI would dial the wrong agent and reopen setup", cfg.Port)
	}
	if needsSetup(false, config.DefaultPath()) {
		t.Error("a migrated install would reopen the setup wizard")
	}
}

// main must migrate before it loads the config.
func TestMainMigratesBeforeConfigLoad(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var migrateAt, loadAt token.Pos
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" || fn.Recv != nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch f := call.Fun.(type) {
			case *ast.Ident:
				if f.Name == "migrateProfileData" && migrateAt == 0 {
					migrateAt = call.Pos()
				}
			case *ast.SelectorExpr:
				if x, ok := f.X.(*ast.Ident); ok && x.Name == "config" && f.Sel.Name == "Load" && loadAt == 0 {
					loadAt = call.Pos()
				}
			}
			return true
		})
	}
	if migrateAt == 0 || loadAt == 0 || migrateAt > loadAt {
		t.Fatalf("main must call migrateProfileData before config.Load (migrate at %v, load at %v)", fset.Position(migrateAt), fset.Position(loadAt))
	}
}
