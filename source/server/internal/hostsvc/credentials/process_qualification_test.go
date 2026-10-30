package credentials

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/filelock"
)

// processRecordBackend is TEST ONLY. Same-directory rename publishes a complete
// file atomically to fresh readers on the tested filesystem. All successful
// writes close before rename; pre-rename errors preserve the old file. There is
// no fsync, crash durability, uncertainty quarantine, native-store adapter, or
// claim to qualify native durability.
type processRecordBackend struct{ dir string }

const processIdentity = "process-test-profile"

func (b processRecordBackend) Read(name string) (string, error) {
	if name != processIdentity {
		return "", ErrCoordination
	}
	raw, err := os.ReadFile(filepath.Join(b.dir, "record"))
	return string(raw), err
}
func (b processRecordBackend) Replace(name, raw string) error {
	if name != processIdentity {
		return ErrCoordination
	}
	f, err := os.CreateTemp(b.dir, "replacement-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = io.WriteString(f, raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(b.dir, "record"))
}
func (b processRecordBackend) Keys() ([]string, error) { return []string{processIdentity}, nil }

// This entry point is inert in ordinary test runs. Children use only the direct
// coordinator and the fake above; policy remains disabled throughout.
func TestCredentialProcessHelper(t *testing.T) {
	mode := os.Getenv("CERCANO_TEST_PROCESS_MODE")
	if mode == "" {
		return
	}
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" || os.Getenv("GOPROXY") != "off" || os.Getenv("GOSUMDB") != "off" {
		t.Fatal("helper must be disabled/offline")
	}
	dir := os.Getenv("CERCANO_TEST_PROCESS_DIR")
	s, err := NewCoordinatedStore(processRecordBackend{dir}, filepath.Join(dir, "locks"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	signal := func(v string) { fmt.Println("PROCESS:" + v) }
	barrier := func() {
		var v string
		if _, err := fmt.Fscanln(os.Stdin, &v); err != nil || v != "release" {
			t.Fatalf("barrier: %q %v", v, err)
		}
	}
	stale := func(err error) {
		if !errors.Is(err, ErrStale) {
			t.Fatalf("want stale, got %v", err)
		}
	}
	switch mode {
	case "login", "refresh", "admit":
		r, err := s.snapshot(ctx, processIdentity)
		if err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "login":
			signal("ready")
			barrier()
			stale(s.replace(ctx, processIdentity, r.Revision, "active", "delayed-login-secret"))
		case "refresh":
			err = s.refresh(ctx, processIdentity, r.Revision, func(_ context.Context, payload string) (string, error) {
				if payload != "original-secret" {
					t.Fatalf("unexpected issuer input")
				}
				signal("ready")
				barrier()
				return "delayed-refresh-secret", nil
			})
			if err != nil {
				t.Fatal(err)
			}
		case "admit":
			err = s.Admit(ctx, processIdentity, r.Revision, func() error { signal("ready"); barrier(); return nil })
			if err != nil {
				t.Fatal(err)
			}
			signal("done")
			return
		}
		called := false
		stale(s.Admit(ctx, processIdentity, r.Revision, func() error { called = true; return nil }))
		if called {
			t.Fatal("stale request dispatched")
		}
	case "busy-refresh", "busy-logout", "busy-replace", "busy-admit":
		var err error
		switch mode {
		case "busy-refresh":
			err = s.refresh(ctx, processIdentity, strings.Repeat("ab", 32), func(context.Context, string) (string, error) { t.Fatal("busy issuer ran"); return "", nil })
		case "busy-logout":
			err = s.Delete(processIdentity)
		case "busy-replace":
			err = s.Set(processIdentity, "other-secret")
		case "busy-admit":
			err = s.AdmitAttempt(ctx, processIdentity, strings.Repeat("ab", 32))
		}
		if !errors.Is(err, ErrBusy) {
			t.Fatalf("want busy, got %v", err)
		}
	case "logout":
		if err := s.Delete(processIdentity); err != nil {
			t.Fatal(err)
		}
	case "probe-commit":
		sum := sha256.Sum256([]byte("cercano\x00" + processIdentity))
		release, ok, err := filelock.TryLock(filepath.Join(dir, "locks", hex.EncodeToString(sum[:])+".commit.lock"))
		if ok {
			release()
		}
		if err != nil || ok {
			t.Fatalf("exclusion unavailable: %v %v", ok, err)
		}
	default:
		t.Fatalf("unknown mode %q", mode)
	}
	signal("done")
}

type credentialChild struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	lines  <-chan string
	done   <-chan error
	cancel context.CancelFunc
}

