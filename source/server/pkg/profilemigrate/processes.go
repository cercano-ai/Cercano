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
	return brewrestart.ListUserProcesses(isCercanoExecutable)
}

// helperCommands are the subcommands of processes that a profile-aware
// Cercano starts for itself; they never open profile files.
var helperCommands = map[string]bool{"model-host": true, "worker": true}

// otherCercanoPIDs returns, sorted, the PIDs among procs that may be a
// Cercano started before profiles with this home's legacy files open. It
// leaves out: the process self and its parents (a launcher that started it),
// helper subcommands, processes running the same executable as self (this
// build, waiting for the migration lock), and processes whose environment
// was read and does not name this home.
func otherCercanoPIDs(procs []Process, self int, home string) []int {
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
	selfExe := byPID[self].Executable
	var pids []int
	for _, p := range procs {
		switch {
		case excluded[p.PID], !isCercanoExecutable(p.Executable):
			continue
		case len(p.Args) > 1 && helperCommands[p.Args[1]]:
			continue
		case selfExe != "" && p.Executable == selfExe:
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
		return otherCercanoPIDs(procs, os.Getpid(), home)
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
