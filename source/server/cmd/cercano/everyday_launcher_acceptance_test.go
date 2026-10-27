package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestEverydayLauncherCompiledPTY is an explicit, relatively expensive gate. A
// failure is not replaced by the cheaper refusal coverage below. Its current
// bounded probe fails before independent dispatch; see the progress artifact.
func TestEverydayLauncherCompiledPTY(t *testing.T) {
	if os.Getenv("CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE") != "1" {
		t.Skip("set CERCANO_RUN_COMPILED_LAUNCHER_ACCEPTANCE=1 for real tagged launcher probe")
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
