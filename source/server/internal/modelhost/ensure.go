package modelhost

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"cercano/source/server/internal/filelock"
	"cercano/source/server/pkg/proto"
)

const (
	defaultEnsureTimeout = 10 * time.Second
	probeTimeout         = 2 * time.Second
	pollInterval         = 100 * time.Millisecond
)

var replaceWait = 5 * time.Second

// ErrNewerModelHost reports that the running model host speaks a newer API
// than this client. Such a host is left alone: an older client never shuts
// it down or launches a competing one.
var ErrNewerModelHost = errors.New("model host is newer than this client")

// EnsureOptions configures Ensure.
type EnsureOptions struct {
	// CoexistenceProtection prohibits replacement and shared runtime mutation.
	CoexistenceProtection bool
	// RunDir holds the host record and launch lock. Default RunDir().
	RunDir string
	// ClientVersion is sent in Hello.
	ClientVersion string
	// Launch starts a host process; it must not wait for it to be ready.
	Launch func(ctx context.Context) error
	// Timeout bounds how long Ensure waits for a launched host. Default 10s.
	Timeout time.Duration
}

// checkLoopback requires addr to be host:port with a loopback IP literal, so
// the token is never offered to a remote or name-resolved endpoint.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("model host address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("model host address %q is not a loopback IP literal", addr)
	}
	return nil
}

// dialVerified connects to the host named by r and authenticates it before
// the token is ever sent: Hello carries a fresh nonce and no token, and the
// host must answer with HMAC-SHA256(token, nonce). Only afterwards do the
// returned connection's interceptors attach the token to calls.
func dialVerified(ctx context.Context, r Record, clientVersion string) (*grpc.ClientConn, *proto.HelloResponse, error) {
	if err := checkLoopback(r.Addr); err != nil {
		return nil, nil, err
	}
	var verified atomic.Bool
	withToken := func(ctx context.Context) context.Context {
		if verified.Load() {
			return metadata.AppendToOutgoingContext(ctx, tokenHeader, r.Token)
		}
		return ctx
	}
	// The connection may open exactly one transport. gRPC would otherwise
	// redial the same address after the host dies, and whatever process bound
	// the port next would receive token-bearing calls without having proved
	// anything. Once the transport is lost every call fails Unavailable and
	// the caller must verify a host afresh.
	var dialed atomic.Bool
	conn, err := grpc.NewClient(r.Addr,
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			if !dialed.CompareAndSwap(false, true) {
				return nil, errors.New("model host connection lost; refusing to redial without verification")
			}
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(withToken(ctx), method, req, reply, cc, opts...)
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return streamer(withToken(ctx), desc, cc, method, opts...)
		}),
	)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		conn.Close()
		return nil, nil, err
	}
	hctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	resp, err := proto.NewModelHostClient(conn).Hello(hctx, &proto.HelloRequest{ClientVersion: clientVersion, Nonce: nonce})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if !hmac.Equal(resp.GetProof(), helloProof(r.Token, nonce)) {
		conn.Close()
		return nil, nil, errors.New("model host failed to prove it holds the token")
	}
	verified.Store(true)
	return conn, resp, nil
}

// Dial connects to the host named by r over loopback after verifying that it
// holds the record's token; the returned connection attaches the token to
// every unary and streaming call. It never reconnects: once its transport is
// lost, calls fail with Unavailable and the caller must Dial again.
func Dial(ctx context.Context, r Record) (*grpc.ClientConn, error) {
	conn, _, err := dialVerified(ctx, r, "")
	return conn, err
}

func shutdownHost(ctx context.Context, conn *grpc.ClientConn) error {
	ctx, cancel := context.WithTimeout(ctx, replaceWait)
	defer cancel()
	_, err := proto.NewModelHostClient(conn).Shutdown(ctx, &proto.ShutdownRequest{})
	return err
}

type probeState int

const (
	notLive probeState = iota
	live
	// older: the host speaks an older API; this client replaces it.
	older
	// newer: the host speaks a newer API; this client leaves it alone.
	newer
)

// probe reads the record and classifies the host it names. Any read, dial,
// verification or handshake failure (including a draining host) is notLive.
// For an older host the verified connection is returned; the caller closes
// it. For a newer host the returned error describes it.
func probe(ctx context.Context, dir, clientVersion string) (Record, probeState, *grpc.ClientConn, error) {
	rec, err := ReadRecord(dir)
	if err != nil {
		return Record{}, notLive, nil, nil
	}
	conn, resp, err := dialVerified(ctx, rec, clientVersion)
	if err != nil {
		return rec, notLive, nil, nil
	}
	switch v := int(resp.GetApiVersion()); {
	case v == APIVersion:
		conn.Close()
		return rec, live, nil, nil
	case v > APIVersion:
		conn.Close()
		return rec, newer, nil, fmt.Errorf("%w (host API %d, version %q; client API %d)", ErrNewerModelHost, v, resp.GetVersion(), APIVersion)
	}
	return rec, older, conn, nil
}

