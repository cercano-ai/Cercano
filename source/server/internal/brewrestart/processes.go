package brewrestart

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// UserProcess is one of the current user's processes as the kernel reports
// it. It is a point-in-time observation; the process may exit at any moment.
type UserProcess struct {
	PID, PPID int
	// Executable is the kernel's path of the program, "" when unreadable. On
	// Linux it ends in " (deleted)" once the file was replaced or removed.
	Executable string
	// ExecutableGone is true on macOS when the process runs but the kernel
	// no longer gives its executable's path, as happens once the file or its
	// directory was removed (Homebrew's cleanup deletes old kegs). Executable
	// is then "" and only Name identifies the program.
	ExecutableGone bool
	// Name is the kernel's name for the process on macOS: the file name the
	// program was started from, truncated to 32 bytes.
	Name string
	// Started is when the process started (on macOS when it was forked); zero
	// when unknown, as on Linux.
	Started time.Time
	// Args, Home and HomeKnown are read only for processes the caller asked
	// details for. Home is HOME from the environment;
	// HomeKnown is false when the environment could not be read (and true,
	// with Home empty, when it was read and has no HOME). Nothing else from
	// the environment is kept: it can hold credentials.
	Args      []string
	Home      string
	HomeKnown bool
}

// envHome returns HOME from env.
func envHome(env []string) string {
	for _, entry := range env {
		if v, ok := strings.CutPrefix(entry, "HOME="); ok {
			return v
		}
	}
	return ""
}

// splitNUL splits NUL-terminated strings, such as /proc/<pid>/cmdline.
func splitNUL(data []byte) []string {
	data = bytes.TrimSuffix(data, []byte{0})
	if len(data) == 0 {
		return nil
	}
	return strings.Split(string(data), "\x00")
}

// parseProcStatPPID returns the parent PID (field 4) of a /proc/<pid>/stat
// line. comm (field 2) may contain spaces and parentheses.
func parseProcStatPPID(data []byte) (int, error) {
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return 0, fmt.Errorf("malformed stat line")
	}
	fields := strings.Fields(string(data[i+1:]))
	if len(fields) < 2 {
		return 0, fmt.Errorf("malformed stat line")
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("malformed stat parent PID: %w", err)
	}
	return ppid, nil
}
