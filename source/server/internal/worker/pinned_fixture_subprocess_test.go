package worker_test

// This entry exists only in the test executable. The production CLI cannot
// select injected factories or fall back to them.
import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/llm"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agenttools"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	providers "cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

func init() {
	if os.Getenv("CERCANO_PINNED_TEST_WORKER") != "1" || len(os.Args) != 4 || os.Args[1] != "worker" || os.Args[2] != "--socket" {
		return
	}
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		os.Exit(90)
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(91)
	}
	listener, err := net.Listen("unix", os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(92)
	}
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(worker.MaxMsgBytes), grpc.MaxSendMsgSize(worker.MaxMsgBytes))
	proto.RegisterWorkerServer(srv, worker.NewWithFactories(
		func(*proto.StartTurn) (providers.Resolver, error) {
			return &fakeResolver{prov: &fencedFixtureProvider{fixedProvider: fixedProvider{text: filepath.Base(filepath.Dir(exe))}}}, nil
		},
		func(*proto.StartTurn) (runner.ToolSvc, error) {
			reg := agenttools.NewRegistry()
			reg.Register(fencedFixtureTool{})
			reg.Register(readFixtureTool{})
			return &fakeToolSvc{reg: reg}, nil
		},
	))
	if err := srv.Serve(listener); err != nil {
		os.Exit(93)
	}
	os.Exit(0)
}

// Real process coverage for the existing policy spawn path, deliberately not
// presented as durable A/B host handover or independent-task acceptance.
func TestPinnedFixtureSubprocessGenerations(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("CERCANO_PINNED_TEST_WORKER", "1")
	root, err := os.MkdirTemp("/tmp", "gp-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"home", "config", "tmp"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("TMPDIR", filepath.Join(root, "tmp"))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, generation := range []string{"A", "B"} {
		dir := filepath.Join(root, generation)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cercano"), binary, 0700); err != nil {
			t.Fatal(err)
		}
		manifest := filepath.Join(dir, "manifest.json")
		if err := os.WriteFile(manifest, []byte(`{"fixture":true}`), 0600); err != nil {
			t.Fatal(err)
		}
		policy, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: dir, ManifestPath: manifest, HostInstance: generation, ResourceRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		cfg := config.Config{LocusMode: "open_primary", WorkerIdleTimeoutSeconds: -1}
		r := worker.NewWorkerRunnerWithPolicy(policy, &recordingHistory{}, cfgsvc.New("", cfg, secrets.NewMemory()), newTestBroker(), nil, nil, nil, nil, nil)
		defer r.(interface{ CleanupOwnedWorkers() }).CleanupOwnedWorkers()
		r.(interface {
			SetProfileReader(func(string) agent.Profile)
		}).SetProfileReader(func(string) agent.Profile { return agent.PlanProfile() })
		sink := &sinkCollector{}
		if _, err := r.RunTurn(ctx, runner.Request{ConversationID: "foreground", Input: "identify generation", WorkDir: root, Gen: 1}, sink, nil, nil); err != nil {
			t.Fatal(err)
		}
		blocked := false
		allowed := false
		for _, event := range sink.events {
			if event.Kind == runner.EventToolExecComplete && event.ToolName == "fixture_read" && !event.IsError {
				allowed = true
			}
			if event.Kind == runner.EventToolExecComplete && event.ToolName == "run_command" && event.IsError && event.Summary == "blocked by plan profile" {
				blocked = true
			}
		}
		if !allowed {
			t.Fatalf("allowed synthetic read did not execute: %+v", sink.events)
		}
		if !blocked {
			t.Fatalf("missing profile fence rejection: %+v", sink.events)
		}
		found := false
		for _, event := range sink.events {
			if event.Text == generation {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing generation %s output: %+v", generation, sink.events)
		}
	}
}

// The provider deliberately requests a registered forbidden capability even
// though the profile filters its advertised catalog. Execution must be fenced.
type fencedFixtureProvider struct {
	fixedProvider
	called bool
}

func (p *fencedFixtureProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (llm.StreamReader, error) {
	if p.called {
		return p.fixedProvider.StreamChat(ctx, req)
	}
	p.called = true
	return &fixedReader{events: []llm.StreamEvent{
		{Type: llm.EventMessageStart},
		{Type: llm.EventToolUseStart, ToolUseID: "allowed", ToolName: "fixture_read", ToolInputRaw: json.RawMessage(`{}`)},
		{Type: llm.EventToolUseStop},
		{Type: llm.EventToolUseStart, ToolUseID: "forbidden", ToolName: "run_command", ToolInputRaw: json.RawMessage(`{}`)},
		{Type: llm.EventToolUseStop},
		{Type: llm.EventMessageStop, StopReason: "tool_use"},
	}}, nil
}

type fencedFixtureTool struct{}

func (fencedFixtureTool) Name() string { return "run_command" }
func (fencedFixtureTool) Description() string {
	return "synthetic forbidden sentinel; never executes a command"
}
func (fencedFixtureTool) Permission() agenttools.Permission { return agenttools.PermW }
func (fencedFixtureTool) Schema() json.RawMessage           { return json.RawMessage(`{"type":"object"}`) }
func (fencedFixtureTool) Execute(context.Context, json.RawMessage) (*agenttools.Result, error) {
	panic("profile fence bypass: synthetic sentinel executed")
}

type readFixtureTool struct{}

func (readFixtureTool) Name() string                      { return "fixture_read" }
func (readFixtureTool) Description() string               { return "deterministic synthetic read" }
func (readFixtureTool) Permission() agenttools.Permission { return agenttools.PermR }
func (readFixtureTool) Schema() json.RawMessage           { return json.RawMessage(`{"type":"object"}`) }
func (readFixtureTool) Execute(context.Context, json.RawMessage) (*agenttools.Result, error) {
	return &agenttools.Result{Text: "synthetic read completed"}, nil
}
