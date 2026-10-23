//go:build windows

package modelhost

import (
	"syscall"
	"testing"
)

// TestDetachSysProcAttrDetachesFromConsole: a host launched from an agent
// started in a console must not share that console or its Ctrl+C group.
func TestDetachSysProcAttrDetachesFromConsole(t *testing.T) {
	attr := detachSysProcAttr()
	want := uint32(syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess)
	if attr == nil || attr.CreationFlags&want != want {
		t.Fatalf("SysProcAttr = %+v, want CreationFlags with CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS", attr)
	}
}
