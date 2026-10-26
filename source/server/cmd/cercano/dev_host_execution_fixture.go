//go:build devhostfixture

package main

// Dedicated acceptance build only. No runtime switch enables this in releases.
import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/dispatch"
	providers "cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/locus"
	"cercano/source/server/internal/routinglog"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/usage"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func configureDevHostExecution(s *server.Server) {
	s.SetCloudLLMProvider(&devFixtureProvider{})
	r := agenttools.NewRegistry()
	r.Register(devFixtureRead{})
	s.SetToolRegistry(r)
	engine := dispatch.NewEngine(func() inference.Tiers {
		return inference.Tiers{Open: &devFixtureProvider{}, ModelFor: func(inference.Selection, config.Tier) string { return "synthetic" }, TaskFor: func(config.Task) config.TaskAssignment {
			return config.TaskAssignment{Destination: config.DestinationPrimary}
		}}
	}, func() locus.Mode { return locus.Mode("local") }, nil)
	s.SetDispatchEngine(engine)
	engine.SetAgenticRunner(func(ctx context.Context, spec dispatch.Spec, _ inference.Selection, _ string) (dispatch.Result, error) {
		root := os.Getenv("CERCANO_DEV_FIXTURE_BARRIER")
		if root == "" {
			return dispatch.Result{}, fmt.Errorf("missing fixture barrier")
		}
		if err := os.WriteFile(filepath.Join(root, "entered"), []byte(version), 0600); err != nil {
			return dispatch.Result{}, err
		}
		for {
			if _, err := os.Stat(filepath.Join(root, "release")); err == nil {
				return dispatch.Result{Text: "independent completed " + version}, nil
			}
			select {
			case <-ctx.Done():
				return dispatch.Result{}, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
	})
	s.WireDevHostFixtureTasks()
}
func devHostExecutionAllowed(method string) bool {
	return method == proto.Agent_StreamProcessRequest_FullMethodName
}
func init() {
	if len(os.Args) != 4 || os.Args[1] != "worker" || os.Args[2] != "--socket" {
		return
	}
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		os.Exit(90)
	}
	os.Setenv("HOME", filepath.Dir(os.Args[3]))
	os.Setenv("XDG_CONFIG_HOME", filepath.Dir(os.Args[3]))
	l, err := net.Listen("unix", os.Args[3])
	if err != nil {
		panic(err)
	}
	g := grpc.NewServer(grpc.MaxRecvMsgSize(worker.MaxMsgBytes), grpc.MaxSendMsgSize(worker.MaxMsgBytes))
	proto.RegisterWorkerServer(g, worker.NewWithFactories(func(*proto.StartTurn) (providers.Resolver, error) {
		return &devFixtureResolver{p: &devFixtureProvider{}}, nil
	}, func(start *proto.StartTurn) (runner.ToolSvc, error) {
		r := agenttools.NewRegistry()
		r.Register(devFixtureRead{})
		r.Register(devFixtureSentinel{})
		return &devFixtureTools{r: r, parent: start.GetConversationId()}, nil
	}))
	if err := g.Serve(l); err != nil {
		panic(err)
	}
	os.Exit(0)
}

type devFixtureResolver struct {
	providers.Resolver
	p inference.Provider
}

func (r *devFixtureResolver) Main() (inference.Provider, bool, bool, error) {
	return r.p, true, false, nil
}
func (*devFixtureResolver) MainModel(bool) string       { return "synthetic" }
func (*devFixtureResolver) PrimaryModel() string        { return "synthetic" }
func (*devFixtureResolver) LocusMode() string           { return "cloud_primary" }
func (*devFixtureResolver) Cloud() inference.Provider   { return nil }
func (*devFixtureResolver) Open() inference.Provider    { return nil }
func (*devFixtureResolver) Candidates() inference.Tiers { return inference.Tiers{} }

type devFixtureTools struct {
	r      *agenttools.Registry
	parent string
}

func (t *devFixtureTools) SetFixtureTasks(tasks capabilities.Tasks) {
	t.r.Register(devFixtureLaunch{tasks: tasks, parent: t.parent})
}

type devFixtureLaunch struct {
	devFixtureRead
	tasks  capabilities.Tasks
	parent string
}

