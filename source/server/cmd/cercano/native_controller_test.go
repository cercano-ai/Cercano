package main

import (
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
	pb "cercano/source/server/pkg/proto"
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
	// Preserve evidence on any failure, especially uncertain detached-host cleanup.
	cleanupComplete := false
	t.Cleanup(func() {
		if cleanupComplete && !t.Failed() {
			_ = os.RemoveAll(root)
		} else {
			t.Logf("preserved fixture state/logs: %s", root)
		}
	})
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
	// Inventory is observation only: never use discovered PIDs as signal authority.
	owned := func() ([]nativeControllerProcess, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "pid=,command=").Output()
		if err != nil {
			return nil, err
		}
		var processes []nativeControllerProcess
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) < 3 || f[1] != kegs[0] {
				continue
			}
			pid, err := strconv.Atoi(f[0])
			if err != nil {
				return nil, err
			}
			processes = append(processes, nativeControllerProcess{pid, f[2]})
		}
		return processes, nil
	}
	var selected *launchSelection
	var closeFixtures []func()
	t.Cleanup(func() {
		defer func() {
			for i := len(closeFixtures) - 1; i >= 0; i-- {
				closeFixtures[i]()
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if selected != nil && selected.Identity != nil {
			controls, err := launchControls(ctx, *selected)
			if err == nil {
				defer controls.Close()
				_, err = controls.LifecycleRelease(ctx, &pb.LifecycleControlRequest{Target: selected.Identity, Owner: &pb.LifecycleOwner{Host: selected.Identity.Host, Conversation: selected.Conversation, Epoch: selected.Epoch}, Management: true})
				if err == nil {
					var response *pb.LifecycleControlResponse
					response, err = controls.LifecycleRetire(ctx, &pb.LifecycleControlRequest{Target: selected.Identity})
					if err == nil && !response.GetReady() {
						err = fmt.Errorf("retirement not ready")
					}
				}
			}
			if err != nil {
				t.Errorf("authenticated release/retire failed: %v; escalation required, no signals sent", err)
			}
		}
		for {
			processes, err := owned()
			if err != nil {
				t.Errorf("cleanup inventory failed: %v; escalation required", err)
				return
			}
			if len(processes) == 0 {
				cleanupComplete = true
				t.Log("owned process cleanup complete (no signals)")
				return
			}
			if ctx.Err() != nil || selected == nil {
				t.Errorf("residual processes=%+v; state=%s; escalation required, no signals sent", processes, root)
				return
			}
			time.Sleep(50 * time.Millisecond)
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
		selected = &s
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
	closeFixtures = append(closeFixtures, endpoint.Close)
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
	closeFixtures = append(closeFixtures, g.Stop)
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
	before, err := owned()
	if err != nil {
		t.Fatal(err)
	}
	if err := nativeControllerInventory(before, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("before turn owned PIDs=%v binary_sha256=%x", before, sha256.Sum256(body))
	var messages []agentclient.StreamMsg
	stream, err := c.StreamChat(ctx, a.Conversation, "Reply with native-controller-ok", root)
	if err != nil {
		t.Fatal(err)
	}
	for msg := range stream {
		messages = append(messages, msg)
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
	after, err := owned()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("persisted=%+v calls=%d owned=%+v", turns, calls.Load(), after)
	if provider.StartCalls() != 1 {
		t.Fatal("unexpected synthetic runtime restart")
	}
	if calls.Load() != 1 {
		t.Fatalf("completion calls=%d, want exactly one", calls.Load())
	}
	if err := nativeControllerFinal(messages); err != nil {
		t.Error(err)
	}
	if err := nativeControllerTurns(turns); err != nil {
		t.Error(err)
	}
	if err := nativeControllerInventory(after, before); err != nil {
		t.Fatal(err)
	}
	for _, process := range after {
		if process.role != "worker" {
			continue
		}
		probeCtx, stopProbe := context.WithTimeout(context.Background(), 3*time.Second)
		mapped, probeErr := exec.CommandContext(probeCtx, "/usr/sbin/lsof", "-a", "-p", strconv.Itoa(process.pid), "-d", "txt", "-Fn").Output()
		stopProbe()
		if probeErr != nil {
			t.Fatalf("mapped executable probe: %v", probeErr)
		}
		if !strings.Contains(string(mapped), "n"+kegs[0]+"\n") {
			t.Fatalf("worker executable mapping mismatch: %s", mapped)
		}
		t.Logf("fresh worker PID=%d canonical executable=%s mapped=%s", process.pid, kegs[0], mapped)
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

// These assertions deliberately do not infer successful completion from tokens.
func nativeControllerFinal(messages []agentclient.StreamMsg) error {
	finals := 0
	for _, msg := range messages {
		if msg.Err != nil || msg.Type == agentclient.TypeError {
			return fmt.Errorf("stream error: %v", msg.Err)
		}
		if msg.Type == agentclient.TypeDone {
			finals++
			if msg.Final != "native-controller-ok" {
				return fmt.Errorf("unexpected final: %q", msg.Final)
			}
		}
	}
	if finals != 1 {
		return fmt.Errorf("final count=%d, want one", finals)
	}
	return nil
}
func nativeControllerTurns(turns []agentclient.PersistedTurn) error {
	if len(turns) != 2 || turns[0].Role != "user" || turns[0].Content != "Reply with native-controller-ok" || turns[1].Role != "assistant" || turns[1].Content != "native-controller-ok" {
		return fmt.Errorf("unexpected ordered turns: %+v", turns)
	}
	return nil
}

type nativeControllerProcess struct {
	pid  int
	role string
}

func nativeControllerInventory(now, before []nativeControllerProcess) error {
	hosts, workers := 0, 0
	for _, p := range now {
		switch p.role {
		case "development-host":
			hosts++
		case "worker":
			workers++
		default:
			return fmt.Errorf("unexpected role: %+v", p)
		}
	}
	wantWorkers := 0
	if before != nil {
		wantWorkers = 1
	}
	if hosts != 1 || workers != wantWorkers {
		return fmt.Errorf("inventory=%+v, want one host and %d workers", now, wantWorkers)
	}
	if before != nil {
		if err := nativeControllerInventory(before, nil); err != nil {
			return err
		}
		for _, p := range now {
			if p.role == "development-host" && p.pid != before[0].pid {
				return fmt.Errorf("host changed: %+v", now)
			}
			if p.role == "worker" && p.pid == before[0].pid {
				return fmt.Errorf("worker not fresh: %+v", now)
			}
		}
	}
	return nil
}
func TestNativeControllerAssertions(t *testing.T) {
	good := agentclient.StreamMsg{Type: agentclient.TypeDone, Final: "native-controller-ok"}
	for name, messages := range map[string][]agentclient.StreamMsg{
		"token only": {{Type: agentclient.TypeToken, Token: "native-controller-ok"}},
		"substring":  {{Type: agentclient.TypeDone, Final: "not native-controller-ok"}},
		"duplicate":  {good, good}, "empty final": {{Type: agentclient.TypeDone}},
		"error after final": {good, {Type: agentclient.TypeError, Err: fmt.Errorf("failed")}},
	} {
		t.Run(name, func(t *testing.T) {
			if nativeControllerFinal(messages) == nil {
				t.Fatal("accepted invalid stream")
			}
		})
	}
	if err := nativeControllerFinal([]agentclient.StreamMsg{{Type: agentclient.TypeToken, Token: "native-controller-ok"}, good}); err != nil {
		t.Fatal(err)
	}
	turns := []agentclient.PersistedTurn{{Role: "user", Content: "Reply with native-controller-ok"}, {Role: "assistant", Content: "native-controller-ok"}}
	if err := nativeControllerTurns(turns); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]agentclient.PersistedTurn{turns[1:], {turns[1], turns[0]}, append(append([]agentclient.PersistedTurn{}, turns...), turns[1]), {{Role: "user", Content: "wrong"}, turns[1]}} {
		if nativeControllerTurns(bad) == nil {
			t.Fatal("accepted invalid persistence")
		}
	}
	host := []nativeControllerProcess{{1, "development-host"}}
	if err := nativeControllerInventory(host, nil); err != nil {
		t.Fatal(err)
	}
	if err := nativeControllerInventory([]nativeControllerProcess{{1, "development-host"}, {2, "worker"}}, host); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]nativeControllerProcess{nil, {{1, "worker"}}, {{1, "development-host"}, {2, "development-host"}}, {{1, "development-host"}, {2, "worker"}}} {
		if nativeControllerInventory(bad, nil) == nil {
			t.Fatal("accepted invalid preturn inventory")
		}
	}
	for _, bad := range [][]nativeControllerProcess{host, {{2, "development-host"}, {1, "worker"}}, {{1, "development-host"}, {2, "worker"}, {3, "worker"}}} {
		if nativeControllerInventory(bad, host) == nil {
			t.Fatal("accepted invalid postturn inventory")
		}
	}
}
