//go:build darwin && cgo

package brewrestart

/*
#include <errno.h>
#include <libproc.h>
#include <stdlib.h>
#include <string.h>
#include <arpa/inet.h>

// Examine one known process, not the system-wide process list. Return an
// error on an unstable/incomplete descriptor snapshot rather than guessing.
static int cercano_listener(int pid, const char *address, int port, int *found) {
 errno=0;
 int n=proc_pidinfo(pid,PROC_PIDLISTFDS,0,NULL,0);
 // An empty FD inventory is ambiguous, not evidence of process death.
 if(n<=0) return errno ? errno : EAGAIN;
 if(n>16*1024*1024) return EOVERFLOW;
 int capacity=n+32*sizeof(struct proc_fdinfo);
 struct proc_fdinfo *fds=calloc(1,capacity);
 if(!fds) return ENOMEM;
 errno=0;
 n=proc_pidinfo(pid,PROC_PIDLISTFDS,0,fds,capacity);
 if(n<=0 || n>=capacity || n%sizeof(*fds)) {int e=errno ? errno : EAGAIN;free(fds);return e;}
 *found=0;
 for(int i=0;i<n/sizeof(*fds);i++) {
  if(fds[i].proc_fdtype!=PROX_FDTYPE_SOCKET) continue;
  struct socket_fdinfo s; memset(&s,0,sizeof(s)); errno=0;
  if(proc_pidfdinfo(pid,fds[i].proc_fd,PROC_PIDFDSOCKETINFO,&s,sizeof(s))!=sizeof(s)) {
   int e=errno ? errno : EAGAIN;free(fds);return e;
  }
  if(s.psi.soi_kind!=SOCKINFO_TCP || s.psi.soi_proto.pri_tcp.tcpsi_state!=TSI_S_LISTEN) continue;
  struct in_sockinfo *in=&s.psi.soi_proto.pri_tcp.tcpsi_ini;
  int actual_port=ntohs((uint16_t)in->insi_lport);
  if(port && actual_port!=port) continue;
  char text[INET6_ADDRSTRLEN]; const char *converted=NULL;
  if(s.psi.soi_family==AF_INET) converted=inet_ntop(AF_INET,&in->insi_laddr.ina_46.i46a_addr4,text,sizeof(text));
  if(s.psi.soi_family==AF_INET6) converted=inet_ntop(AF_INET6,&in->insi_laddr.ina_6,text,sizeof(text));
  if(converted && !strcmp(address,text)) *found=port ? 1 : actual_port;
  // The agent binds :port. A wildcard socket owns the requested loopback
  // endpoint only when the kernel advertises that endpoint's IP family.
  struct in_addr target4;
  struct in6_addr target6;
  int wants4=inet_pton(AF_INET,address,&target4)==1;
  int wants6=inet_pton(AF_INET6,address,&target6)==1;
  if(s.psi.soi_family==AF_INET && in->insi_laddr.ina_46.i46a_addr4.s_addr==INADDR_ANY && wants4 && (in->insi_vflag & INI_IPV4)) *found=port ? 1 : actual_port;
  if(s.psi.soi_family==AF_INET6 && IN6_IS_ADDR_UNSPECIFIED(&in->insi_laddr.ina_6) &&
     ((wants4 && (in->insi_vflag & INI_IPV4)) || (wants6 && (in->insi_vflag & INI_IPV6)))) *found=port ? 1 : actual_port;
 }
 free(fds);return 0;
}
*/
import "C"

import (
	"fmt"
	"net/netip"
	"os"
	"syscall"
	"unsafe"
)

// HoldsListener verifies a known same-user process owns a loopback TCP endpoint.
// Wildcard bindings are accepted only for IP families supported by the socket.
// This is a per-candidate check, not a proof of unique socket ownership.
func HoldsListener(expected Identity, endpoint netip.AddrPort) (bool, error) {
	return holdsListenerInspected(expected, endpoint, Inspect)
}

func holdsListenerInspected(expected Identity, endpoint netip.AddrPort, inspect func(int) (Identity, error)) (bool, error) {
	if !endpoint.IsValid() || !endpoint.Addr().IsLoopback() || endpoint.Port() == 0 || endpoint.Addr().Zone() != "" {
		return false, fmt.Errorf("restart requires an explicit loopback TCP endpoint")
	}
	before, err := inspect(expected.PID)
	if err != nil {
		return false, err
	}
	if !before.SameProcess(expected) || before.UID != uint32(os.Getuid()) {
		return false, fmt.Errorf("listener process identity changed or belongs to another user")
	}
	address := C.CString(endpoint.Addr().String())
	defer C.free(unsafe.Pointer(address))
	var found C.int
	if code := C.cercano_listener(C.int(expected.PID), address, C.int(endpoint.Port()), &found); code != 0 {
		return false, fmt.Errorf("inspect process listener: %w", syscall.Errno(code))
	}
	after, err := inspect(expected.PID)
	if err != nil {
		return false, err
	}
	if !before.SameProcess(after) {
		return false, fmt.Errorf("process changed during socket inspection")
	}
	return found != 0, nil
}

// upgradeListenerEndpoint discovers socket authority for an additional host.
// It accepts only IPv4 loopback or wildcard listeners, then reuses the normal
// identity-bracketed ownership check. No argv port or executable name suffices.
func upgradeListenerEndpoint(id Identity) (netip.AddrPort, error) {
	address := C.CString("127.0.0.1")
	defer C.free(unsafe.Pointer(address))
	var port C.int
	if code := C.cercano_listener(C.int(id.PID), address, 0, &port); code != 0 {
		return netip.AddrPort{}, fmt.Errorf("inspect additional host listener: %w", syscall.Errno(code))
	}
	if port == 0 {
		return netip.AddrPort{}, fmt.Errorf("host pid=%d has no verified loopback listener", id.PID)
	}
	endpoint := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
	owns, err := HoldsListener(id, endpoint)
	if err != nil || !owns {
		return netip.AddrPort{}, fmt.Errorf("additional host listener identity lost pid=%d: %v", id.PID, err)
	}
	return endpoint, nil
}
