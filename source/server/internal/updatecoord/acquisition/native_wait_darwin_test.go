//go:build darwin

package acquisition

import (
	"context"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/nativeprocess"
	"golang.org/x/sys/unix"
)

// Register the kernel exit watch while the exact observed updater is alive,
// then recheck birth identity. This is separate from the launch-owned reaper.
func nativePinUpdater(t *testing.T, id nativeprocess.Identity) func(context.Context) {
	t.Helper()
	fd, err := unix.Kqueue()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	change := unix.Kevent_t{Ident: uint64(id.PID), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ENABLE | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}
	if _, err = unix.Kevent(fd, []unix.Kevent_t{change}, nil, nil); err != nil {
		t.Fatal(err)
	}
	now, err := nativeprocess.Inspect(id.PID)
	if err != nil || !now.SameProcess(id) {
		t.Fatalf("updater changed while pinning native exit watch: %v", err)
	}
	return func(ctx context.Context) {
		t.Helper()
		events := make([]unix.Kevent_t, 1)
		for ctx.Err() == nil {
			timeout := unix.NsecToTimespec(int64(100 * time.Millisecond))
			n, err := unix.Kevent(fd, nil, events, &timeout)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if n == 0 {
				continue
			}
			event := events[0]
			if event.Ident != uint64(id.PID) || event.Filter != unix.EVFILT_PROC || event.Flags&unix.EV_ERROR != 0 || event.Fflags&unix.NOTE_EXIT == 0 {
				t.Fatalf("unexpected native exit event: %+v", event)
			}
			return
		}
		t.Fatalf("pinned native updater exit unproven: %v", ctx.Err())
	}
}
