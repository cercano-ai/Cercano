package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestEverydayLauncherCompiledPTY is an explicit, relatively expensive gate. A
// failure is not replaced by the cheaper refusal coverage below. This
// gate covers live A/B publication, task completion and explicit retirement.
func TestEverydayLauncherCompiledPTY(t *testing.T) {
	if os.Getenv("CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE") != "1" {
		t.Skip("set CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE=1 for real tagged launcher acceptance")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	evidence := os.Getenv("CERCANO_COMPILED_LAUNCHER_EVIDENCE")
	if evidence == "" {
		evidence = filepath.Join(t.TempDir(), "compiled-evidence")
	}
	cmd := exec.Command(python, filepath.Join(repo, "efforts", "dev-host-generations", "everyday-launcher-compiled.py"), "--repo", repo, "--evidence", evidence)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compiled ordinary launcher probe: %v; evidence %s\n%s", err, evidence, output)
	}
	t.Log(string(output))
}

// This negative entrypoint coverage deliberately does not claim the compiled
// production A/B acceptance journey. No fixture development-host is started.
func TestEverydayLauncherEntrypointRefusalsPTY(t *testing.T) {
	// Reproduce an ordinary developer shell's inherited authority. The disposable
	// harness must isolate it rather than masking its injected-failure assertion.
	t.Setenv("CERCANO_LAUNCHED_DEVELOPMENT", "1")
	t.Setenv("CERCANO_DEVELOPMENT_CONFIG_AUTHORITY", filepath.Join(t.TempDir(), "caller-config.yaml"))
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 required for actual shell-entrypoint PTY test")
	}
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	harness := filepath.Join(repo, "efforts", "dev-host-generations", "everyday-launcher-pty.py")
	if _, err := os.Stat(harness); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, harness, "--repo", repo, "--evidence", filepath.Join(t.TempDir(), "evidence.json"))
	cmd.Env = append(os.Environ(), "CERCANO_DISABLE_CREDENTIALS=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ordinary launcher PTY refusals: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}

// TestEverydayLauncherHistorySmoke reuses the disposable compiled harness, not
// an installed runtime. Kept opt-in because it builds two real generations.
func TestEverydayLauncherHistorySmoke(t *testing.T) {
	if os.Getenv("CERCANO_RUN_LAUNCHER_HISTORY_SMOKE") != "1" {
		t.Skip("set CERCANO_RUN_LAUNCHER_HISTORY_SMOKE=1")
	}
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", filepath.Join(repo, "efforts/dev-host-generations/everyday-launcher-compiled.py"), "--repo", repo, "--evidence", filepath.Join(t.TempDir(), "history-smoke"), "--history-smoke")
	cmd.Env = append(os.Environ(), "CERCANO_DISABLE_CREDENTIALS=1", "GOPROXY=off", "GOSUMDB=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bounded launcher history smoke: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
