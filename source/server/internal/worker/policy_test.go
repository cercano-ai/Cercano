package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func policyFixture(t *testing.T) SpawnOptions {
	t.Helper()
	// Resolve macOS /var alias; only fixture files are inspected.
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(base, "generation")
	root := filepath.Join(base, "resources")
	for _, dir := range []string{generation, root} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range map[string]string{cercanoBinaryName: "never executed", "manifest.json": `{"generation":"fixture"}`} {
		if err := os.WriteFile(filepath.Join(generation, name), []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return SpawnOptions{GenerationDir: generation, ManifestPath: filepath.Join(generation, "manifest.json"), HostInstance: "host-A", ResourceRoot: root}
}

func TestPinnedPolicySelectionAndNamespaces(t *testing.T) {
	o := policyFixture(t)
	a, err := NewSpawnPolicy(o)
	if err != nil {
		t.Fatal(err)
	}
	o.HostInstance = "host-B"
	b, err := NewSpawnPolicy(o)
	if err != nil {
		t.Fatal(err)
	}
	if a.dir == b.dir {
		t.Fatal("sibling namespace collision")
	}
	if _, err := NewSpawnPolicy(o); err == nil {
		t.Fatal("reused host accepted")
	}
	bin, err := a.workerBinary()
	if err != nil || bin != filepath.Join(o.GenerationDir, cercanoBinaryName) {
		t.Fatalf("binary %s: %v", bin, err)
	}
	sock, pid, err := a.paths("conversation", 1)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(sock) != a.dir || filepath.Dir(pid) != a.dir {
		t.Fatal("unscoped paths")
	}
	// A missing pinned binary must fail even with a plausible PATH candidate.
	fallback := t.TempDir()
	if err := os.WriteFile(filepath.Join(fallback, cercanoBinaryName), []byte("never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fallback)
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if _, err := a.workerBinary(); err == nil {
		t.Fatal("missing pinned binary fell back")
	}
}

func TestPinnedPolicyRejectsPartialAndEscapingGeneration(t *testing.T) {
	for _, kind := range []string{"manifest", "symlink", "executable"} {
		t.Run(kind, func(t *testing.T) {
			o := policyFixture(t)
			switch kind {
			case "manifest":
				os.WriteFile(o.ManifestPath, []byte("{}"), 0600)
			case "symlink":
				bin := filepath.Join(o.GenerationDir, cercanoBinaryName)
				os.Remove(bin)
				outside := filepath.Join(o.ResourceRoot, "outside")
				os.WriteFile(outside, []byte("never executed"), 0700)
				if err := os.Symlink(outside, bin); err != nil {
					t.Fatal(err)
				}
			case "executable":
				os.Remove(filepath.Join(o.GenerationDir, cercanoBinaryName))
			}
			if _, err := NewSpawnPolicy(o); err == nil {
				t.Fatal("invalid generation accepted")
			}
		})
	}
}

func TestOwnedCleanupDoesNotScanSiblingResources(t *testing.T) {
	o := policyFixture(t)
	a, err := NewSpawnPolicy(o)
	if err != nil {
		t.Fatal(err)
	}
	o.HostInstance = "host-B"
	b, err := NewSpawnPolicy(o)
	if err != nil {
		t.Fatal(err)
	}
	// No child processes or live scans: pidfile contents must never be consulted.
	for _, dir := range []string{a.dir, b.dir} {
		if err := os.WriteFile(filepath.Join(dir, "unknown.pid"), []byte("123\n456\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	killedA, killedB := 0, 0
	makeRunner := func(killed *int) *workerRunner {
		pool := newWorkerPool(func(context.Context, string, uint64) (*workerHandle, error) {
			return &workerHandle{onKill: func() { *killed++ }}, nil
		})
		h, err := pool.Acquire(context.Background(), "conversation", 1)
		if err != nil {
			t.Fatal(err)
		}
		pool.Release("conversation", h, true)
		return &workerRunner{pool: pool}
	}
	w := makeRunner(&killedA)
	sibling := makeRunner(&killedB)
	defer sibling.CleanupOwnedWorkers()
	w.CleanupOwnedWorkers()
	w.CleanupOwnedWorkers()
	if killedA != 1 || killedB != 0 {
		t.Fatalf("owned/sibling teardown: %d/%d", killedA, killedB)
	}
	for _, dir := range []string{a.dir, b.dir} {
		if _, err := os.Stat(filepath.Join(dir, "unknown.pid")); err != nil {
			t.Fatal("cleanup touched unowned resources", err)
		}
	}
}

func TestPinnedCommandInheritsCredentialDisable(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	cmd := workerCommand("/fixture/generation/cercano", "/fixture/host/worker.sock")
	if cmd.Path != "/fixture/generation/cercano" || cmd.Env != nil {
		t.Fatal("command changed pin or inheritance")
	}
	found := false
	for _, value := range cmd.Environ() {
		if strings.HasPrefix(value, "CERCANO_DISABLE_CREDENTIALS=") {
			found = value == "CERCANO_DISABLE_CREDENTIALS=1"
		}
	}
	if !found {
		t.Fatal("credential-disable setting not inherited")
	}
	// Never Start: this test constructs only a command descriptor.
}
