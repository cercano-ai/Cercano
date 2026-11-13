//go:build darwin && cgo

package brewrestart

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPreparedInventoryChild(t *testing.T) {
	if os.Getenv("PREPARED_INVENTORY_CHILD") != "1" {
		return
	}
	fmt.Println("ready")
	io.Copy(io.Discard, os.Stdin)
}

func TestPreparedGenerationInventory(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "store.db")
	f, err := os.Create(store)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// An unrelated process opening the database is outside this trust boundary.
	other := exec.Command("/bin/sh", "-c", "exec 3<\"$1\"; echo ready; read stop", "fixture", store)
	otherIn, _ := other.StdinPipe()
	otherOut, _ := other.StdoutPipe()
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { otherIn.Close(); other.Wait() }()
	if line, err := bufio.NewReader(otherOut).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("child readiness: %q %v", line, err)
	}
	if err := VerifyPreparedWritersAbsent(context.Background(), root, store); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "cercano")
	if err := os.WriteFile(binary, body, 0700); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestPreparedInventoryChild$")
	child.Env = append(os.Environ(), "PREPARED_INVENTORY_CHILD=1")
	childIn, _ := child.StdinPipe()
	childOut, _ := child.StdoutPipe()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { childIn.Close(); child.Wait() }()
	if line, err := bufio.NewReader(childOut).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("child readiness: %q %v", line, err)
	}
	if err := VerifyPreparedWritersAbsent(context.Background(), root, store); err == nil {
		t.Fatal("ambiguous generation process admitted")
	}
	// Kernel executable attribution must continue to refuse after unlink.
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPreparedWritersAbsent(context.Background(), root, store); err == nil {
		exe, argv, e := recoveryProcessArguments(child.Process.Pid)
		id, ie := Inspect(child.Process.Pid)
		t.Fatalf("unlinked generation process admitted: exe=%q argv=%q err=%v id=%+v inspect=%v", exe, argv, e, id, ie)
	}
}

func TestPreparedAbsenceClassification(t *testing.T) {
	for _, tc := range []struct {
		name, executable string
		cause            error
		present          bool
	}{
		{"present", "/prepared/cercano", nil, true},
		{"unrelated", "/other/cercano", nil, false},
		{"uncertain", "relative", nil, false},
		{"permission", "", syscall.EPERM, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyPreparedWritersAbsent(context.Background(), "/prepared", func() ([]int, error) { return []int{123}, nil }, func(int) (string, []string, error) { return tc.executable, nil, tc.cause }, func(int) (Identity, error) { return Identity{}, tc.cause })
			if errors.Is(err, ErrPreparedProcessPresent) != tc.present {
				t.Fatalf("classification: %v", err)
			}
			if tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Fatalf("lost cause: %v", err)
			}
			if tc.name == "uncertain" && err == nil {
				t.Fatal("uncertain admitted")
			}
		})
	}
}
