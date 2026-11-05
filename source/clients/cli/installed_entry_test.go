package main

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise actual main routing without any successful dial, credential access or TUI.
func TestInstalledEntryRouting(t *testing.T) {
	if os.Getenv("CERCANO_INSTALLED_ENTRY_CHILD") == "1" {
		flag.CommandLine = flag.NewFlagSet("installed-entry", flag.ExitOnError)
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"installed-entry"}, os.Args[i+1:]...)
				break
			}
		}
		main()
		return
	}
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"bare", nil, "installed activation deferred:"},
		{"remote", []string{"--address="}, "address"},
		{"development", []string{"--dev-host=" + filepath.Join(root, "missing")}, "missing"},
		{"reconnect", []string{"--session=" + filepath.Join(root, "missing")}, "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestInstalledEntryRouting$", "--"}, tc.args...)...)
			cmd.Env = []string{"HOME=" + root, "XDG_CONFIG_HOME=" + root, "XDG_DATA_HOME=" + root, "XDG_CACHE_HOME=" + root, "PATH=/usr/bin:/bin", "CERCANO_DISABLE_CREDENTIALS=1", "CERCANO_INSTALLED_ENTRY_CHILD=1"}
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), tc.want) {
				t.Fatalf("routing: %v: %s", err, output)
			}
			if tc.name != "bare" && strings.Contains(string(output), "installed activation deferred:") {
				t.Fatalf("explicit lane routed to installed: %s", output)
			}
		})
	}
}
