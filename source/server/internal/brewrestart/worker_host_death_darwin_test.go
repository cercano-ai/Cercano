//go:build darwin && cgo

package brewrestart

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
)

// This is deliberately the minimal production-spawn experiment, NOT the
// authenticated native_recovery_bridge/consent/CLI integration journey.
func TestProductionWorkerExitsOnHostDeath(t *testing.T) {
	if os.Getenv("CERCANO_RUN_HOST_DEATH_PROBE") != "1" {
		t.Skip("opt-in native subprocess/build probe")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	must(err)
	must(os.Chmod(root, 0700))
	resources, err := os.MkdirTemp("/tmp", "whd-")
	must(err)
	defer os.RemoveAll(resources)
	resources, err = filepath.EvalSymlinks(resources)
	must(err)
	generation := filepath.Join(root, "generation")
	must(os.MkdirAll(generation, 0700))
	home := filepath.Join(root, "home")
	must(os.Mkdir(home, 0700))
	must(os.WriteFile(filepath.Join(generation, "manifest.json"), []byte(`{"probe":true}`), 0600))
	module, err := filepath.Abs("../..")
	must(err)
	build := func(args ...string) {
		t.Helper()
		if os.Getenv("CERCANO_PROBE_RACE") == "1" {
			args = append(args[:1], append([]string{"-race"}, args[1:]...)...)
		}
		c := exec.CommandContext(ctx, "go", args...)
		c.Dir = module
		out, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("build: %v\n%s", e, out)
		}
	}
	build("build", "-o", filepath.Join(generation, "cercano"), "./cmd/cercano")
	helper := filepath.Join(root, "spawn.test")
	build("test", "-c", "-o", helper, "./internal/worker")
	storePath := filepath.Join(root, "conversations.db")
	must(conversation.PrepareDevelopmentStore(ctx, storePath))
	store, err := conversation.OpenCoexistence(ctx, storePath, conversation.DevelopmentCoexistenceDeclaration())
	must(err)
	defer store.Close()
	owner, err := conversation.AcquireOwnership(ctx, store, "idle", "disposable-source")
	must(err)
	must(store.EnsureConversation(conversation.WithOwnership(ctx, owner), "idle", "", ""))
	before, err := conversation.ReadOwnership(ctx, store, "idle")
	must(err)
	// Close releases the OS lock but leaves the durable owned row. This is the
	// minimal abandoned-owner fixture, not proof the subprocess owned persistence.
	must(owner.Close())
	cmd := exec.Command(helper, "-test.run=^TestHostDeathSpawnProbe$", "-test.v")
	cmd.Env = append(os.Environ(), "HOME="+home, "CERCANO_DISABLE_CREDENTIALS=1", "CERCANO_HOST_DEATH_PROBE="+root, "CERCANO_HOST_DEATH_RESOURCES="+resources)
	stdin, err := cmd.StdinPipe()
	must(err)
	defer stdin.Close()
	log, err := os.Create(filepath.Join(root, "host.log"))
	must(err)
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	must(cmd.Start())
	reaped := false
	defer func() {
		if !reaped {
			// Before identity capture, ask the helper to execute its retained
			// handle cleanup rather than killing it and losing child authority.
			_ = stdin.Close()
			_ = cmd.Wait()
		}
	}()
	var child struct {
		Identity Identity
		Socket   string
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		data, e := os.ReadFile(filepath.Join(root, "child.json"))
		if e == nil && json.Unmarshal(data, &child) == nil {
			break
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(filepath.Join(root, "host.log"))
			t.Fatalf("no child identity: %s", b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	id := child.Identity
	// Cleanup signals only this exact, pre-stop captured incarnation, using the
	// production Darwin process-version-bound syscall (never PPID/group scans).
	defer func() {
		now, e := Inspect(id.PID)
		if e == nil && now.SameProcess(id) {
			if e = signalIdentity(id, syscall.SIGKILL); e != nil {
				t.Errorf("exact child cleanup: %v", e)
				return
			}
			until := time.Now().Add(5 * time.Second)
			for time.Now().Before(until) {
				now, e = Inspect(id.PID)
				if errors.Is(e, syscall.ESRCH) || (e == nil && !now.SameProcess(id)) {
					t.Log("exact child exit observed after cleanup")
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Errorf("child exit not observed: %v %+v", e, now)
		}
	}()
	host, err := Inspect(cmd.Process.Pid)
	must(err)
	if id.ParentPID != host.PID || id.ProcessGroup != id.PID || id.ProcessGroup == host.ProcessGroup || id.Executable != filepath.Join(generation, "cercano") {
		t.Fatalf("unexpected spawn identity host=%+v child=%+v", host, id)
	}
	verify := func(c context.Context) error { return VerifyPreparedWritersAbsent(c, generation, storePath) }
	if err := verify(ctx); err == nil {
		t.Fatal("live worker incorrectly certified absent")
	}
	if unexpected, err := conversation.RecoverDeadOwner(ctx, store, "idle", before, "disposable-destination", verify); err == nil {
		_ = unexpected.Close()
		t.Fatal("recovered ownership while source worker remained alive")
	}
	after, err := conversation.ReadOwnership(ctx, store, "idle")
	must(err)
	if after != before {
		t.Fatalf("refused recovery changed ownership: before=%+v after=%+v", before, after)
	}
	t.Logf("before stop host=%+v worker=%+v", host, id)
	start := time.Now()
	must(signalIdentity(host, syscall.SIGKILL))
	err = cmd.Wait()
	reaped = true
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("host exit: %v", err)
	}
	t.Logf("host exit=%v elapsed=%s", err, time.Since(start))
	deadline = time.Now().Add(8 * time.Second)
	for {
		now, e := Inspect(id.PID)
		if errors.Is(e, syscall.ESRCH) || (e == nil && !now.SameProcess(id)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("worker survived owner death: %+v %v", now, e)
		}
		time.Sleep(20 * time.Millisecond)
	}
	must(verify(ctx))
	recovered, err := conversation.RecoverDeadOwner(ctx, store, "idle", before, "disposable-destination", verify)
	must(err)
	must(recovered.Close())
}
