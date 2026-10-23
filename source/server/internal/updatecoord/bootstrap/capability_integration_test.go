package bootstrap_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/bootstrap"
	"cercano/source/server/internal/updatecoord/oneshot"
)

// Exercise the actual production producer, not a test-binary JSON fixture.
// The only runtime invocation is Probe's offline version command.
func TestProbeBuiltCapabilityProducer(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating module")
	}
	module := filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	binary := filepath.Join(t.TempDir(), "cercano")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/cercano")
	build.Dir = module
	// Use cached dependencies only; building must not fetch from the network.
	build.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build producer: %v\n%s", err, out)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	image, err := bootstrap.Prepare(ctx, bootstrap.Request{
		Oneshot:   oneshot.Request{InstallID: "producer-test", OperationID: 1},
		Source:    bootstrap.Source{Path: binary, ExpectedSHA256: hex.EncodeToString(digest[:]), ExpectedLength: int64(len(data))},
		StateRoot: t.TempDir(), ForbiddenRoots: []string{filepath.Dir(binary)},
	})
	if err != nil {
		t.Fatal(err)
	}
	// A complete, credential-free child environment, with every writable user
	// root isolated. No inherited application configuration or fixture dispatch.
	var env, roots []string
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "TMPDIR", "TMP", "TEMP"} {
		root := t.TempDir()
		roots = append(roots, root)
		env = append(env, key+"="+root)
	}
	got, probeErr := bootstrap.Probe(ctx, image, bootstrap.ProbeOptions{Env: env})
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("offline version wrote to isolated root %s: %v", root, entries)
		}
	}
	if probeErr != nil {
		t.Fatalf("built producer rejected by public Probe: %+v: %v", got, probeErr)
	}
	if got.Status != bootstrap.ProbeProtocolNotReady || got.Protocol != 1 || got.ExecutionReady || !got.OfflinePreflight || got.Pid <= 0 {
		t.Fatalf("built producer capability: %+v", got)
	}
}
