package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"
)

// useDialProfile replaces how commands reach a profile's agent and records
// the profiles they asked for.
func useDialProfile(t *testing.T, fn func(context.Context, string) (*agentclient.Client, error)) *[]string {
	t.Helper()
	var dialed []string
	prev := dialProfile
	dialProfile = func(ctx context.Context, profile string) (*agentclient.Client, error) {
		dialed = append(dialed, profile)
		return fn(ctx, profile)
	}
	t.Cleanup(func() { dialProfile = prev })
	return &dialed
}

func failDial(context.Context, string) (*agentclient.Client, error) {
	return nil, errors.New("fake dial failure")
}

func TestMCPConnectsToProfileHost(t *testing.T) {
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	dialed := useDialProfile(t, failDial)

	if _, _, err := connectMCPAgent(context.Background()); err == nil || !strings.Contains(err.Error(), "fake dial failure") {
		t.Fatalf("connectMCPAgent err = %v, want the dial error (for the degraded server)", err)
	}
	if len(*dialed) != 1 || (*dialed)[0] != "work" {
		t.Fatalf("dialed %v, want [work]", *dialed)
	}

	// MCP mode never starts a host of its own.
	for _, fn := range []string{"runMCPMode", "connectMCPAgent"} {
		if calls := callsIn(t, "main.go", fn, "startGRPCServer"); calls != 0 {
			t.Errorf("%s calls startGRPCServer %d times, want 0", fn, calls)
		}
	}
}

// subscribeHost is a registered profile agent that counts event subscribers.
type subscribeHost struct {
	proto.UnimplementedAgentServer
	profile, instance string
	subscribers       atomic.Int32
}

func (s *subscribeHost) GetHostIdentity(context.Context, *proto.GetHostIdentityRequest) (*proto.HostIdentity, error) {
	return &proto.HostIdentity{Profile: s.profile, InstanceId: s.instance, Pid: int32(os.Getpid())}, nil
}

func (s *subscribeHost) SubscribeEvents(_ *proto.SubscribeEventsRequest, stream proto.Agent_SubscribeEventsServer) error {
	s.subscribers.Add(1)
	defer s.subscribers.Add(-1)
	<-stream.Context().Done()
	return nil
}

// MCP stays subscribed to the agent's events, so an auto-launched agent
// counts it as a client and does not shut down under it.
func TestMCPHoldsAgentEventSubscription(t *testing.T) {
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	host := &subscribeHost{profile: "work", instance: "mcp-test"}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, host)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	if err := hostregistry.Write(paths.ProfileDir("work"), hostregistry.Record{
		Profile: "work", PID: os.Getpid(), Addr: lis.Addr().String(), InstanceID: "mcp-test", StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	useDialProfile(t, agentclient.DialProfile)

	agent, closeAgent, err := connectMCPAgent(context.Background())
	if err != nil {
		t.Fatalf("connectMCPAgent: %v", err)
	}
	id, err := agent.GetHostIdentity(context.Background(), &proto.GetHostIdentityRequest{})
	if err != nil || id.GetInstanceId() != "mcp-test" {
		t.Fatalf("MCP agent client reached %v (%v), want the work agent", id, err)
	}
	waitFor(t, func() bool { return host.subscribers.Load() == 1 }, "MCP to subscribe to agent events")
	closeAgent()
	waitFor(t, func() bool { return host.subscribers.Load() == 0 }, "the subscription to end when MCP closes")
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// errDialed stops runHeadless at its dial so the test can inspect it.
var errDialed = errors.New("dialed")

func TestRunDialsActiveProfile(t *testing.T) {
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	t.Cleanup(func() { signal.Reset(os.Interrupt) })
	dialed := useDialProfile(t, func(context.Context, string) (*agentclient.Client, error) { panic(errDialed) })

	func() {
		defer func() {
			if r := recover(); r != errDialed {
				t.Fatalf("runHeadless did not dial through dialProfile (recovered %v)", r)
			}
		}()
		runHeadless([]string{"hello"})
	}()
	if len(*dialed) != 1 || (*dialed)[0] != "work" {
		t.Fatalf("dialed %v, want [work]", *dialed)
	}
}

func TestEnterpriseDialsActiveProfileWithoutAddress(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("enterprise commands run on macOS only")
	}
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	dialed := useDialProfile(t, failDial)

	err := enterpriseCommand(context.Background(), []string{"status"}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "fake dial failure") {
		t.Fatalf("enterprise status err = %v, want the dial error", err)
	}
	if len(*dialed) != 1 || (*dialed)[0] != "work" {
		t.Fatalf("dialed %v, want [work]", *dialed)
	}

	*dialed = nil
	_ = enterpriseCommand(context.Background(), []string{"status", "--address", refusedAddr(t)}, &strings.Builder{})
	if len(*dialed) != 0 {
		t.Fatalf("--address still dialed the profile: %v", *dialed)
	}
}

// callsIn counts calls to callee inside function fn of file.
func callsIn(t *testing.T, file, fn, callee string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found, calls := false, 0
	for _, decl := range f.Decls {
		d, ok := decl.(*ast.FuncDecl)
		if !ok || d.Name.Name != fn || d.Recv != nil {
			continue
		}
		found = true
		ast.Inspect(d.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == callee {
					calls++
				}
			}
			return true
		})
	}
	if !found {
		t.Fatalf("%s not found in %s", fn, file)
	}
	return calls
}
