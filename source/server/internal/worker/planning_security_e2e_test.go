package worker_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/approvaldisclosure"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

type planningSecurityProvider struct {
	calls           atomic.Int32
	name            string
	args            json.RawMessage
	duringInference func()
}

func (*planningSecurityProvider) Name() string { return "planning-security" }
func (*planningSecurityProvider) Capabilities() inference.Capabilities {
	return inference.Capabilities{SupportsTools: true}
}
func (*planningSecurityProvider) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}
func (p *planningSecurityProvider) StreamChat(context.Context, llm.ChatRequest) (llm.StreamReader, error) {
	if p.calls.Add(1) == 1 {
		if p.duringInference != nil {
			p.duringInference()
		}
		return &scriptedReader{events: []llm.StreamEvent{{Type: llm.EventMessageStart}, {Type: llm.EventToolUseStart, ToolUseID: "probe", ToolName: p.name, ToolInputRaw: p.args}, {Type: llm.EventToolUseStop}, {Type: llm.EventMessageStop, StopReason: "tool_use"}}}, nil
	}
	return &scriptedReader{events: []llm.StreamEvent{{Type: llm.EventMessageStart}, {Type: llm.EventTextDelta, TextDelta: "done"}, {Type: llm.EventMessageStop, StopReason: "end_turn"}}}, nil
}

type disclosureCaptureStream struct {
	grpc.ServerStream
	disclosures chan string
}

func (s *disclosureCaptureStream) SendMsg(m any) error {
	if msg, ok := m.(*proto.WorkerToHost); ok && msg.GetPermRequest() != nil {
		s.disclosures <- msg.GetPermRequest().GetDisclosure()
	}
	return s.ServerStream.SendMsg(m)
}

// Real host runner + production worker stack over gRPC, with only inference scripted.
func TestWorkerPlanningSecurityRoundtrip(t *testing.T) {
	for _, phase := range []string{"control", "inference", "approval", "read-error", "experiment-allow", "experiment-deny", "experiment-cancel"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "executed")
			var profile atomic.Value
			profile.Store(agent.Profile{Name: "developer"})
			p := &planningSecurityProvider{name: "RunCommand"}
			p.args, _ = json.Marshal(map[string]any{"cmd": []string{"/usr/bin/touch", marker}})
			experiment := strings.HasPrefix(phase, "experiment")
			if experiment {
				profile.Store(agent.PlanProfile())
				p.name = "plan_experiment"
				p.args = json.RawMessage(`{"package":".","test":"TestSafe"}`)
				for name, body := range map[string]string{"go.mod": "module safe.fixture\n\ngo 1.23\n", "safe_test.go": `package fixture
import("os";"testing")
func TestSafe(t *testing.T){ if os.Getenv("GOFLAGS")!="" || os.Getenv("GOENV")!="off" { t.Fatal("wrong environment") }; if err:=os.WriteFile("executed",[]byte("safe"),0600);err!=nil {t.Fatal(err)} }
`} {
					if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if phase == "inference" {
				p.duringInference = func() { profile.Store(agent.PlanProfile()) }
			}
			ws := worker.NewWithFactories(func(*proto.StartTurn) (providers.Resolver, error) { return &echoResolver{prov: p}, nil }, nil)
			disclosures := make(chan string, 8)
			srv := grpc.NewServer(grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				return handler(srv, &disclosureCaptureStream{ServerStream: stream, disclosures: disclosures})
			}))
			lis := bufconn.Listen(1 << 20)
			proto.RegisterWorkerServer(srv, ws)
			go func() { _ = srv.Serve(lis) }()
			defer srv.Stop()
			store, err := agent.LoadPermissionStore(filepath.Join(t.TempDir(), "permissions.json"))
			if err != nil {
				t.Fatal(err)
			}
			broker := permissions.New(store, nil, nil)
			if !experiment {
				if err := broker.SetMode(agent.ModeStrict); err != nil {
					t.Fatal(err)
				}
			}
			r := worker.NewWorkerRunnerWithDial(&recordingHistory{}, cfgsvc.New("", config.Config{LocusMode: "open_only"}, secrets.NewMemory()), broker, worker.BufconnDial(lis))
			if phase != "read-error" {
				r.(worker.ProfileReaderSetter).SetProfileReader(func(string) agent.Profile { return profile.Load().(agent.Profile) })
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			asks := 0
			_, err = r.RunTurn(ctx, runner.Request{ConversationID: "planning-security", Input: "probe", WorkDir: dir, Gen: 1}, &sinkCollector{}, func(_ context.Context, _, name string, args json.RawMessage, _ llm.Permission, _ bool) (bool, error) {
				asks++
				if phase == "approval" {
					profile.Store(agent.PlanProfile())
				}
				if phase == "experiment-cancel" {
					cancel()
				}
				if experiment && (name != "plan_experiment" || string(args) != string(p.args)) {
					t.Errorf("changed approval: %s %s", name, args)
				}
				return phase != "experiment-deny", nil
			}, func(llm.Message) {})
			if phase != "read-error" && phase != "experiment-cancel" && err != nil {
				t.Fatal(err)
			}
			_, statErr := os.Stat(marker)
			ran := statErr == nil
			if ran != (phase == "experiment-allow" || phase == "control") {
				t.Fatalf("executed=%v err=%v", ran, err)
			}
			if experiment {
				if asks != 1 {
					t.Fatalf("asks=%d", asks)
				}
				select {
				case got := <-disclosures:
					if want := approvaldisclosure.ForTool(p.name, p.args, dir); got != want {
						t.Fatalf("disclosure=%q want=%q", got, want)
					}
				default:
					t.Fatal("no disclosure on worker wire")
				}
				if profile.Load().(agent.Profile).Name != "plan" {
					t.Fatal("experiment lifted planning")
				}
			}
			if phase == "inference" && asks != 0 {
				t.Fatalf("forbidden queued tool reached approval: %d", asks)
			}
		})
	}
}