func (devFixtureLaunch) Name() string                      { return "fixture_launch" }
func (devFixtureLaunch) Permission() agenttools.Permission { return agenttools.PermW }
func (t devFixtureLaunch) Execute(ctx context.Context, _ json.RawMessage) (*agenttools.Result, error) {
	r, e := t.tasks.Launch(ctx, dispatch.Spec{Mode: dispatch.Agentic, ConversationID: t.parent, Task: "blocked synthetic sibling", Tools: []string{"fixture_read"}})
	if e != nil {
		return nil, e
	}
	return &agenttools.Result{Text: "launched " + r.ID}, nil
}

func (t *devFixtureTools) Registry() *agenttools.Registry { return t.r }
func (t *devFixtureTools) GrantedRegistry([]string) (*agenttools.Registry, []string, []string, error) {
	return t.r, nil, nil, nil
}

type devFixtureProvider struct{ called bool }

func (*devFixtureProvider) Name() string { return "dev-host-synthetic" }
func (*devFixtureProvider) Capabilities() inference.Capabilities {
	return inference.Capabilities{SupportsTools: true}
}
func (*devFixtureProvider) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	panic("unexpected nonstream fixture execution")
}
func (p *devFixtureProvider) StreamChat(_ context.Context, req llm.ChatRequest) (llm.StreamReader, error) {
	if p.called {
		return &devFixtureReader{events: []llm.StreamEvent{{Type: llm.EventMessageStart}, {Type: llm.EventTextDelta, TextDelta: version}, {Type: llm.EventMessageStop, StopReason: "end_turn"}}}, nil
	}
	p.called = true
	name := "fixture_read"
	b, _ := json.Marshal(req.Messages[len(req.Messages)-1])
	if strings.Contains(string(b), "launch independent") {
		name = "fixture_launch"
	}
	return &devFixtureReader{events: []llm.StreamEvent{{Type: llm.EventMessageStart}, {Type: llm.EventToolUseStart, ToolUseID: "allowed", ToolName: name, ToolInputRaw: json.RawMessage(`{}`)}, {Type: llm.EventToolUseStop}, {Type: llm.EventToolUseStart, ToolUseID: "forbidden", ToolName: "run_command", ToolInputRaw: json.RawMessage(`{}`)}, {Type: llm.EventToolUseStop}, {Type: llm.EventMessageStop, StopReason: "tool_use"}}}, nil
}

type devFixtureReader struct {
	events []llm.StreamEvent
	pos    int
}

func (r *devFixtureReader) Next() (llm.StreamEvent, bool, error) {
	if r.pos == len(r.events) {
		return llm.StreamEvent{}, false, nil
	}
	e := r.events[r.pos]
	r.pos++
	return e, true, nil
}
func (*devFixtureReader) Close() error { return nil }

type devFixtureRead struct{}

func (devFixtureRead) Name() string                      { return "fixture_read" }
func (devFixtureRead) Description() string               { return "synthetic read" }
func (devFixtureRead) Permission() agenttools.Permission { return agenttools.PermR }
func (devFixtureRead) Schema() json.RawMessage           { return json.RawMessage(`{"type":"object"}`) }
func (devFixtureRead) Execute(context.Context, json.RawMessage) (*agenttools.Result, error) {
	return &agenttools.Result{Text: "synthetic read completed"}, nil
}

type devFixtureSentinel struct{ devFixtureRead }

func (devFixtureSentinel) Name() string                      { return "run_command" }
func (devFixtureSentinel) Permission() agenttools.Permission { return agenttools.PermW }
func (devFixtureSentinel) Execute(context.Context, json.RawMessage) (*agenttools.Result, error) {
	panic("profile fence bypass: synthetic sentinel executed")
}

func (*devFixtureResolver) SetRoutingLog(*routinglog.Writer)         {}
func (*devFixtureResolver) SetUsageSink(func(usage.Usage))           {}
func (*devFixtureResolver) SetModelSupportsVision(func(string) bool) {}

func configureDevHostWorker(w worker.ProfileReaderSetter) {
	w.SetProfileReader(func(string) agent.Profile { p := agent.PlanProfile(); p.ExtraCaps["fixture_launch"] = true; return p })
}
