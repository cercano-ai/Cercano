//go:build linux

package brewrestart

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// ListUserProcesses lists the current user's processes with their parent
// and executable from /proc. For those whose executable details accepts it
// also reads the arguments and HOME. Processes that exit while being read
// are skipped.
func ListUserProcesses(details func(executable string) bool) ([]UserProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	uid := uint32(os.Getuid())
	var out []UserProcess
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		dir := filepath.Join("/proc", e.Name())
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}
		if st, ok := info.Sys().(*syscall.Stat_t); !ok || st.Uid != uid {
			continue
		}
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		ppid, err := parseProcStatPPID(stat)
		if err != nil {
			continue
		}
		exe, _ := os.Readlink(filepath.Join(dir, "exe"))
		p := UserProcess{PID: pid, PPID: ppid, Executable: exe}
		if exe != "" && details != nil && details(exe) {
			if cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
				p.Args = splitNUL(cmdline)
			}
			if environ, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
				env := splitNUL(environ)
				p.Home, p.HomeKnown = envHome(env), true
				clear(env)
				clear(environ)
			}
		}
		out = append(out, p)
	}
	return out, nil
}
