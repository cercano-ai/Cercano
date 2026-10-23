package profilemigrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/paths"
)

func useProcessWait(t *testing.T, wait time.Duration) {
	t.Helper()
	prevWait, prevPoll, prevNotice := otherProcessWait, otherProcessPoll, slowExitNotice
	otherProcessWait, otherProcessPoll, slowExitNotice = wait, 10*time.Millisecond, 30*time.Millisecond
	t.Cleanup(func() { otherProcessWait, otherProcessPoll, slowExitNotice = prevWait, prevPoll, prevNotice })
}

// oldMCP is a Cercano started before profiles, serving MCP for this home.
func oldMCP(pid int, home string) Process {
	return Process{PID: pid, PPID: 1, Executable: "/opt/homebrew/Cellar/cercano/0.9.0/bin/cercano", Args: []string{"cercano", "--mcp"}, Home: home, HomeKnown: true}
}

func TestOtherCercanoPIDsExclusions(t *testing.T) {
	home := "/Users/someone"
	self := os.Getpid()
	newExe := "/opt/homebrew/Cellar/cercano/1.0.0/bin/cercano"
	procs := []Process{
		{PID: self, PPID: 99, Executable: newExe, Args: []string{"cercano", "agent"}, Home: home, HomeKnown: true},
		{PID: 99, PPID: 50, Executable: "/opt/homebrew/Cellar/cercano/0.9.0/bin/cercano", Args: []string{"cercano"}, Home: home, HomeKnown: true}, // parent
		{PID: 50, PPID: 1, Executable: "/bin/zsh"},
		oldMCP(60, home), // included
		{PID: 61, PPID: 1, Executable: "/x/cercano", Args: []string{"cercano", "model-host"}, Home: home, HomeKnown: true},
		{PID: 62, PPID: 1, Executable: "/x/cercano", Args: []string{"cercano", "worker", "--socket", "s"}, Home: home, HomeKnown: true},
		oldMCP(63, "/Users/other"), // another home
		{PID: 64, PPID: 1, Executable: "/usr/local/bin/node", Args: []string{"node"}, Home: home, HomeKnown: true},
		{PID: 65, PPID: 1, Executable: newExe, Args: []string{"cercano", "--mcp"}, Home: home, HomeKnown: true}, // this build, waiting to migrate
		{PID: 66, PPID: 1, Executable: `C:\cercano\cercano.exe`, Args: []string{"cercano.exe", "--mcp"}},        // environment unreadable: included
		{PID: 67, PPID: 1, Executable: "/x/cercano", Args: []string{"cercano", "--mcp"}, HomeKnown: true},       // no HOME: uses no home
		{PID: 68, PPID: 1, Executable: "/x/cercano (deleted)", Args: []string{"cercano", "--mcp"}, Home: home + "/", HomeKnown: true},
	}
	got := otherCercanoPIDs(procs, self, home)
	if want := []int{60, 66, 68}; !reflect.DeepEqual(got, want) {
		t.Fatalf("otherCercanoPIDs = %v, want %v", got, want)
	}
}

func TestMigrateWaitsForOtherCercanoToExit(t *testing.T) {
	home, out := setHome(t)
	useProcessWait(t, 5*time.Second)
	legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
	writeFile(t, legacy, "port: \"50052\"\n")
	var calls atomic.Int32
	opts := noAgent(home)
	opts.ListProcesses = func() ([]Process, error) {
		if calls.Add(1) <= 6 {
			if !exists(legacy) {
				t.Error("a file moved while another Cercano was running")
			}
			return []Process{oldMCP(4242, home)}, nil
		}
		return nil, nil
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(filepath.Join(paths.ProfileDir("default"), "config.yaml")) || !exists(markerPath()) {
		t.Fatal("migration did not complete after the other Cercano exited")
	}
	if n := strings.Count(out.String(), "4242"); n != 1 || !strings.Contains(out.String(), "Claude Code") {
		t.Errorf("want one notice naming pid 4242 and Claude Code, got %q", out.String())
	}
}

func TestMigrateRefusesWhileOtherCercanoRuns(t *testing.T) {
	home, _ := setHome(t)
	useProcessWait(t, 150*time.Millisecond)
	legacy := filepath.Join(paths.ConfigRoot(), "config.yaml")
	writeFile(t, legacy, "port: \"50052\"\n")
	opts := noAgent(home)
	opts.ListProcesses = func() ([]Process, error) { return []Process{oldMCP(4242, home), oldMCP(4343, home)}, nil }

	err := Run(context.Background(), opts)
	want := "Cercano is still running in another program (pids 4242, 4343). Close editors or Claude Code sessions that use Cercano, then try again."
	if err == nil || err.Error() != want {
		t.Fatalf("Run err = %v, want %q", err, want)
	}
	if !exists(legacy) || exists(filepath.Join(paths.ProfileDir("default"), "config.yaml")) {
		t.Error("files moved while another Cercano was running")
	}
	if exists(markerPath()) {
		t.Error("marker written while another Cercano was running")
	}
}

func TestMigrateDoesNotWaitWithoutOtherCercano(t *testing.T) {
	useProcessWait(t, 5*time.Second)
	for name, list := range map[string]func() ([]Process, error){
		"none":          func() ([]Process, error) { return []Process{oldMCP(70, "/Users/other")}, nil },
		"listing fails": func() ([]Process, error) { return nil, errors.ErrUnsupported },
	} {
		t.Run(name, func(t *testing.T) {
			home, out := setHome(t)
			writeFile(t, filepath.Join(paths.ConfigRoot(), "config.yaml"), "port: \"50052\"\n")
			opts := noAgent(home)
			opts.ListProcesses = list
			start := time.Now()
			if err := Run(context.Background(), opts); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if time.Since(start) > time.Second || strings.Contains(out.String(), "Waiting") {
				t.Errorf("Run waited (%s, output %q)", time.Since(start), out.String())
			}
			if !exists(markerPath()) {
				t.Error("marker not written")
			}
		})
	}
}

func TestProcessesNotListedWithoutLegacyData(t *testing.T) {
	home, _ := setHome(t)
	opts := noAgent(home)
	opts.ListProcesses = func() ([]Process, error) {
		t.Error("processes listed on a fresh install")
		return nil, nil
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