func startCredentialChild(t *testing.T, dir, mode string) *credentialChild {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestCredentialProcessHelper$", "-test.count=1", "-test.timeout=18s")
	// Remove inherited helper/offline settings before installing exact values.
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if key != "CERCANO_DISABLE_CREDENTIALS" && key != "GOPROXY" && key != "GOSUMDB" && !strings.HasPrefix(key, "CERCANO_TEST_PROCESS_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "CERCANO_DISABLE_CREDENTIALS=1", "GOPROXY=off", "GOSUMDB=off", "CERCANO_TEST_PROCESS_MODE="+mode, "CERCANO_TEST_PROCESS_DIR="+dir)
	input, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		input.Close()
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cancel()
		input.Close()
		t.Fatal(err)
	}
	lines := make(chan string, 128)
	done := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanErr := scanner.Err()
		err := cmd.Wait()
		if err == nil {
			err = scanErr
		}
		done <- err
		close(lines)
	}()
	c := &credentialChild{cmd, input, lines, done, cancel}
	t.Cleanup(func() {
		input.Close()
		cancel()
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			t.Errorf("child %d not reaped", cmd.Process.Pid)
		}
	})
	return c
}
func (c *credentialChild) expect(t *testing.T, want string) {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	var transcript []string
	for {
		select {
		case line, ok := <-c.lines:
			if !ok {
				t.Fatalf("child ended before %s: %s", want, strings.Join(transcript, "\n"))
			}
			transcript = append(transcript, line)
			if line == "PROCESS:"+want {
				return
			}
		case <-timer.C:
			t.Fatalf("child barrier %s timed out: %v", want, transcript)
		}
	}
}
func (c *credentialChild) release(t *testing.T) {
	t.Helper()
	if _, err := io.WriteString(c.input, "release\n"); err != nil {
		t.Fatal(err)
	}
}
func (c *credentialChild) finish(t *testing.T) {
	t.Helper()
	c.expect(t, "done")
	// Keep the exit status available for cleanup's ownership/reaping check.
	select {
	case err := <-c.done:
		if err != nil {
			t.Errorf("child failed: %v", err)
		}
		c.done = closedCredentialChild(err)
	case <-time.After(20 * time.Second):
		t.Fatal("child exit timed out")
	}
}
func closedCredentialChild(err error) <-chan error { ch := make(chan error, 1); ch <- err; return ch }

func seedCredentialProcess(t *testing.T) (string, processRecordBackend) {
	t.Helper()
	dir := t.TempDir()
	b := processRecordBackend{dir}
	raw, err := json.Marshal(credentialRecord{1, strings.Repeat("ab", 32), "active", "original-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Replace(processIdentity, string(raw)); err != nil {
		t.Fatal(err)
	}
	return dir, b
}
func TestCredentialProcessLogoutFencesDelayedPublication(t *testing.T) {
	dir, b := seedCredentialProcess(t)
	delayed := startCredentialChild(t, dir, "login")
	delayed.expect(t, "ready")
	startCredentialChild(t, dir, "logout").finish(t)
	marker, err := b.Read(processIdentity)
	if err != nil {
		t.Fatal(err)
	}
	delayed.release(t)
	delayed.finish(t)
	after, err := b.Read(processIdentity)
	if err != nil || after != marker {
		t.Fatal("stale login changed marker", err)
	}
}

func TestCredentialProcessRefreshExclusion(t *testing.T) {
	dir, b := seedCredentialProcess(t)
	holder := startCredentialChild(t, dir, "refresh")
	holder.expect(t, "ready")
	for _, mode := range []string{"busy-refresh", "busy-logout", "busy-replace", "busy-admit", "probe-commit"} {
		startCredentialChild(t, dir, mode).finish(t)
	}
	raw, err := b.Read(processIdentity)
	if err != nil {
		t.Fatal(err)
	}
	r, err := decodeRecord(raw)
	if err != nil || r.State != "active" || r.Payload != "original-secret" {
		t.Fatal("refresh changed record before publication", err)
	}
	holder.release(t)
	holder.finish(t)
	startCredentialChild(t, dir, "logout").finish(t)
}
func TestCredentialProcessCommitExclusion(t *testing.T) {
	dir, _ := seedCredentialProcess(t)
	holder := startCredentialChild(t, dir, "admit")
	holder.expect(t, "ready")
	startCredentialChild(t, dir, "probe-commit").finish(t)
	holder.release(t)
	holder.finish(t)
	startCredentialChild(t, dir, "logout").finish(t)
}
