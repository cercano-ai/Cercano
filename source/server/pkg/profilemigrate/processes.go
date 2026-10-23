package profilemigrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cercano/source/server/internal/brewrestart"
)

// Process is a running program as the check for other Cercano processes
// sees it.
type Process = brewrestart.UserProcess

var (
	// otherProcessWait bounds the wait for other Cercano processes to exit.
	otherProcessWait = 60 * time.Second
	// otherProcessPoll is the interval between process listings meanwhile.
	otherProcessPoll = 250 * time.Millisecond
)

// listUserProcesses lists the user's processes, reading the arguments and
// HOME of cercano executables only.
func listUserProcesses() ([]Process, error) {
	return brewrestart.ListUserProcesses(isCercanoProcess)
}

// isCercanoProcess reports whether p runs a cercano binary: by its
// executable path, or, when the kernel can no longer give that path (macOS,
// once the file or its directory was removed, as Homebrew's cleanup does to
// an old keg), by its process name.
func isCercanoProcess(p Process) bool {
	return isCercanoImage(p.Executable, p.ExecutableGone, p.Name)
}

// isCercanoImage is isCercanoProcess for an executable path, whether that
// path is gone, and the kernel's process name. The macOS name is at least
// the 16 characters of pbi_comm, so "cercano" is never truncated.
func isCercanoImage(executable string, gone bool, name string) bool {
	if gone {
		return name == "cercano"
	}
	return isCercanoExecutable(executable)
}

// program is the executable file this process runs.
type program struct {
	Path    string    // the kernel's path for it, as for other processes
	ModTime time.Time // zero when the file cannot be read
}

// currentProgram returns the program self runs, as listed in procs, with the
// file's modification time. It is the kernel's path rather than
// os.Executable: that can name a symlink (Homebrew's bin/cercano) that an
// upgrade has since repointed. It is empty when self's path is unknown.
func currentProgram(procs []Process, self int) program {
	for _, p := range procs {
		if p.PID != self || p.ExecutableGone || p.Executable == "" {
			continue
		}
		cur := program{Path: p.Executable}
		if info, err := os.Stat(p.Executable); err == nil {
			cur.ModTime = info.ModTime()
		}
		return cur
	}
	return program{}
}

// sameBuild reports whether p runs this very build: the same executable
// file, started at or after it was last written. macOS keeps reporting the
// original path for a process whose executable was replaced in place (go
// install, the dev launcher's rename, an installer copying into
// /usr/local/bin), so a process that started before the file was written
// runs an older build and is not this one. A process whose executable is
// gone never is. When either time is unknown the path decides; on Linux a
// replaced executable reads "<path> (deleted)" and so already differs.
func sameBuild(p Process, cur program) bool {
	if p.ExecutableGone || cur.Path == "" || p.Executable != cur.Path {
		return false
	}
	if p.Started.IsZero() || cur.ModTime.IsZero() {
		return true
	}
	return !p.Started.Before(cur.ModTime)
}

// helperCommands are the subcommands of processes that a profile-aware
// Cercano starts for itself; they never open profile files.
var helperCommands = map[string]bool{"model-host": true, "worker": true}

// otherCercanoPIDs returns, sorted, the PIDs among procs that may be a
// Cercano started before profiles with this home's legacy files open. It
// leaves out: the process self and its parents (a launcher that started it),
// helper subcommands, processes running this same build cur (see sameBuild:
// they wait for the migration lock), and processes whose environment was
// read and does not name this home.
func otherCercanoPIDs(procs []Process, self int, home string, cur program) []int {
	byPID := make(map[int]Process, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	excluded := map[int]bool{self: true, os.Getppid(): true}
	for pid, seen := self, map[int]bool{}; !seen[pid]; {
		seen[pid] = true
		p, ok := byPID[pid]
		if !ok || p.PPID <= 0 {
			break
		}
		excluded[p.PPID] = true
		pid = p.PPID
	}
	var pids []int
	for _, p := range procs {
		switch {
		case excluded[p.PID], !isCercanoProcess(p):
			continue
		case len(p.Args) > 1 && helperCommands[p.Args[1]]:
			continue
		case sameBuild(p, cur):
			continue
		case p.HomeKnown && !sameDir(p.Home, home):
			continue
		}
		pids = append(pids, p.PID)
	}
	sort.Ints(pids)
	return pids
}

// sameDir reports whether a and b name the same directory.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

func formatPIDs(pids []int) string {
	parts := make([]string, len(pids))
	for i, pid := range pids {
		parts[i] = strconv.Itoa(pid)
	}
	return strings.Join(parts, ", ")
}

// waitForOtherCercano returns once no other Cercano process that may have
// this home's legacy files open is running. Such a process (a pre-profile
// `cercano --mcp` ran its own host on another port) would keep writing to
// files after they move, and on Windows the move fails. It says once that
// it is waiting after slowExitNotice, and gives up after otherProcessWait.
// When processes cannot be listed the check is skipped.
func waitForOtherCercano(ctx context.Context, opts Options, home string) error {
	if opts.ListProcesses == nil {
		return nil
	}
	find := func() []int {
		procs, err := opts.ListProcesses()
		if err != nil {
			return nil
		}
		return otherCercanoPIDs(procs, os.Getpid(), home, currentProgram(procs, os.Getpid()))
	}
	pids := find()
	start := time.Now()
	noticed := false
	for len(pids) > 0 {
		elapsed := time.Since(start)
		if elapsed >= otherProcessWait {
			return fmt.Errorf("Cercano is still running in another program (pids %s). Close editors or Claude Code sessions that use Cercano, then try again.", formatPIDs(pids))
		}
		if !noticed && elapsed >= slowExitNotice {
			fmt.Fprintf(logOut, "Waiting for Cercano running in other programs to exit (pids %s); close editors or Claude Code sessions that use Cercano MCP.\n", formatPIDs(pids))
			noticed = true
		}
		timer := time.NewTimer(otherProcessPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		pids = find()
	}
	return nil
}
