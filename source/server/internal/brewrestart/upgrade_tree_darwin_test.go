//go:build darwin && cgo

package brewrestart

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUpgradeExecSTOPRollback(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", `echo ready > "$1"; read x; exec /bin/sleep 30`, "fixture", filepath.Join(root, "ready"))
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitFixtureFile(t, filepath.Join(root, "ready"))
	id, err := Inspect(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		now, e := Inspect(id.PID)
		if e == nil && sameLifetime(now, id) {
			_ = signalIdentity(now, syscall.SIGKILL)
		}
		_ = cmd.Wait()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var record stoppedIdentity
	err = freezeUpgradeProcess(ctx, id, &record, func() error {
		fmt.Fprintln(in, "exec")
		for {
			now, e := Inspect(id.PID)
			if e != nil {
				return e
			}
			if !sameLifetime(now, id) {
				return fmt.Errorf("fixture lifetime changed")
			}
			if now.Executable != id.Executable {
				return syscall.Kill(id.PID, syscall.SIGSTOP)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond):
			}
		}
	})
	if err == nil || !record.resume {
		t.Fatalf("exec race not reproduced: %v %+v", err, record)
	}
	if err = signalIdentity(id, syscall.SIGCONT); err == nil {
		t.Fatal("stale strict identity unexpectedly accepted")
	}
	if err = resumeUpgradeProcess(record); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		out, err := exec.Command("/bin/ps", "-o", "state=", "-p", strconv.Itoa(id.PID)).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	if strings.Contains(state(), "T") {
		t.Fatal("same lifetime remained stopped after rollback")
	}
	// Freeze again with the new strict identity; prior stopped state must survive rollback.
	now, err := Inspect(id.PID)
	if err != nil {
		t.Fatal(err)
	}
	var first, second stoppedIdentity
	if err = freezeUpgradeHost(ctx, now, &first); err != nil {
		t.Fatal(err)
	}
	if err = freezeUpgradeHost(ctx, now, &second); err != nil {
		t.Fatal(err)
	}
	if second.resume {
		t.Fatal("prior STOP would be undone")
	}
	if err = resumeUpgradeProcess(second); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state(), "T") {
		t.Fatal("rollback resumed a previously stopped process")
	}
	bad := first
	bad.StartSeconds++
	if err = resumeUpgradeProcess(bad); err == nil {
		t.Fatal("rollback mismatch hidden")
	}
	if err = resumeUpgradeProcess(first); err != nil {
		t.Fatal(err)
	}
	t.Log("exec between inspect and STOP refused; lifetime rollback succeeded; strict identity and prior STOP preserved")
}

func TestUpgradeDifferentVersionModelHostDescendant(t *testing.T) {
	root := t.TempDir()
	exe, _ := os.Executable()
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "v2", "cercano")
	if err = os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(binary, data, 0700); err != nil {
		t.Fatal(err)
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}
	pidfile := filepath.Join(root, "pid")
	cmd := exec.Command("/bin/sh", "-c", `"$1" model-host & echo $! > "$2"; wait`, "fixture", binary, pidfile)
	cmd.Env = []string{"CERCANO_TREE_FIXTURE=1"}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(waitFixtureFile(t, pidfile))))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := Inspect(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	var child Identity
	deadline := time.Now().Add(3 * time.Second)
	for {
		child, err = Inspect(pid)
		if err == nil && child.Executable == binary {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child exec timeout")
		}
		time.Sleep(time.Millisecond)
	}
	t.Cleanup(func() {
		_ = signalIdentity(child, syscall.SIGKILL)
		_ = signalIdentity(parent, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	frozen := map[int]stoppedIdentity{}
	defer func() {
		for _, id := range frozen {
			if e := resumeUpgradeProcess(id); e != nil {
				t.Error(e)
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = freezeUpgradeDescendants(ctx, parent, map[int]Identity{parent.PID: parent}, frozen); err != nil {
		t.Fatal(err)
	}
	if _, ok := frozen[pid]; ok {
		t.Fatal("different-version model host frozen")
	}
	if len(frozen) != 1 {
		t.Fatalf("unexpected frozen set: %v", frozen)
	}
	fake := parent
	ok, err := verifiedModelHost(fake)
	if err != nil || ok {
		t.Fatalf("non-Cercano exclusion: %v %v", ok, err)
	}
	t.Log("different executable version model-host excluded from attached descendant freeze")
}

// This exercises a published fork child held before exec, not a kernel-internal
// fork-publication fence. The parent being stopped does not stop that child.
func TestUpgradeInFlightForkObservable(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "fork.c")
	binary := filepath.Join(root, "fork-fixture")
	code := `#include <unistd.h>
#include <stdio.h>
#include <stdlib.h>
int main(int argc,char **argv) {
 int p[2]; if(pipe(p)) return 2;
 pid_t child=fork(); if(child<0) return 3;
 if(child==0) { close(p[0]); char c='r'; write(p[1],&c,1); close(p[1]); if(read(0,&c,1)!=1) return 4; FILE *f=fopen(argv[2],"w"); fputs("child advanced after parent STOP",f); fclose(f); execl("/bin/sleep","sleep","30",NULL); return 5; }
 close(p[1]); char c; if(read(p[0],&c,1)!=1) return 6;
 FILE *f=fopen(argv[1],"w"); fprintf(f,"%d",child); fclose(f); sleep(30); return 0;
}`
	if err := os.WriteFile(src, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/clang", src, "-o", binary).CombinedOutput(); err != nil {
		t.Fatalf("clang: %v %s", err, out)
	}
	pidfile := filepath.Join(root, "pid")
	work := filepath.Join(root, "work")
	cmd := exec.Command(binary, pidfile, work)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(waitFixtureFile(t, pidfile)))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := Inspect(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	child, err := Inspect(pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		now, e := Inspect(pid)
		if e == nil && sameLifetime(now, child) {
			_ = signalIdentity(now, syscall.SIGKILL)
		}
		_ = signalIdentity(parent, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var stopped stoppedIdentity
	if err = freezeUpgradeHost(ctx, parent, &stopped); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resumeUpgradeProcess(stopped); err != nil {
			t.Error(err)
		}
	}()
	fmt.Fprintln(in, "go")
	waitFixtureFile(t, work)
	for {
		now, e := Inspect(pid)
		if e != nil {
			t.Fatal(e)
		}
		if now.Executable != child.Executable {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	frozen := map[int]stoppedIdentity{}
	defer func() {
		for _, id := range frozen {
			if e := resumeUpgradeProcess(id); e != nil {
				t.Error(e)
			}
		}
	}()
	if err = freezeUpgradeDescendants(ctx, parent, map[int]Identity{parent.PID: parent}, frozen); err != nil {
		t.Fatal(err)
	}
	if _, ok := frozen[pid]; !ok {
		t.Fatal("published in-flight child missed")
	}
	t.Log("published fork child advanced and execed after parent STOP; descendant scan subsequently froze it; no kernel publication-fence claim")
}
