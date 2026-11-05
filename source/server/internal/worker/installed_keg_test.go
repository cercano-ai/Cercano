package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/runner"
	"cercano/source/server/pkg/config"
)

// Exercise the production runner's pool spawn closure, not exec.Command on a
// manually selected path. The fixture is intentionally not an agent or model:
// it records dispatch and exits before the worker handshake.
func TestInstalledKegPoolPin(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", root) // all pool resources stay inside the fixture
	marker := filepath.Join(root, "launched")
	makeKeg := func(version string) string {
		bin := filepath.Join(root, "Cellar", "cercano", version, "bin", "cercano")
		if err := os.MkdirAll(filepath.Dir(bin), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '"+version+"' > '"+marker+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
		return bin
	}
	a, b := makeKeg("1.0"), makeKeg("2.0")
	alias := filepath.Join(root, "cercano")
	if err := os.Symlink(a, alias); err != nil {
		t.Fatal(err)
	}
	policy, selected, err := SelectInstalledKegPolicy(alias)
	if err != nil || !selected {
		t.Fatalf("selection: %v, %v", selected, err)
	}
	if policy.binary != a {
		t.Fatalf("selected %s, want %s", policy.binary, a)
	}
	if err := policy.ValidatePrepared(); err == nil {
		t.Fatal("installed policy must not grant prepared authority")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, alias); err != nil {
		t.Fatal(err)
	}
	r := NewWorkerRunnerWithPolicy(policy, nil, cfgsvc.New("", config.Config{WorkerIdleTimeoutSeconds: -1}, nil), nil, nil, nil, nil, nil, nil).(*workerRunner)
	defer r.pool.Shutdown()
	if r.prepared {
		t.Fatal("installed policy acquired prepared credential/session behavior")
	}
	acquire := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := r.RunTurn(ctx, runner.Request{ConversationID: "fixture", Gen: 1}, nil, nil, nil)
		return err
	}
	if err := acquire(); err == nil {
		t.Fatal("fixture must not complete handshake")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "1.0" {
		t.Fatalf("launched %q: %v", got, err)
	}
	if err := os.WriteFile(a, []byte("#!/bin/sh\nexit 42\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := acquire(); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("replacement not rejected: %v", err)
	}
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	if err := acquire(); err == nil {
		t.Fatal("missing selected keg must fail closed")
	}
	// Neither rejection launched the new alias or performed PATH discovery.
	got, err = os.ReadFile(marker)
	if err != nil || string(got) != "1.0" {
		t.Fatalf("fail-closed marker %q: %v", got, err)
	}
}

func TestInstalledKegSelectionBoundaries(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "Cellar", "cercano", "1.0", "bin", "cercano")
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(binary, contents, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := NewInstalledKegPolicy(binary)
	if err != nil {
		t.Fatal(err)
	}
	// Selection requires no manifest, inventory, models, llama, or copied bundle.
	if err := p.ValidateInstalled(); err != nil {
		t.Fatal(err)
	}
	replacement := binary + ".new"
	if err := os.WriteFile(replacement, contents, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, binary); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateInstalled(); err == nil {
		t.Fatal("identical-byte inode replacement must fail closed")
	}
	dev := filepath.Join(root, "cercano")
	if err := os.WriteFile(dev, contents, 0700); err != nil {
		t.Fatal(err)
	}
	if _, selected, err := SelectInstalledKegPolicy(dev); err != nil || selected {
		t.Fatalf("development executable selected as keg: %v, %v", selected, err)
	}
	if _, err := NewInstalledKegPolicy(dev); err == nil {
		t.Fatal("explicit installed policy accepted non-keg")
	}
}
