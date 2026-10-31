//go:build darwin && cgo

package brewrestart

/*
#include <libproc.h>
#include <sys/proc_info.h>
#include <sys/stat.h>
static int upgrade_pipe(int pid, int fd, struct pipe_fdinfo *p) {
 return proc_pidfdinfo(pid, fd, PROC_PIDFDPIPEINFO, p, sizeof(*p)) == sizeof(*p)
   && (p->pipeinfo.pipe_stat.vst_mode & S_IFMT) == S_IFIFO;
}
*/
import "C"

import (
	"fmt"
	"reflect"
	"unsafe"
)

type upgradePipe struct {
	FD           int
	Handle, Peer uint64
	Mode         uint32
}

// Kernel endpoint handles are transient evidence, never logged or persisted.
func upgradePipes(id Identity) ([]upgradePipe, error) {
	n := C.proc_pidinfo(C.int(id.PID), C.PROC_PIDLISTFDS, 0, nil, 0)
	if n <= 0 || n > 1<<20 {
		return nil, fmt.Errorf("pipe fd inventory unavailable")
	}
	size := int(unsafe.Sizeof(C.struct_proc_fdinfo{}))
	buf := make([]C.struct_proc_fdinfo, int(n)/size+32)
	n = C.proc_pidinfo(C.int(id.PID), C.PROC_PIDLISTFDS, 0, unsafe.Pointer(&buf[0]), C.int(len(buf)*size))
	if n <= 0 || int(n) >= len(buf)*size || int(n)%size != 0 {
		return nil, fmt.Errorf("pipe fd inventory incomplete")
	}
	var out []upgradePipe
	for _, fd := range buf[:int(n)/size] {
		if fd.proc_fdtype != C.PROX_FDTYPE_PIPE {
			continue
		}
		var p C.struct_pipe_fdinfo
		if C.upgrade_pipe(C.int(id.PID), C.int(fd.proc_fd), &p) == 0 {
			return nil, fmt.Errorf("pipe endpoint unavailable")
		}
		out = append(out, upgradePipe{int(fd.proc_fd), uint64(p.pipeinfo.pipe_handle), uint64(p.pipeinfo.pipe_peerhandle), uint32(p.pfi.fi_openflags) & 3})
	}
	return out, nil
}

func privateStdioPairs(host, child []upgradePipe) bool {
	var in, out upgradePipe
	ni, no := 0, 0
	for _, p := range child {
		if p.FD == 0 {
			in = p
			ni++
		}
		if p.FD == 1 {
			out = p
			no++
		}
	}
	if ni != 1 || no != 1 || in.Mode != 1 || out.Mode != 2 || in.Handle == 0 || in.Peer == 0 || out.Handle == 0 || out.Peer == 0 {
		return false
	}
	// Two disjoint anonymous pipes, not a duplex/aliased single connection.
	if in.Handle == in.Peer || out.Handle == out.Peer || in.Handle == out.Handle || in.Handle == out.Peer || in.Peer == out.Handle || in.Peer == out.Peer {
		return false
	}
	matches := func(p upgradePipe, mode uint32) bool {
		found := false
		for _, h := range host {
			if h.Handle == p.Peer || h.Peer == p.Handle {
				if h.Handle != p.Peer || h.Peer != p.Handle || h.Mode != mode {
					return false
				}
				found = true // dup FDs of the same endpoint are harmless.
			}
		}
		return found
	}
	return matches(in, 2) && matches(out, 1)
}

// This proves the existing host-private stdio ownership shape, not MCP protocol
// membership. Parentage or executable names alone never authorize this root.
type stdioWitness struct{ host, child []upgradePipe }

func verifiedPrivateStdio(host, child Identity, witness ...*stdioWitness) (bool, error) {
	if child.UID != host.UID || child.ParentPID != host.PID || child.StartSeconds < host.StartSeconds || (child.StartSeconds == host.StartSeconds && child.StartMicroseconds < host.StartMicroseconds) {
		return false, nil
	}
	shared, err := verifiedModelHost(child)
	if err != nil || shared {
		return false, err
	}
	check := func() error {
		for _, id := range []Identity{host, child} {
			now, err := Inspect(id.PID)
			if err != nil || !now.SameProcess(id) || now.ParentPID != id.ParentPID {
				return fmt.Errorf("stdio identity changed")
			}
		}
		return nil
	}
	if err := check(); err != nil {
		return false, err
	}
	h, err := upgradePipes(host)
	if err != nil {
		return false, err
	}
	c, err := upgradePipes(child)
	if err != nil {
		return false, err
	}
	h2, err := upgradePipes(host)
	if err != nil {
		return false, err
	}
	c2, err := upgradePipes(child)
	if err != nil {
		return false, err
	}
	if err := check(); err != nil {
		return false, err
	}
	if !reflect.DeepEqual(h, h2) || !reflect.DeepEqual(c, c2) {
		return false, fmt.Errorf("stdio endpoints changed")
	}
	if !privateStdioPairs(h, c) {
		return false, nil
	}
	for _, w := range witness {
		if w.host != nil && (!reflect.DeepEqual(w.host, h) || !reflect.DeepEqual(w.child, c)) {
			return false, fmt.Errorf("stdio ownership witness changed")
		}
		w.host, w.child = h, c
	}
	return true, nil
}
