//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <sys/sysctl.h>
#include <sys/proc.h>
static int upgrade_status(int pid) {
 struct proc_bsdinfo info;
 if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, sizeof(info)) != sizeof(info)) return -1;
 return info.pbi_status;
}
static int upgrade_args(int pid, char *buf, size_t *size) {
 int mib[3] = { CTL_KERN, KERN_PROCARGS2, pid };
 return sysctl(mib, 3, buf, size, NULL, 0);
}
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// processRole is only a classifier AFTER kernel executable/start identity and
// ancestry checks. Neither argv nor a process name establishes ownership.
func processRole(id Identity) (string, error) {
	buf := make([]byte, 1<<20)
	size := C.size_t(len(buf))
	if C.upgrade_args(C.int(id.PID), (*C.char)(unsafe.Pointer(&buf[0])), &size) != 0 {
		return "", fmt.Errorf("cannot read arguments pid=%d", id.PID)
	}
	now, err := Inspect(id.PID)
	if err != nil || !now.SameProcess(id) {
		return "", fmt.Errorf("identity changed pid=%d", id.PID)
	}
	buf = buf[:int(size)]
	if len(buf) < 4 {
		return "", fmt.Errorf("short process arguments")
	}
	argc := int(binary.NativeEndian.Uint32(buf[:4]))
	buf = buf[4:]
	n := bytes.IndexByte(buf, 0)
	if n < 0 {
		return "", fmt.Errorf("invalid process arguments")
	}
	buf = bytes.TrimLeft(buf[n+1:], "\x00")
	args := bytes.Split(buf, []byte{0})
	if argc < 2 || len(args) < argc {
		return "", nil
	}
	return string(args[1]), nil
}

func signalIdentity(id Identity, sig syscall.Signal) error {
	now, err := Inspect(id.PID)
	if err != nil {
		return err
	}
	if !now.SameProcess(id) {
		return fmt.Errorf("identity changed pid=%d", id.PID)
	}
	return syscall.Kill(id.PID, sig)
}

func freezeUpgradeHost(ctx context.Context, id Identity) error {
	if err := signalIdentity(id, syscall.SIGSTOP); err != nil {
		return err
	}
	for {
		now, err := Inspect(id.PID)
		if err != nil || !now.SameProcess(id) {
			return fmt.Errorf("host identity lost while freezing: %v", err)
		}
		if C.upgrade_status(C.int(id.PID)) == C.SSTOP {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// IsSharedModelHost excludes a verified same-user model host from the bounded
// conversation-writer detector. It never authorizes sending a signal.
func IsSharedModelHost(pid int) bool {
	id, err := Inspect(pid)
	if err != nil || id.UID != uint32(os.Getuid()) {
		return false
	}
	role, err := processRole(id)
	return err == nil && role == "model-host"
}

func upgradeChildren(pid int) ([]int, error) {
	size := C.proc_listchildpids(C.int(pid), nil, 0)
	if size < 0 {
		return nil, fmt.Errorf("cannot enumerate host children")
	}
	// proc_listchildpids returns a PID count (buffer length is in bytes).
	buf := make([]C.int, int(size)+1024)
	n := C.proc_listchildpids(C.int(pid), unsafe.Pointer(&buf[0]), C.int(len(buf)*4))
	if n < 0 || int(n) >= len(buf) {
		return nil, fmt.Errorf("host child enumeration overflow")
	}
	out := make([]int, 0, int(n))
	for _, p := range buf[:int(n)] {
		if p > 0 {
			out = append(out, int(p))
		}
	}
	return out, nil
}
