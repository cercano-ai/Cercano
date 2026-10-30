package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func devHostLog(t *testing.T, root, name string) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(root, name), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// Only the exec.Cmd returned by our own Start is signalled, never a discovered PID.
func devHostReap(t *testing.T, cmd *exec.Cmd, name string) {
	t.Helper()
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Logf("owned host %s pid=%d reaped: %v", name, cmd.Process.Pid, err)
		if err != nil {
			t.Errorf("host %s failed; see private stderr", name)
		}
	case <-time.After(5 * time.Second):
		t.Errorf("host %s did not join in 5s; collecting owned-child stack", name)
		devHostDump(cmd)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Errorf("owned host %s did not reap", name)
			}
		}
	}
}

func devHostBuild(t *testing.T, original *exec.Cmd, root string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Resolve shared compiler caches before giving the compiler a private HOME.
	query := exec.CommandContext(ctx, "go", "env", "GOCACHE", "GOMODCACHE")
	caches, err := query.Output()
	if err != nil {
		return nil, err
	}
	paths := strings.Split(strings.TrimSpace(string(caches)), "\n")
	if len(paths) != 2 {
		return nil, fmt.Errorf("expected two compiler cache paths")
	}
	cmd := exec.CommandContext(ctx, original.Path, original.Args[1:]...)
	cmd.WaitDelay = 2 * time.Second
	devHostBoundBuild(cmd)
	buildHome := filepath.Join(root, "build-home")
	if err := os.MkdirAll(buildHome, 0700); err != nil {
		return nil, err
	}
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + buildHome, "TMPDIR=" + filepath.Join(root, "tmp"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "CERCANO_DISABLE_CREDENTIALS=1", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOCACHE=" + paths[0], "GOMODCACHE=" + paths[1]}
	start := time.Now()
	output, err := cmd.CombinedOutput()
	t.Logf("compiled %s in %s: %v", filepath.Base(filepath.Dir(original.Args[len(original.Args)-2])), time.Since(start), err)
	return output, err
}

// Exercise lease revocation on an otherwise idle tagged worker, independently
// of graceful host shutdown, so successful teardown cannot hide a broken lease.
func devHostCheckWorkerLease(t *testing.T, binary, root string, env []string) {
	t.Helper()
	dir := filepath.Join(root, "lease-check")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "worker", "--socket", filepath.Join(dir, "w.sock"), "--prepared", "--failure-log", filepath.Join(dir, "failure.jsonl"), "--routing-log", filepath.Join(dir, "routing.jsonl"))
	cmd.Env = append([]string(nil), env...)
	for i, value := range cmd.Env {
		if strings.HasPrefix(value, "CERCANO_DEV_FIXTURE_STOP=") {
			cmd.Env[i] = "CERCANO_DEV_FIXTURE_STOP=" + filepath.Join(dir, "stop")
		}
	}
	cmd.Stderr = devHostLog(t, root, "lease.stderr")
	cmd.WaitDelay = time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cmd.Wait(); err != nil {
			t.Errorf("owned lease worker failed to exit: %v", err)
		} else {
			t.Logf("owned lease worker pid=%d reaped after revocation", cmd.Process.Pid)
		}
	}()
	for {
		if _, err := os.Stat(filepath.Join(dir, "w.sock")); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("lease worker startup deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "stop"), nil, 0600); err != nil {
		t.Fatal(err)
	}
}
