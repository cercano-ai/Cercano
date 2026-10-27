package worker

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shortPinnedFixtureRoot must not inherit TMPDIR or t.TempDir's test-name
// components: the canonical pinned resource root has a 38-byte socket budget.
func shortPinnedFixtureRoot(t *testing.T) string {
	t.Helper()
	allocated, err := os.MkdirTemp("/tmp", "wp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(allocated); err != nil {
			t.Errorf("remove pinned fixture root: %v", err)
		}
	})
	base, err := filepath.EvalSymlinks(allocated)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(base) != parent || !strings.HasPrefix(filepath.Base(base), "wp-") {
		t.Fatalf("pinned fixture root escaped private /tmp allocation: %q", base)
	}
	info, err := os.Stat(base)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || len(base) > 38 {
		t.Fatalf("pinned fixture root must be private and short: %q (%v)", base, info.Mode())
	}
	return base
}

func policyFixture(t *testing.T) SpawnOptions {
	t.Helper()
	// No executable fixture is started.
	base := shortPinnedFixtureRoot(t)
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

func TestPinnedPolicySocketBudget(t *testing.T) {
	for _, rootBytes := range []int{38, 39} {
		t.Run(fmt.Sprint(rootBytes), func(t *testing.T) {
			o := policyFixture(t)
			base := filepath.Dir(o.ResourceRoot)
			o.ResourceRoot = filepath.Join(base, strings.Repeat("r", rootBytes-len(base)-1))
			if err := os.Mkdir(o.ResourceRoot, 0700); err != nil {
				t.Fatal(err)
			}
			p, err := NewSpawnPolicy(o)
			if rootBytes == 39 {
				if err == nil || !strings.Contains(err.Error(), "103") {
					t.Fatalf("oversized root must fail early with socket limit: %v", err)
				}
				entries, readErr := os.ReadDir(o.ResourceRoot)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("rejected policy mutated root: %v, %v", entries, readErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			sock, pid, err := p.paths(strings.Repeat("conversation", 100), ^uint64(0))
			if err != nil {
				t.Fatal(err)
			}
			if len(sock) != 103 || len(filepath.Base(sock)) != 37 || strings.Contains(sock, "conversation") {
				t.Fatalf("unbounded/nonopaque socket: %q (%d bytes)", sock, len(sock))
			}
			listener, listenErr := net.Listen("unix", sock)
			if listenErr != nil {
				t.Fatalf("boundary socket cannot bind: %v", listenErr)
			}
			listener.Close()
			if strings.TrimSuffix(sock, ".sock") != strings.TrimSuffix(pid, ".pid") {
				t.Fatal("socket/pid identity differs")
			}
		})
	}
}

func TestPinnedSpawnRejectsLongPathBeforeMutation(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	o := policyFixture(t)
	p, err := NewSpawnPolicy(o)
	if err != nil {
		t.Fatal(err)
	}
	// Defense in depth for the spawn path: the fake executable must never be
	// started, nor may a rejected path create a directory, socket, pid or log.
	p.dir = filepath.Join(o.ResourceRoot, strings.Repeat("x", MaxWorkerSocketPathBytes))
	_, err = spawnWorkerWithPolicy(context.Background(), "conversation", 1, p)
	if err == nil || !strings.Contains(err.Error(), "paths:") || !strings.Contains(err.Error(), "103") {
		t.Fatalf("expected pre-launch path rejection: %v", err)
	}
	if _, err := os.Stat(p.dir); !os.IsNotExist(err) {
		t.Fatalf("rejected spawn mutated namespace: %v", err)
	}
}

func TestPinnedPolicyCountsResolvedRootBytes(t *testing.T) {
	o := policyFixture(t)
	base := filepath.Dir(o.ResourceRoot)
	// 39 bytes but only 38 runes: a rune-count check would wrongly accept it.
	o.ResourceRoot = filepath.Join(base, strings.Repeat("r", 39-len(base)-3)+"é")
	if err := os.Mkdir(o.ResourceRoot, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSpawnPolicy(o); err == nil || !strings.Contains(err.Error(), "39 bytes") {
		t.Fatalf("multibyte oversized root accepted: %v", err)
	}
}

func TestLegacyWorkerPathsKeepNaming(t *testing.T) {
	o := policyFixture(t)
	t.Setenv("TMPDIR", o.ResourceRoot)
	t.Setenv("TMP", o.ResourceRoot)
	t.Setenv("TEMP", o.ResourceRoot)
	sock, pid, err := workerPaths("ordinary conversation", 42)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(sock) != filepath.Join(os.TempDir(), workerSocketDir) || !strings.HasPrefix(filepath.Base(sock), "ordinary_convers-42-") || !strings.HasSuffix(sock, ".sock") || !strings.HasSuffix(pid, ".pid") {
		t.Fatalf("legacy paths changed: %s, %s", sock, pid)
	}
}
