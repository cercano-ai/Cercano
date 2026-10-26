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
	built := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cur := program{Path: newExe, ModTime: built}
	procs := []Process{
		{PID: self, PPID: 99, Executable: newExe, Args: []string{"cercano", "agent"}, Home: home, HomeKnown: true},
		{PID: 99, PPID: 50, Executable: "/opt/homebrew/Cellar/cercano/0.9.0/bin/cercano", Args: []string{"cercano"}, Home: home, HomeKnown: true}, // parent
		{PID: 50, PPID: 1, Executable: "/bin/zsh"},
		oldMCP(60, home), // included
		{PID: 61, PPID: 1, Executable: "/x/cercano", Args: []string{"cercano", "model-host"}, Home: home, HomeKnown: true},
		{PID: 62, PPID: 1, Executable: "/x/cercano", Args: []string{"cercano", "worker", "--socket", "s"}, Home: home, HomeKnown: true},
		oldMCP(63, "/Users/other"), // another home
		{PID: 64, PPID: 1, Executable: "/usr/local/bin/node", Args: []string{"node"}, Home: home, HomeKnown: true},
		{PID: 65, PPID: 1, Executable: newExe, Started: built.Add(time.Second), Args: []string{"cercano", "--mcp"}, Home: home, HomeKnown: true}, // this build, waiting to migrate
		{PID: 66, PPID: 1, Executable: `C:\cercano\cercano.exe`, Args: []string{"cercano.exe", "--mcp"}},                                         // environment unreadable: included
		{PID: 67, PPID: 1, Executable: "/x/cercano", Args: []string{"cercano", "--mcp"}, HomeKnown: true},                                        // no HOME: uses no home
		{PID: 68, PPID: 1, Executable: "/x/cercano (deleted)", Args: []string{"cercano", "--mcp"}, Home: home + "/", HomeKnown: true},
		{PID: 69, PPID: 1, Executable: newExe, Started: built.Add(-time.Hour), Args: []string{"cercano", "--mcp"}, Home: home, HomeKnown: true}, // older build replaced in place
		{PID: 70, PPID: 1, ExecutableGone: true, Name: "cercano", Args: []string{"cercano", "--mcp"}, Home: home, HomeKnown: true},              // keg deleted
		{PID: 71, PPID: 1, ExecutableGone: true, Name: "cercano-cli", Args: []string{"cercano-cli"}, Home: home, HomeKnown: true},
		{PID: 72, PPID: 1, ExecutableGone: true, Name: "cercano", Args: []string{"cercano", "model-host"}, Home: home, HomeKnown: true},
		{PID: 73, PPID: 1, ExecutableGone: true, Name: "cercano"}, // environment unreadable: included
	}
	got := otherCercanoPIDs(procs, self, home, cur)
	if want := []int{60, 66, 68, 69, 70, 73}; !reflect.DeepEqual(got, want) {
		t.Fatalf("otherCercanoPIDs = %v, want %v", got, want)
	}
}

func TestSameBuild(t *testing.T) {
	exe := "/usr/local/bin/cercano"
	built := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cur := program{Path: exe, ModTime: built}
	for _, tt := range []struct {
		name string
		p    Process
		cur  program
		want bool
	}{
		{"started after the file was written", Process{Executable: exe, Started: built.Add(time.Millisecond)}, cur, true},
		{"started as the file was written", Process{Executable: exe, Started: built}, cur, true},
		{"started before: older build replaced in place", Process{Executable: exe, Started: built.Add(-time.Millisecond)}, cur, false},
		{"another path", Process{Executable: "/opt/homebrew/Cellar/cercano/0.9/bin/cercano", Started: built.Add(time.Hour)}, cur, false},
		{"executable gone", Process{ExecutableGone: true, Name: "cercano", Started: built.Add(time.Hour)}, cur, false},
		{"executable gone, this build's path unknown", Process{ExecutableGone: true, Name: "cercano"}, program{}, false},
		{"executable gone, with a stale path", Process{Executable: exe, ExecutableGone: true, Started: built.Add(time.Hour)}, cur, false},
		{"start unknown (Linux)", Process{Executable: exe}, cur, true},
		{"modification time unknown", Process{Executable: exe, Started: built}, program{Path: exe}, true},
		{"this build's path unknown", Process{Executable: exe, Started: built}, program{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameBuild(tt.p, tt.cur); got != tt.want {
				t.Fatalf("sameBuild(%+v, %+v) = %v, want %v", tt.p, tt.cur, got, tt.want)
			}
		})
	}
}

func TestCurrentProgramReadsModificationTime(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "cercano")
	writeFile(t, exe, "binary")
	built := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(exe, built, built); err != nil {
		t.Fatal(err)
	}
	self := 4242
	if got := currentProgram([]Process{{PID: 1}, {PID: self, Executable: exe}}, self); got.Path != exe || !got.ModTime.Equal(built) {
		t.Errorf("currentProgram = %+v, want %s modified %v", got, exe, built)
	}
	if got := currentProgram([]Process{{PID: self, ExecutableGone: true, Name: "cercano"}}, self); got != (program{}) {
		t.Errorf("currentProgram with this executable gone = %+v, want none", got)
	}
	missing := exe + ".missing"
	if got := currentProgram([]Process{{PID: self, Executable: missing}}, self); got.Path != missing || !got.ModTime.IsZero() {
		t.Errorf("currentProgram for an unreadable file = %+v, want the path without a time", got)
	}
}

func TestIsCercanoProcess(t *testing.T) {
	for _, tt := range []struct {
		p    Process
		want bool
	}{
		{Process{Executable: "/opt/homebrew/Cellar/cercano/1.0/bin/cercano"}, true},
		{Process{Executable: "/x/cercano (deleted)"}, true},
		{Process{Executable: `C:\Cercano\CERCANO.EXE`}, true},
		{Process{Executable: "/x/cercano-cli"}, false},
		{Process{ExecutableGone: true, Name: "cercano"}, true},
		{Process{ExecutableGone: true, Name: "cercano-cli"}, false},
		{Process{ExecutableGone: true, Name: "Cercano"}, false},
		{Process{Name: "cercano"}, false}, // path unknown for another reason
	} {
		if got := isCercanoProcess(tt.p); got != tt.want {
			t.Errorf("isCercanoProcess(%+v) = %v, want %v", tt.p, got, tt.want)
		}
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
