package research

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOutputDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	absolute := filepath.Join(t.TempDir(), "reports")
	for _, tt := range []struct {
		name string
		cfg  RunConfig
		want string
	}{
		{"empty project uses cwd", RunConfig{Topic: "Test Topic"}, filepath.Join(cwd, "scratch", "research", "test-topic")},
		{"relative explicit unchanged", RunConfig{OutputDir: "./reports/../research", ProjectDir: "workspace"}, "./reports/../research"},
		{"absolute explicit unchanged", RunConfig{OutputDir: absolute, ProjectDir: "workspace"}, absolute},
		{"relative project default unchanged", RunConfig{Topic: "Test Topic", ProjectDir: "workspace"}, filepath.Join("workspace", "scratch", "research", "test-topic")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOutputDir(tt.cfg)
			if err != nil || got != tt.want {
				t.Fatalf("ResolveOutputDir(%+v) = %q, %v; want %q", tt.cfg, got, err, tt.want)
			}
		})
	}
}
