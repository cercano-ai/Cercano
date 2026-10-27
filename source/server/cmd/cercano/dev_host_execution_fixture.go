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
		if fixtureWorker == nil || spec.Task != "blocked synthetic sibling" || len(spec.Tools) != 1 || spec.Tools[0] != "fixture_read" {
			return dispatch.Result{}, fmt.Errorf("restricted fixture worker request required")
		}
		result, err := fixtureWorker.RunTurn(ctx, runner.Request{ConversationID: spec.ConversationID + "-independent", Input: spec.Task}, devFixtureSink{}, nil, func(llm.Message) {})
		return dispatch.Result{Text: result.FinalText}, err
	})
	s.WireDevHostFixtureTasks()
}
func devHostExecutionAllowed(method string) bool {
	switch method {
	case proto.Agent_StreamProcessRequest_FullMethodName,
		proto.Agent_AttachConversation_FullMethodName,
		proto.Agent_SubscribeTasks_FullMethodName,
		proto.Agent_StreamResumeConversationViewportFirst_FullMethodName:
		return true
	default:
		return false
	}
}

var fixtureWorker runner.TurnRunner

type devFixtureSink struct{}

func (devFixtureSink) Emit(runner.Event) {}

// Validate before listening, constructing the server, or writing barrier files.
func validateFixtureBarrier(root, barrier string) error {
	if root == "" || barrier == "" || !filepath.IsAbs(root) || !filepath.IsAbs(barrier) || filepath.Clean(barrier) != barrier {
		return fmt.Errorf("invalid fixture barrier")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(barrier)
	if err != nil {
		return err
	}
	if resolved != resolvedRoot || resolved != barrier {
		return fmt.Errorf("fixture barrier must be declared canonical fixture root")
	}
	for _, name := range []string{"entered", "release"} {
		if _, err := os.Lstat(filepath.Join(barrier, name)); !os.IsNotExist(err) {
			return fmt.Errorf("fixture barrier must be fresh")
		}
	}
	return nil
}
func init() {
	if len(os.Args) > 1 && os.Args[1] == "dev-host" && os.Getenv("CERCANO_DISABLE_CREDENTIALS") == "1" {
		root := ""
		for i := 2; i+1 < len(os.Args); i++ {
			if os.Args[i] == "--fixture" {
				root = os.Args[i+1]
			}
		}
		// B attaches to the same barrier after A has entered it; only path
		// validation belongs here, freshness is checked by the worker write.
		barrier := os.Getenv("CERCANO_DEV_FIXTURE_BARRIER")
		r, e := filepath.EvalSymlinks(root)
		b, be := filepath.EvalSymlinks(barrier)
		if e != nil || be != nil || !filepath.IsAbs(barrier) || barrier != filepath.Clean(barrier) || b != r || b != barrier {
			fmt.Fprintln(os.Stderr, "invalid fixture barrier")
			os.Exit(91)
		}
	}
	if len(os.Args) < 2 || os.Args[1] != "worker" {
		return
	}
	socket, prepared, err := parseWorkerArgs(os.Args[2:])
	if err != nil {
		panic(err)
	}
	var options []worker.Option
	if prepared {
		options = append(options, worker.WithPrepared())
	}
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		os.Exit(90)
	}
	os.Setenv("HOME", filepath.Dir(socket))
	os.Setenv("XDG_CONFIG_HOME", filepath.Dir(socket))
	l, err := net.Listen("unix", socket)
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
	}, options...))
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
func (p *devFixtureProvider) StreamChat(ctx context.Context, req llm.ChatRequest) (llm.StreamReader, error) {
	b, _ := json.Marshal(req.Messages[len(req.Messages)-1])
	if !p.called && strings.Contains(string(b), "blocked synthetic sibling") {
		root := os.Getenv("CERCANO_DEV_FIXTURE_BARRIER")
		if err := validateFixtureBarrier(root, root); err != nil {
			return nil, err
		}
		marker := fmt.Sprintf("%s pid=%d", version, os.Getpid())
		if err := os.WriteFile(filepath.Join(root, "entered"), []byte(marker), 0600); err != nil {
			return nil, err
		}
		for {
			if _, err := os.Stat(filepath.Join(root, "release")); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
		return &devFixtureReader{events: []llm.StreamEvent{{Type: llm.EventMessageStart}, {Type: llm.EventTextDelta, TextDelta: "independent completed " + version}, {Type: llm.EventMessageStop, StopReason: "end_turn"}}}, nil
	}
	if p.called {
		return &devFixtureReader{events: []llm.StreamEvent{{Type: llm.EventMessageStart}, {Type: llm.EventTextDelta, TextDelta: version}, {Type: llm.EventMessageStop, StopReason: "end_turn"}}}, nil
	}
	p.called = true
	name := "fixture_read"
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
	fixtureWorker = w.(runner.TurnRunner)
	w.SetProfileReader(func(id string) agent.Profile {
		p := agent.PlanProfile()
		if !strings.HasSuffix(id, "-independent") {
			p.ExtraCaps["fixture_launch"] = true
		}
		return p
	})
}
