//go:build darwin && cgo

package brewrestart

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPrivateStdioPairs(t *testing.T) {
	child := []upgradePipe{{0, 1, 2, 1}, {1, 3, 4, 2}}
	host := []upgradePipe{{7, 2, 1, 2}, {8, 4, 3, 1}}
	if !privateStdioPairs(host, child) {
		t.Fatal("valid pair refused")
	}
	if !privateStdioPairs(append(host, host[0]), child) {
		t.Fatal("duplicate endpoint refused")
	}
	for name, h := range map[string][]upgradePipe{"none": nil, "one": host[:1], "wrong-peer": {{7, 2, 9, 2}, {8, 4, 3, 1}}, "wrong-mode": {{7, 2, 1, 1}, {8, 4, 3, 1}}, "ambiguous": append(append([]upgradePipe{}, host...), upgradePipe{9, 2, 99, 2})} {
		t.Run(name, func(t *testing.T) {
			if privateStdioPairs(h, child) {
				t.Fatal("unsafe pair accepted")
			}
		})
	}
	for name, c := range map[string][]upgradePipe{
		"single-pipe":     {{0, 1, 2, 1}, {1, 2, 1, 2}},
		"missing-stdout":  child[:1],
		"not-stdin":       {{2, 1, 2, 1}, {1, 3, 4, 2}},
		"duplicate-stdin": append(append([]upgradePipe{}, child...), child[0]),
		"zero-handle":     {{0, 0, 2, 1}, {1, 3, 4, 2}},
		"zero-peer":       {{0, 1, 0, 1}, {1, 3, 4, 2}},
		"wrong-direction": {{0, 1, 2, 2}, {1, 3, 4, 1}},
		"duplex":          {{0, 1, 2, 3}, {1, 3, 4, 3}},
	} {
		t.Run(name, func(t *testing.T) {
			if privateStdioPairs(host, c) {
				t.Fatal("unsafe child endpoints accepted")
			}
		})
	}
}

// Real external shell, not a renamed Go test binary. Native pipe evidence
// establishes the root; its attached sleep child uses the existing tree path.
func TestPrivateStdioNativeRootAndDescendant(t *testing.T) {
	root := t.TempDir()
	pidfile := filepath.Join(root, "pid")
	cmd := exec.Command("/bin/sh", "-c", `/bin/sleep 60 & echo $! > "$1"; wait`, "fixture", pidfile)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := Inspect(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	host, err := Inspect(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(waitFixtureFile(t, pidfile))))
	if err != nil {
		t.Fatal(err)
	}
	id, err = Inspect(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	child, err := Inspect(pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = signalIdentity(child, syscall.SIGKILL)
		_ = signalIdentity(id, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	w := &stdioWitness{}
	if ok, e := verifiedPrivateStdio(host, id, w); e != nil || !ok {
		t.Fatalf("native evidence: %v %v", ok, e)
	}
	changed := &stdioWitness{host: append([]upgradePipe{}, w.host...), child: w.child}
	changed.host[0].Handle++
	if ok, _ := verifiedPrivateStdio(host, id, changed); ok {
		t.Fatal("changed witness accepted")
	}
	wrongParent := id
	wrongParent.ParentPID++
	if ok, _ := verifiedPrivateStdio(host, wrongParent); ok {
		t.Fatal("wrong parent accepted")
	}
	wrongUID := id
	wrongUID.UID++
	if ok, _ := verifiedPrivateStdio(host, wrongUID); ok {
		t.Fatal("wrong UID accepted")
	}
	stale := id
	stale.StartMicroseconds++
	if ok, _ := verifiedPrivateStdio(host, stale); ok {
		t.Fatal("reused identity accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	frozen := map[int]stoppedIdentity{}
	checks := 0
	check := func(i Identity) error {
		checks++
		_, recorded := frozen[i.PID]
		if recorded != (checks == 2) {
			t.Fatal("expected ownership checks before and after root freeze")
		}
		ok, e := verifiedPrivateStdio(host, i, w)
		if e != nil {
			return e
		}
		if !ok {
			return syscall.EINVAL
		}
		return nil
	}
	if err = freezeUpgradeDescendants(ctx, host, map[int]Identity{id.PID: id}, frozen, check); err != nil {
		t.Fatal(err)
	}
	if len(frozen) != 2 || checks != 2 {
		t.Fatalf("frozen count %d, ownership checks %d", len(frozen), checks)
	}
	for _, f := range frozen {
		if err = signalIdentity(f.Identity, syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
	}
	_ = cmd.Wait()
	for _, f := range frozen {
		if err = waitProcessExit(ctx, f.Identity); err != nil {
			t.Fatal(err)
		}
	}
}
