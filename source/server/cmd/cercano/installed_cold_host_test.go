//go:build !windows

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
)

// Unlike the composition fixture, this builds the shipping entrypoint and starts
// with no host records. No TestMain, dependency factory or prepublished readiness.
func TestInstalledProductionColdHost(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "ich-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "built-cercano")
	buildCtx, stopBuild := context.WithTimeout(context.Background(), 90*time.Second)
	defer stopBuild()
	// Reuse only the existing compiler/module caches; do not inherit runtime
	// credentials, proxies, Go flags/tags, or user config into the product build.
	cacheCmd := exec.CommandContext(buildCtx, "go", "env", "GOCACHE", "GOPATH")
	cacheOut, err := cacheCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	caches := strings.Split(strings.TrimSpace(string(cacheOut)), "\n")
	if len(caches) != 2 {
		t.Fatalf("unexpected Go cache paths: %q", cacheOut)
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	build := exec.CommandContext(buildCtx, goBinary, "build", "-tags=", "-o", binary, ".")
	build.Env = []string{"PATH=" + filepath.Dir(goBinary) + ":/usr/bin:/bin", "HOME=" + root, "XDG_CONFIG_HOME=" + root, "XDG_DATA_HOME=" + root, "XDG_CACHE_HOME=" + root, "TMPDIR=" + root, "GOCACHE=" + caches[0], "GOPATH=" + caches[1], "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOENV=off", "CERCANO_DISABLE_CREDENTIALS=1"}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("production build: %v\n%s", err, out)
	}
	body, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	state, runtime := filepath.Join(root, "state"), filepath.Join(root, "rt")
	for _, dir := range []string{state, runtime, filepath.Join(state, "hosts")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := conversation.PrepareDevelopmentStore(context.Background(), filepath.Join(state, "conversations.db")); err != nil {
		t.Fatal(err)
	}
	// No local runtime, provider call or OS credential backend is needed for
	// readiness/acquisition. The child environment is an allowlist, not inherited.
	if err := os.WriteFile(filepath.Join(state, "config.yaml"), []byte("locus_mode: cloud_only\nworker_idle_timeout_seconds: -1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + root, "XDG_CONFIG_HOME=" + root, "XDG_DATA_HOME=" + root, "XDG_CACHE_HOME=" + root, "XDG_STATE_HOME=" + root, "TMPDIR=" + root, "TMP=" + root, "TEMP=" + root, "CERCANO_DISABLE_CREDENTIALS=1"}
	kegs := make([]string, 2)
	for i, version := range []string{"A", "B"} {
		kegs[i] = filepath.Join(root, "Cellar", "cercano-agent", version, "bin", "cercano")
		if err := os.MkdirAll(filepath.Dir(kegs[i]), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(kegs[i], body, 0700); err != nil {
			t.Fatal(err)
		}
	}
	// The production launcher detaches its host. Identify only exact owned keg
	// commands, even on a readiness failure before a record exists. Never pkill.
	owned := func() []int {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,command=").Output()
		if err != nil {
			t.Errorf("owned PID inventory: %v", err)
			return nil
		}
		var pids []int
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			if (fields[1] == kegs[0] || fields[1] == kegs[1]) && fields[2] == "development-host" {
				pid, err := strconv.Atoi(fields[0])
				if err == nil {
					pids = append(pids, pid)
				}
			}
		}
		return pids
	}
	t.Cleanup(func() {
		for _, pid := range owned() {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if len(owned()) == 0 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		for _, pid := range owned() {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		time.Sleep(100 * time.Millisecond)
		if pids := owned(); len(pids) != 0 {
			t.Errorf("owned hosts remain: %v", pids)
		}
	})
	alias := filepath.Join(root, "cercano")
	selectKeg := func(keg string) {
		if err := os.Remove(alias); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Symlink(keg, alias); err != nil {
			t.Fatal(err)
		}
	}
	launch := func(keg string) (string, launchSelection) {
		generation := filepath.Dir(filepath.Dir(keg))
		record := filepath.Join(state, "hosts", fmt.Sprintf("%x.json", sha256.Sum256([]byte(generation))))
		if _, err := os.Stat(record); !os.IsNotExist(err) {
			t.Fatalf("host was not cold: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, alias, "development-launch", "--installed", "--state", state, "--runtime-root", runtime)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			logs, _ := filepath.Glob(filepath.Join(state, "hosts", "*.log"))
			for _, log := range logs {
				b, _ := os.ReadFile(log)
				t.Logf("%s:\n%s", log, b)
			}
			t.Fatalf("cold launch: %v\n%s", err, out)
		}
		path := strings.TrimSpace(string(out))
		var s launchSelection
		if err := launchRead(path, &s); err != nil {
			t.Fatalf("persisted selection %q: %v", out, err)
		}
		if s.Fixture || s.Identity == nil || s.Identity.Host == "" || s.Conversation == "" || s.Epoch == "" || s.Generation != generation {
			t.Fatalf("invalid production selection: %+v", s)
		}
		var host launchSelection
		if err := launchRead(record, &host); err != nil {
			t.Fatal(err)
		}
		if host.Fixture || host.Identity.GetHost() != s.Identity.Host || host.Endpoint != s.Endpoint || host.Conversation != "" {
			t.Fatalf("host record: %+v", host)
		}
		var current launchSelection
		if err := launchRead(filepath.Join(state, "current-target.json"), &current); err != nil {
			t.Fatal(err)
		}
		if current.Identity.GetHost() != s.Identity.Host {
			t.Fatal("discovery did not publish selected host")
		}
		return path, s
	}
	dial := func(s launchSelection) *agentclient.Client {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: s.Endpoint, HostInstance: s.Identity.Host, Build: s.Identity.Build, Contract: s.Identity.Contract, SchemaVersion: s.Identity.Schema, ConversationID: s.Conversation, OwnershipEpoch: s.Epoch}, s.TokenFile)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}
	entries, err := os.ReadDir(filepath.Join(state, "hosts"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("hosts not initially empty: %v %v", entries, err)
	}
	selectKeg(kegs[0])
	pathA, a := launch(kegs[0])
	ca := dial(a)
	selectKeg(kegs[1])
	_, b := launch(kegs[1])
	dial(b)
	if a.Identity.Host == b.Identity.Host || a.Conversation == b.Conversation || a.Generation == b.Generation {
		t.Fatal("B reused A")
	}
	if pids := owned(); len(pids) != 2 {
		t.Fatalf("expected two live production hosts, got %v", pids)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ca.ReconnectPinnedSession(ctx); err != nil {
		t.Fatalf("A reconnect: %v", err)
	}
	var persisted launchSelection
	if err := launchRead(pathA, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Identity.GetHost() != a.Identity.Host || persisted.Conversation != a.Conversation || persisted.Epoch != a.Epoch {
		t.Fatal("A binding changed")
	}
	dial(persisted)
	controls, err := launchControls(ctx, persisted)
	if err != nil {
		t.Fatal(err)
	}
	defer controls.Close()
	owners, err := controls.LifecycleList(ctx, &proto.LifecycleControlRequest{Target: a.Identity, Management: true})
	if err != nil || owners.GetIdentity().GetHost() != a.Identity.Host {
		t.Fatalf("A is not serving: %v %v", owners, err)
	}
	found := false
	for _, owner := range owners.OpenOwners {
		if owner.Host == a.Identity.Host && owner.Conversation == a.Conversation && owner.Epoch == a.Epoch {
			found = true
		}
	}
	if !found {
		t.Fatalf("A lost acquired owner: %v", owners)
	}
	t.Logf("production cold A/B ready; A=%s B=%s; persisted A owner retained; live owned PIDs=%v", a.Identity.Host, b.Identity.Host, owned())
}
