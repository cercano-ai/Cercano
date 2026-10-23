package modelhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/pkg/proto"
)

// defaultRelaunchTimeout bounds each phase of Relaunch.
const defaultRelaunchTimeout = 10 * time.Second

// RelaunchOptions configures Relaunch.
type RelaunchOptions struct {
	// RunDir holds the host record and launch lock. Default RunDir().
	RunDir string
	// ClientVersion is sent in Hello.
	ClientVersion string
	// Launch starts the replacement host; it must not wait for it to be
	// ready. Typically LaunchBinary with the installed executable.
	Launch func(ctx context.Context) error
	// Timeout bounds each phase: stopping the running host, and the
	// replacement answering. Default 10s.
	Timeout time.Duration
}

// Relaunch replaces the running model host with one started by opts.Launch.
// It holds the launch lock that Ensure takes before launching, from before
// the old host is told to shut down until the new one answers, so an agent
// that loses its connection meanwhile waits for the lock and then attaches to
// the new host instead of launching a host from its own executable.
//
// When no host is running (no record, or a recorded host that does not
// answer) it launches nothing and returns false. If the running host does not
// accept Shutdown, it launches nothing and returns the error. If the host
// accepts but has not exited within the timeout, the replacement is launched
// anyway: a starting host waits for the old one to release its lock.
func Relaunch(ctx context.Context, opts RelaunchOptions) (bool, error) {
	if opts.Launch == nil {
		return false, errors.New("relaunch: no launcher configured")
	}
	dir := opts.RunDir
	if dir == "" {
		var err error
		if dir, err = RunDir(); err != nil {
			return false, err
		}
	}
	if err := ensureRunDir(dir); err != nil {
		return false, err
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultRelaunchTimeout
	}

	stopCtx, cancelStop := context.WithTimeout(ctx, timeout)
	defer cancelStop()
	release, err := filelock.Lock(stopCtx, filepath.Join(dir, "launch.lock"))
	if err != nil {
		return false, fmt.Errorf("waiting for model host launch lock: %w", err)
	}
	defer release()

	old, err := ReadRecord(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	conn, _, err := dialVerified(stopCtx, old, opts.ClientVersion)
	if err != nil {
		return false, nil // nothing answering: nothing to replace
	}
	_, err = proto.NewModelHostClient(conn).Shutdown(stopCtx, &proto.ShutdownRequest{})
	conn.Close()
	if err != nil {
		return false, fmt.Errorf("shutting down model host: %w", err)
	}
	awaitHostExit(stopCtx, dir, old, opts.ClientVersion)

	readyCtx, cancelReady := context.WithTimeout(ctx, timeout)
	defer cancelReady()
	if err := opts.Launch(readyCtx); err != nil {
		return false, fmt.Errorf("launching model host: %w", err)
	}
	for {
		rec, st, conn, err := probe(readyCtx, dir, opts.ClientVersion)
		if conn != nil {
			conn.Close()
		}
		if st == live && (rec.PID != old.PID || rec.Addr != old.Addr) {
			return true, nil
		}
		if st == newer {
			return false, err
		}
		if err := readyCtx.Err(); err != nil {
			return false, fmt.Errorf("replacement model host did not answer within %s: %w", timeout, err)
		}
		sleep(readyCtx, pollInterval)
	}
}

// awaitHostExit waits until the host old names has gone: its record is
// removed or replaced, or it no longer accepts connections. A host that is
// still draining (answering Hello with Unavailable) has not gone yet. It
// returns when ctx is done.
func awaitHostExit(ctx context.Context, dir string, old Record, clientVersion string) {
	for ctx.Err() == nil {
		cur, err := ReadRecord(dir)
		if err != nil || cur.PID != old.PID || cur.Addr != old.Addr {
			return
		}
		conn, _, err := dialVerified(ctx, cur, clientVersion)
		if conn != nil {
			conn.Close()
		}
		if err != nil && !isDraining(err) && ctx.Err() == nil {
			return
		}
		sleep(ctx, pollInterval)
	}
}

// isDraining reports whether err is a host's answer that it is shutting down.
func isDraining(err error) bool {
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.Unavailable && st.Message() == status.Convert(errDraining).Message()
}
