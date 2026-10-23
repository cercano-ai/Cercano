//go:build (darwin && cgo) || linux

package brewrestart

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestListUserProcessesFindsThisProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	asked := map[string]bool{}
	procs, err := ListUserProcesses(func(executable string) bool {
		asked[executable] = true
		return filepath.Base(executable) == filepath.Base(exe)
	})
	if err != nil {
		t.Fatalf("ListUserProcesses: %v", err)
	}
	for _, p := range procs {
		if p.PID != os.Getpid() {
			continue
		}
		if p.PPID != os.Getppid() {
			t.Errorf("PPID = %d, want %d", p.PPID, os.Getppid())
		}
		if filepath.Base(p.Executable) != filepath.Base(exe) {
			t.Errorf("Executable = %q, want %q", p.Executable, exe)
		}
		if !reflect.DeepEqual(p.Args, os.Args) {
			t.Errorf("Args = %q, want %q", p.Args, os.Args)
		}
		if !p.HomeKnown || p.Home != os.Getenv("HOME") {
			t.Errorf("Home = %q (known %v), want %q", p.Home, p.HomeKnown, os.Getenv("HOME"))
		}
		return
	}
	t.Fatalf("this process (%d) not listed among %d", os.Getpid(), len(procs))
}

func TestListUserProcessesReadsDetailsOnlyWhenAsked(t *testing.T) {
	procs, err := ListUserProcesses(func(string) bool { return false })
	if err != nil {
		t.Fatalf("ListUserProcesses: %v", err)
	}
	for _, p := range procs {
		if p.Args != nil || p.HomeKnown {
			t.Fatalf("details read for %d (%s) though not asked", p.PID, strings.TrimSpace(p.Executable))
		}
	}
}
