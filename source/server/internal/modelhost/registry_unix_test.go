//go:build unix

package modelhost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRunDirRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ensureRunDir(link); err == nil {
		t.Fatal("symlinked run dir accepted")
	}
	if err := WriteRecord(link, Record{PID: 1}); err == nil {
		t.Fatal("WriteRecord into symlinked dir succeeded")
	}
}

func TestEnsureRunDirTightensOpenDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureRunDir(dir); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v, want 0700", fi.Mode().Perm())
	}
}

func TestEnsureRunDirRejectsFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	os.WriteFile(p, nil, 0o600)
	if err := ensureRunDir(p); err == nil {
		t.Fatal("file accepted as run dir")
	}
}

func TestRunDirErrorsWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	if d, err := RunDir(); err == nil {
		t.Fatalf("RunDir = %q, want error", d)
	}
	t.Setenv("HOME", "/home/x")
	if d, err := RunDir(); err != nil || d != "/home/x/.cercano/run/modelhost" {
		t.Fatalf("RunDir = %q, %v", d, err)
	}
}

func TestLaunchBinaryRefusesSymlinkedLog(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	if err := ensureRunDir(dir); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("keep"), 0o600)
	if err := os.Symlink(victim, filepath.Join(dir, "modelhost.log")); err != nil {
		t.Fatal(err)
	}
	if err := LaunchBinary("/bin/true", dir)(nil); err == nil {
		t.Fatal("LaunchBinary followed a symlinked log")
	}
}
