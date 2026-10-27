package main

import (
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Runs only the refusal/version paths: never config, credentials, dial or TUI.
func TestDevelopmentLauncherGuard(t *testing.T) {
	if os.Getenv("CERCANO_GUARD_TEST_CHILD") == "1" {
		flag.CommandLine = flag.NewFlagSet("guard-probe", flag.ExitOnError)
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"guard-probe"}, os.Args[i+1:]...)
				break
			}
		}
		main()
		return
	}
	for _, args := range [][]string{
		{}, {"--dev-host="}, {"-dev-host="},
		{"--dev-host", "injected", "--dev-host="},
		{"positional", "--dev-host=hidden"}, {"--unknown"}, {"--version"}, {"--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestDevelopmentLauncherGuard$", "--"}, args...)...)
			cmd.Env = append(os.Environ(), "CERCANO_GUARD_TEST_CHILD=1", "CERCANO_LAUNCHED_DEVELOPMENT=1", "CERCANO_DISABLE_CREDENTIALS=1")
			output, err := cmd.CombinedOutput()
			info := len(args) == 1 && (args[0] == "--version" || args[0] == "--help")
			if info {
				if err != nil {
					t.Fatalf("informational invocation: %v: %s", err, output)
				}
			} else if err == nil {
				t.Fatalf("malformed development invocation succeeded: %s", output)
			} else if !(len(args) == 1 && args[0] == "--unknown") && !strings.Contains(string(output), "legacy fallback refused") {
				t.Fatalf("did not hit failclosed guard: %s", output)
			}
		})
	}
}
