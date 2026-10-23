package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/crashlog"
	"cercano/source/server/internal/filelock"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/config"
)

const (
	modelHostLockFile = "modelhost.lock"
	modelHostIdleEnv  = "CERCANO_MODEL_HOST_IDLE"
)

// modelHostHandoverWait bounds how long a new model host waits for an old,
// draining one to release the lock. A variable so tests can shorten it.
var modelHostHandoverWait = 15 * time.Second

// modelHostStopTimeout bounds the graceful gRPC stop before a hard stop.
const modelHostStopTimeout = 5 * time.Second

// launchModelHost returns the function agents use to start a model host
// process. A variable so tests can replace it.
var launchModelHost = func(runDir string) func(context.Context) error {
	exe, _ := os.Executable()
	return modelhost.LaunchBinary(exe, runDir)
}

type modelHostOptions struct {
	// RunDir holds the record, lock and log. Default modelhost.RunDir().
	RunDir string
	Config config.Config
	// IdleTimeout overrides the server default when positive. The
	// CERCANO_MODEL_HOST_IDLE environment variable overrides both.
	IdleTimeout time.Duration
	// Listen is the address to bind. Default 127.0.0.1:0.
	Listen string
	// Manager is the runtime manager to serve. When nil it is built with
	// newLocalRuntimeManager.
	Manager localruntime.Manager
	// Events is the durable event log passed to newLocalRuntimeManager.
	Events *crashlog.Writer
}

// runModelHost runs the shared model host until it goes idle, is told to shut
// down, or ctx is cancelled. It returns nil when another healthy host already
// owns the run directory.
func runModelHost(ctx context.Context, opts modelHostOptions) error {
	dir := opts.RunDir
	if dir == "" {
		d, err := modelhost.RunDir()
		if err != nil {
			return err
		}
		dir = d
	}
	// ReadRecord creates and validates the run directory (the lock file lives
	// in it). A missing or unreadable record is not fatal here: an unsafe
	// directory fails WriteRecord below, and a stale record is replaced.
	if _, err := modelhost.ReadRecord(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("[WARN] model host: reading existing record: %v", err)
	}

	release, err := acquireModelHostLock(ctx, dir)
	if err != nil {
		return err
	}
	if release == nil {
		fmt.Println("model host already running")
		return nil
	}
	defer release()

	idle := opts.IdleTimeout
	if v := os.Getenv(modelHostIdleEnv); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			idle = d
		} else {
			log.Printf("[WARN] ignoring invalid %s=%q", modelHostIdleEnv, v)
		}
	}

	mgr := opts.Manager
	if mgr == nil {
		mgr = newLocalRuntimeManagerWithLoader(opts.Config, opts.Events, machineRuntimeConfig)
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generating token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	listen := opts.Listen
	if listen == "" {
		listen = "127.0.0.1:0"
	}
	lis, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", listen, err)
	}

	exit := make(chan struct{})
	var exitOnce sync.Once
	signalExit := func() { exitOnce.Do(func() { close(exit) }) }

	srv := modelhost.NewServer(mgr, modelhost.ServerOptions{
		Token:       token,
		IdleTimeout: idle,
		Version:     version,
		OnIdle:      signalExit,
		OnShutdown:  signalExit,
	})
	g := grpc.NewServer()
	srv.Register(g)

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	go srv.Run(runCtx)

	serveErr := make(chan error, 1)
	go func() { serveErr <- g.Serve(lis) }()

	pid := os.Getpid()
	rec := modelhost.Record{
		PID:        pid,
		Addr:       lis.Addr().String(),
		Token:      token,
		APIVersion: modelhost.APIVersion,
		Version:    version,
		StartedAt:  time.Now(),
	}
	if err := modelhost.WriteRecord(dir, rec); err != nil {
		g.Stop()
		return fmt.Errorf("writing model host record: %w", err)
	}
	log.Printf("model host %d serving on %s", pid, rec.Addr)

	var result error
	select {
	case <-exit:
		// StopAll already ran before OnIdle/OnShutdown.
	case <-ctx.Done():
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := srv.StopAll(stopCtx); err != nil {
			log.Printf("model host: stopping instances: %v", err)
		}
		cancel()
	case err := <-serveErr:
		result = fmt.Errorf("model host serve: %w", err)
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_ = srv.StopAll(stopCtx)
		cancel()
	}

	// Remove the record first so no new client discovers a host that is
	// going away, then stop the server.
	if err := modelhost.RemoveRecord(dir, pid); err != nil {
		log.Printf("model host: removing record: %v", err)
	}
	stopped := make(chan struct{})
	go func() { g.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(modelHostStopTimeout):
		g.Stop()
		<-stopped
	}
	return result
}

// modelHostPollInterval is how often a waiting launcher retries the lock and
// probes the recorded host.
const modelHostPollInterval = 200 * time.Millisecond

// acquireModelHostLock takes the host lock. It returns a nil release when a
// healthy host already runs, including one that is still starting and only
// begins answering while we wait. If the holder never answers and never
// releases within modelHostHandoverWait, it returns an error.
func acquireModelHostLock(ctx context.Context, dir string) (func(), error) {
	path := filepath.Join(dir, modelHostLockFile)
	deadline := time.Now().Add(modelHostHandoverWait)
	for {
		release, ok, err := filelock.TryLock(path)
		if err != nil {
			return nil, fmt.Errorf("model host lock: %w", err)
		}
		if ok {
			return release, nil
		}
		if hostAnswers(ctx, dir) {
			return nil, nil
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("model host: previous host still holds the lock after %s", modelHostHandoverWait)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(modelHostPollInterval):
		}
	}
}

// hostAnswers reports whether the recorded host verifies and answers Hello
// normally; Dial performs the verified Hello, which a draining host answers
// with Unavailable.
func hostAnswers(ctx context.Context, dir string) bool {
	rec, err := modelhost.ReadRecord(dir)
	if err != nil {
		return false
	}
	dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := modelhost.Dial(dctx, rec)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
