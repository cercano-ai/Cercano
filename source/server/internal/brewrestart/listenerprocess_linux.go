//go:build linux

package brewrestart

import (
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// ListenerProcess finds the socket inodes listening for endpoint in
// /proc/net/tcp{,6}, then the same-user process holding one of them.
func ListenerProcess(endpoint netip.AddrPort) (Identity, bool, error) {
	inodes := map[string]bool{}
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(table)
		if err != nil {
			continue
		}
		for inode := range parseProcNetTCP(data, endpoint) {
			inodes[inode] = true
		}
	}
	if len(inodes) == 0 {
		return Identity{}, false, nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return Identity{}, false, err
	}
	uid := uint32(os.Getuid())
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
		fds, err := os.ReadDir(filepath.Join(dir, "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(dir, "fd", fd.Name()))
			if err != nil || len(link) < 9 || link[:8] != "socket:[" {
				continue
			}
			if inodes[link[8:len(link)-1]] {
				id, err := procIdentity(pid, uid)
				if err != nil {
					break
				}
				return id, true, nil
			}
		}
	}
	return Identity{}, false, nil
}

func procIdentity(pid int, uid uint32) (Identity, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Identity{}, err
	}
	_, start, err := parseProcStat(data)
	if err != nil {
		return Identity{}, err
	}
	exe, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	return Identity{PID: pid, UID: uid, Executable: exe, StartSeconds: start}, nil
}

// ProcessExited reads /proc/<pid>/stat: missing, a zombie, or a different
// start time all mean the original process is gone.
func ProcessExited(id Identity) (bool, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", id.PID))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
			return true, nil
		}
		return false, err
	}
	state, start, err := parseProcStat(data)
	if err != nil {
		return false, err
	}
	return state == 'Z' || state == 'X' || start != id.StartSeconds, nil
}
