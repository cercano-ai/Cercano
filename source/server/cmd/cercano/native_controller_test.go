package main

import (
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"fmt"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
)

// Unlike the composition fixture, this builds the shipping entrypoint and starts
// with no host records. No TestMain, dependency factory or prepublished readiness.
func TestNativeProductionController(t *testing.T) {
	if os.Getenv("CERCANO_NATIVE_CONTROLLER") != "1" {
		t.Skip("opt-in native controller")
	}
	parent := os.Getenv("CERCANO_NATIVE_STATE_PARENT")
	if parent == "" {
		parent = "/tmp"
	}
	root, err := os.MkdirTemp(parent, "nc-")
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
	build.Env = []string{"PATH=" + filepath.Dir(goBinary) + ":/usr/bin:/bin", "HOME=" + root, "XDG_CONFIG_HOME=" + root, "XDG_DATA_HOME=" + root, "XDG_CACHE_HOME=" + root, "TMPDIR=" + root, "GOCACHE=" + caches[0], "GOPATH=" + caches[1], "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "GOENV=off", "GOMAXPROCS=2", "GOFLAGS=-p=2", "CERCANO_DISABLE_CREDENTIALS=1"}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("production build: %v\n%s", err, out)
	}
	if supplied := os.Getenv("CERCANO_NATIVE_AGENT"); supplied != "" {
		binary = supplied
	}
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range info.Settings {
		if setting.Key == "-tags" && setting.Value != "" {
			t.Fatalf("tagged product refused: %s", setting.Value)
		}
	}
	if info.Path != "cercano/source/server/cmd/cercano" {
		t.Fatalf("not product entrypoint: %s", info.Path)
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
	// The child environment is an allowlist, not inherited.
	if err := os.WriteFile(filepath.Join(state, "config.yaml"), []byte("locus_mode: open_only\nopen_runtime: llama_server\nllama_server:\n  enabled: true\nmodels:\n  open:\n    overrides:\n      llama_server:\n        most_capable: m1\n        balanced: m1\n        fastest: m1\nworker_idle_timeout_seconds: -1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + root, "XDG_CONFIG_HOME=" + root, "XDG_DATA_HOME=" + root, "XDG_CACHE_HOME=" + root, "XDG_STATE_HOME=" + root, "TMPDIR=" + root, "TMP=" + root, "TEMP=" + root, "CERCANO_DISABLE_CREDENTIALS=1"}
	kegs := make([]string, 1)
	for i, version := range []string{"A"} {
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
			if (fields[1] == kegs[0]) && (fields[2] == "development-host" || fields[2] == "worker") {
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
				t.Log("owned process cleanup complete")
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

	var calls atomic.Int64
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"native-controller-ok"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer endpoint.Close()
	cfgPath := filepath.Join(state, "config.yaml")
	cfgBody, _ := os.ReadFile(cfgPath)
	cfgBody = append(cfgBody, []byte("ollama_url: "+endpoint.URL+"\n")...)
	if err := os.WriteFile(cfgPath, cfgBody, 0600); err != nil {
		t.Fatal(err)
	}
	provider := localruntimetest.NewFakeProvider("llama_server", localruntime.ModelRecord{ID: "m1", Runtime: "llama_server", Path: "/synthetic/not-a-real-model.gguf", DownloadState: localruntime.Downloaded})
	provider.Endpoint = endpoint.URL
	provider.Context = localruntime.ContextCapacity{ConfirmedTokens: 32768, ConfirmedAt: time.Now()}
	manager := localruntime.NewManager()
	manager.RegisterProvider(provider)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	if _, err := manager.Start(ctx, localruntime.StartRequest{Runtime: "llama_server", ModelID: "m1"}); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	modelhost.NewServer(nativeControllerManager{manager}, modelhost.ServerOptions{Token: "native-controller-owned-token", CoexistenceProtection: true}).Register(g)
	go g.Serve(listener)
	defer g.Stop()
	modelDir := filepath.Join(root, ".cercano", "run", "modelhost")
	if err := modelhost.WriteRecord(modelDir, modelhost.Record{PID: os.Getpid(), Addr: listener.Addr().String(), Token: "native-controller-owned-token", APIVersion: modelhost.APIVersion, Version: version, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "modelhost.yaml"), []byte("run_dir: "+modelDir+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selectKeg(kegs[0])
	_, a := launch(kegs[0])
	c := dial(a)
	before := owned()
	t.Logf("before turn owned PIDs=%v binary_sha256=%x", before, sha256.Sum256(body))
	var output strings.Builder
	stream, err := c.StreamChat(ctx, a.Conversation, "Reply with native-controller-ok", root)
	if err != nil {
		t.Fatal(err)
	}
	for msg := range stream {
		output.WriteString(msg.Token)
		output.WriteString(msg.Final)
		t.Logf("stream: %+v", msg)
		if msg.Err != nil {
			t.Errorf("turn: %v", msg.Err)
			logs, _ := filepath.Glob(filepath.Join(state, "hosts", "*.log"))
			for _, log := range logs {
				b, _ := os.ReadFile(log)
				t.Logf("host log %s: %s", log, b)
			}
		}
	}
	turns, err := c.ResumeConversation(ctx, a.Conversation)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("persisted=%+v calls=%d owned=%v", turns, calls.Load(), owned())
	if provider.StartCalls() != 1 {
		t.Fatal("unexpected synthetic runtime restart")
	}
	if calls.Load() == 0 {
		t.Fatal("no completion calls")
	}
	if !strings.Contains(output.String(), "native-controller-ok") {
		t.Fatalf("missing output: %q", output.String())
	}
	found := false
	for _, turn := range turns {
		if turn.Role == "assistant" && turn.Content == "native-controller-ok" {
			found = true
		}
	}
	if !found {
		t.Fatal("assistant turn not persisted")
	}
	fresh := false
	for _, pid := range owned() {
		old := false
		for _, prior := range before {
			if prior == pid {
				old = true
			}
		}
		if !old {
			identity, err := exec.Command("/bin/ps", "-p", strconv.Itoa(pid), "-o", "comm=,command=").Output()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(identity), kegs[0]+" worker") {
				t.Fatalf("unexpected fresh executable: %s", identity)
			}
			probeCtx, stopProbe := context.WithTimeout(context.Background(), 3*time.Second)
			mapped, probeErr := exec.CommandContext(probeCtx, "/usr/sbin/lsof", "-a", "-p", strconv.Itoa(pid), "-d", "txt", "-Fn").Output()
			stopProbe()
			if probeErr != nil {
				t.Fatalf("mapped executable probe: %v", probeErr)
			}
			if !strings.Contains(string(mapped), "n"+kegs[0]+"\n") {
				t.Fatalf("worker executable mapping mismatch: %s", mapped)
			}
			t.Logf("fresh worker PID=%d executable=%s mapped=%s", pid, identity, mapped)
			fresh = true
		}
	}
	if !fresh {
		t.Fatal("no fresh owned worker")
	}
}

// Resolve synthetic metadata without asking a real on-disk model catalog.
type nativeControllerManager struct{ localruntime.Manager }

func (m nativeControllerManager) ResolveOpenModel(ctx context.Context, runtime, want string) (localruntime.ModelRecord, error) {
	if runtime != "llama_server" || want != "m1" {
		return localruntime.ModelRecord{}, fmt.Errorf("unexpected synthetic model %s/%s", runtime, want)
	}
	return localruntime.ModelRecord{ID: "m1", Runtime: runtime, Path: "/synthetic/not-a-real-model.gguf", DownloadState: localruntime.Downloaded}, nil
}

func (m nativeControllerManager) EnsureModelsPresent(ctx context.Context, runtime string, want []string) error {
	for _, id := range want {
		if _, err := m.ResolveOpenModel(ctx, runtime, id); err != nil {
			return err
		}
	}
	return nil
}
