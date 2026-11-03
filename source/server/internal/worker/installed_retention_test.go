package worker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This helper handles the real worker command's arguments without starting an
// agent, RPC server or model. All executable and asset bytes belong to TempDir.
func init() {
	if os.Getenv("CERCANO_INSTALLED_RETENTION_HELPER") != "1" {
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "worker" {
		os.Exit(90)
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(91)
	}
	root := filepath.Dir(exe)
	marker, err := os.ReadFile(filepath.Join(root, "generation"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(92)
	}
	asset, err := os.ReadFile(filepath.Join(root, string(marker)+"-only.resource"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(93)
	}
	fmt.Printf("generation=%s resource=%s\n", marker, asset)
	os.Exit(0)
}

// Characterization of the installed dependency gap, NOT upgrade acceptance.
// NewSpawnPolicy is the existing worker admission seam, not installed host or
// session admission. The fake manager alias models selection only; no retention
// is implemented by this fixture and no real package manager is invoked.
func TestInstalledRetentionCharacterization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix manager alias fixture; native Windows acceptance deferred")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "CERCANO_SEARCH_SCRIPT_DIR"} {
		t.Setenv(key, root)
	}
	resourceRoot := shortPinnedFixtureRoot(t) // sockets only, never binaries/helpers
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	makePackage := func(marker string) string {
		dir := filepath.Join(root, "manager", marker)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		src, err := os.Open(exe)
		if err != nil {
			t.Fatal(err)
		}
		defer src.Close()
		dst, err := os.OpenFile(filepath.Join(dir, cercanoBinaryName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			t.Fatal(err)
		}
		_, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatalf("copy helper: %v / %v", copyErr, closeErr)
		}
		for name, data := range map[string]string{"manifest.json": `{"generation":"` + marker + `"}`, "generation": marker, marker + "-only.resource": marker + " asset"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	a, b := makePackage("A"), makePackage("B")
	selected := filepath.Join(root, "current")
	selectPackage := func(dir string) {
		if err := os.Remove(selected); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Symlink(dir, selected); err != nil {
			t.Fatal(err)
		}
	}
	admit := func(host string) SpawnPolicy {
		dir, err := filepath.EvalSymlinks(selected)
		if err != nil {
			t.Fatal(err)
		}
		p, err := NewSpawnPolicy(SpawnOptions{GenerationDir: dir, ManifestPath: filepath.Join(dir, "manifest.json"), HostInstance: host, ResourceRoot: resourceRoot})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	run := func(p SpawnPolicy) (string, error) {
		bin, err := p.workerBinary()
		if err != nil {
			return "", err
		}
		cmd := workerCommandWithPolicy(bin, filepath.Join(p.dir, "probe.sock"), p)
		cmd.Env = []string{"CERCANO_INSTALLED_RETENTION_HELPER=1", "HOME=" + root, "TMPDIR=" + root, "PATH=" + b}
		// Keep the actual command construction seam; cap this test-owned child.
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Start(); err != nil {
			return "", err
		}
		timer := time.AfterFunc(10*time.Second, func() { _ = cmd.Process.Kill() })
		defer timer.Stop()
		err = cmd.Wait()
		return output.String(), err
	}
	requireMarker := func(p SpawnPolicy, marker string) {
		out, err := run(p)
		if err != nil || out != "generation="+marker+" resource="+marker+" asset\n" {
			t.Fatalf("worker %s: %v, output %q", marker, err, out)
		}
		t.Logf("worker: %s", strings.TrimSpace(out))
	}
	selectPackage(a)
	pinnedA := admit("host-A")
	requireMarker(pinnedA, "A")
	selectPackage(b)
	pinnedB := admit("host-B")
	requireMarker(pinnedB, "B")
	requireMarker(pinnedA, "A")
	// A-only asset loss is not checked by the executable/manifest digests.
	if err := os.Remove(filepath.Join(a, "A-only.resource")); err != nil {
		t.Fatal(err)
	}
	if _, err := pinnedA.workerBinary(); err != nil {
		t.Fatalf("unexpected asset validation: %v", err)
	}
	out, err := run(pinnedA)
	if err == nil || !strings.Contains(out, "A-only.resource") || !strings.Contains(out, "no such file") {
		t.Fatalf("missing asset probe: %v, %q", err, out)
	}
	t.Logf("asset gap: selection succeeds; helper fails: %v: %s", err, strings.TrimSpace(out))
	// Removal is confined to this test's manager-owned A fixture.
	if err := os.RemoveAll(a); err != nil {
		t.Fatal(err)
	}
	// Passing reproduction of the observed failure, not a retention assertion.
	out, err = run(pinnedA)
	if err == nil || !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), a) || out != "" {
		t.Fatalf("expected missing A dependency without fallback to B: %v, %q", err, out)
	}
	t.Logf("retention gap: later A worker refused before launch: %v", err)
	requireMarker(pinnedB, "B")
}
