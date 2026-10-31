//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <sys/sysctl.h>
#include <sys/proc.h>
static int upgrade_args(int pid, char *buf, size_t *size) {
 int mib[3] = { CTL_KERN, KERN_PROCARGS2, pid };
 return sysctl(mib, 3, buf, size, NULL, 0);
}
*/
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// processRole is only a classifier AFTER kernel executable/start identity
// checks. Classification alone never establishes signal authority.
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
	if argc < 1 || len(args) < argc {
		return "", fmt.Errorf("invalid process argc")
	}
	argv := make([]string, argc)
	for i := range argv {
		argv[i] = string(args[i])
	}
	return upgradeArgvRole(argv), nil
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

// IsUpgradeUnidentifiedOrNonWriter excludes unavailable images and recognized
// non-writers from the known-agent gate. It grants no signal authority and does
// not claim that every excluded process has been classified as a non-writer.
func IsUpgradeUnidentifiedOrNonWriter(pid int) bool {
	id, err := Inspect(pid)
	if err != nil {
		return true
	}
	if id.UID != uint32(os.Getuid()) {
		return false
	}
	name := filepath.Base(id.Executable)
	if name != "cercano" && name != "cercano-agent" {
		return false
	}
	role, err := processRole(id)
	return err == nil && (role == "model-host" || role == "worker" || role == "mcp-client")
}