// replaceOlder asks an older host to shut down and waits for
// its record to disappear or for it to stop answering. If it does neither
// within replaceWait, the caller launches anyway: the launched process waits
// for the old host to exit.
func replaceOlder(ctx context.Context, dir string, rec Record, conn *grpc.ClientConn, clientVersion string) {
	defer conn.Close()
	_ = shutdownHost(ctx, conn)
	deadline := time.Now().Add(replaceWait)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		cur, err := ReadRecord(dir)
		if err != nil || cur.PID != rec.PID || cur.Addr != rec.Addr {
			return
		}
		c, _, err := dialVerified(ctx, cur, clientVersion)
		if err != nil {
			return
		}
		c.Close()
		sleep(ctx, 50*time.Millisecond)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// Ensure returns the record of a live, compatible model host, launching one
// if necessary. At most one Launch call is made per Ensure. A host speaking
// an older API is shut down and replaced; a host speaking a newer API is left
// running and Ensure returns an error wrapping ErrNewerModelHost, so an older
// client lingering after an upgrade never takes the newer host down.
func Ensure(ctx context.Context, opts EnsureOptions) (Record, error) {
	dir := opts.RunDir
	if dir == "" {
		var err error
		if dir, err = RunDir(); err != nil {
			return Record{}, err
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultEnsureTimeout
	}
	if !opts.CoexistenceProtection {
		if err := ensureRunDir(dir); err != nil {
			return Record{}, err
		}

	}
	rec, st, conn, err := probe(ctx, dir, opts.ClientVersion)
	switch st {
	case live:
		return rec, nil
	case newer:
		return Record{}, err
	case older:
		if opts.CoexistenceProtection {
			conn.Close()
			return Record{}, ErrCoexistenceProtected
		}
		if opts.Launch == nil {
			conn.Close()
			return Record{}, errors.New("incompatible model host running and no launcher configured")
		}
		replaceOlder(ctx, dir, rec, conn, opts.ClientVersion)
	}
	if opts.CoexistenceProtection {
		return Record{}, ErrCoexistenceProtected
	}
	if opts.Launch == nil {
		return Record{}, errors.New("model host is not running and no launcher is configured")
	}

	lockCtx, cancelLock := context.WithTimeout(ctx, timeout+replaceWait)
	release, err := filelock.Lock(lockCtx, filepath.Join(dir, "launch.lock"))
	cancelLock()
	if err != nil {
		return Record{}, fmt.Errorf("waiting for model host launch lock: %w", err)
	}
	defer release()

	// Another Ensure may have launched while we waited for the lock.
	rec, st, conn, err = probe(ctx, dir, opts.ClientVersion)
	if conn != nil {
		conn.Close()
	}
	switch st {
	case live:
		return rec, nil
	case newer:
		return Record{}, err
	}
	if err := opts.Launch(ctx); err != nil {
		return Record{}, fmt.Errorf("launching model host: %w", err)
	}

	// The record may be absent, stale, or name a draining host until the
	// replacement publishes its own, so keep polling until the deadline.
	deadline := time.Now().Add(timeout)
	for {
		rec, st, conn, err := probe(ctx, dir, opts.ClientVersion)
		if conn != nil {
			conn.Close()
		}
		switch st {
		case live:
			return rec, nil
		case newer:
			return Record{}, err
		}
		if err := ctx.Err(); err != nil {
			return Record{}, err
		}
		if time.Now().After(deadline) {
			return Record{}, fmt.Errorf("model host did not become ready within %s", timeout)
		}
		sleep(ctx, pollInterval)
	}
}

// LaunchBinary returns a Launch func that spawns "<path> model-host"
// detached, in runDir (RunDir() when runDir is empty), appending its output to
// runDir/modelhost.log.
func LaunchBinary(path, runDir string) func(ctx context.Context) error {
	return func(context.Context) error {
		dir := runDir
		if dir == "" {
			var err error
			if dir, err = RunDir(); err != nil {
				return err
			}
		}
		if err := ensureRunDir(dir); err != nil {
			return err
		}
		logf, err := os.OpenFile(filepath.Join(dir, "modelhost.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND|logOpenFlags, 0o600)
		if err != nil {
			return err
		}
		defer logf.Close()
		cmd := exec.Command(path, "model-host")
		// The host outlives the launching agent; it must not pin (or depend
		// on) that agent's working directory.
		cmd.Dir = dir
		cmd.Stdout = logf
		cmd.Stderr = logf
		cmd.SysProcAttr = detachSysProcAttr()
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
}
