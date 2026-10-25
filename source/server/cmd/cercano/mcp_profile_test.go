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

// With per-profile agent hosts on, MCP mode without --grpc-addr routes
// through the profile's shared agent host: it dials the active profile, and
// neither it nor connectMCPAgent ever starts a gRPC server of its own.
func TestMCPConnectsToProfileHost(t *testing.T) {
	tempHome(t)
	t.Setenv(paths.PerProfileAgentsEnv, "1")
	t.Setenv("CERCANO_PROFILE", "work")
	dialed := useDialProfile(t, failDial)

	if _, _, err := connectMCPAgent(context.Background()); err == nil || !strings.Contains(err.Error(), "fake dial failure") {
		t.Fatalf("connectMCPAgent err = %v, want the dial error (for the degraded server)", err)
	}
	if len(*dialed) != 1 || (*dialed)[0] != "work" {
		t.Fatalf("dialed %v, want [work]", *dialed)
	}

	// Flag-on MCP mode never starts a host of its own.
	for _, fn := range []string{"runMCPMode", "connectMCPAgent"} {
		if calls := callsIn(t, "main.go", fn, "startGRPCServer"); calls != 0 {
			t.Errorf("%s calls startGRPCServer %d times, want 0", fn, calls)
		}
	}
}

// The embedded-agent choice follows the flag: with per-profile agent hosts
// off (the default) only the default profile serves through the embedded
// in-process agent, keeping `cercano --mcp` on its pre-shared-host behavior;
// with the flag on, every profile — and with the flag off, every named
// profile — routes through a shared host instead.
func TestMCPEmbeddedAgentChoiceFollowsFlag(t *testing.T) {
	t.Setenv("CERCANO_PROFILE", "")
	t.Setenv(paths.PerProfileAgentsEnv, "")
	if !mcpUsesEmbeddedAgent(paths.DefaultProfile) {
		t.Errorf("mcpUsesEmbeddedAgent(default) with the flag off = false, want true")
	}
	for _, profile := range []string{"work", "other"} {
		if mcpUsesEmbeddedAgent(profile) {
			t.Errorf("mcpUsesEmbeddedAgent(%q) with the flag off = true, want false", profile)
		}
	}

	t.Setenv(paths.PerProfileAgentsEnv, "1")
	for _, profile := range []string{paths.DefaultProfile, "work"} {
		if mcpUsesEmbeddedAgent(profile) {
			t.Errorf("mcpUsesEmbeddedAgent(%q) with the flag on = true, want false", profile)
		}
	}
}

// The embedded branch is the only place MCP mode starts a gRPC server: it
// lives in startEmbeddedAgent (one start, on a loopback ephemeral port),
// while runMCPMode and connectMCPAgent start none themselves.
func TestMCPEmbeddedStartsOneGRPCServerInItsOwnFunction(t *testing.T) {
	if calls := specCallsIn(t, "main.go", "startEmbeddedAgent", "startGRPCServer"); calls != 1 {
		t.Fatalf("startEmbeddedAgent calls startGRPCServer %d times, want 1", calls)
	}
	if calls := callsIn(t, "main.go", "runMCPMode", "startGRPCServer"); calls != 0 {
		t.Errorf("runMCPMode calls startGRPCServer %d times, want 0", calls)
	}
}

// subscribeHost is a registered profile agent that counts event subscribers.
type subscribeHost struct {
	proto.UnimplementedAgentServer
	profile, instance string
	subscribers       atomic.Int32
	subscriptions     atomic.Int32 // ever opened
	// endFirst ends the first subscription at once, as a draining agent's
	// closeAll does.
	endFirst bool
}

func (s *subscribeHost) GetHostIdentity(context.Context, *proto.GetHostIdentityRequest) (*proto.HostIdentity, error) {
	return &proto.HostIdentity{Profile: s.profile, InstanceId: s.instance, Pid: int32(os.Getpid())}, nil
}

func (s *subscribeHost) SubscribeEvents(_ *proto.SubscribeEventsRequest, stream proto.Agent_SubscribeEventsServer) error {
	n := s.subscriptions.Add(1)
	if s.endFirst && n == 1 {
		return nil
	}
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

// When the agent ends MCP's event stream (a graceful restart, or a terminal
// stream error) MCP subscribes again, so the agent keeps counting it as a
// client.
func TestMCPResubscribesWhenEventStreamEnds(t *testing.T) {
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	host := &subscribeHost{profile: "work", instance: "mcp-test", endFirst: true}
	registerSubscribeHost(t, host)
	useDialProfile(t, agentclient.DialProfile)

	_, closeAgent, err := connectMCPAgent(context.Background())
	if err != nil {
		t.Fatalf("connectMCPAgent: %v", err)
	}
	waitFor(t, func() bool { return host.subscriptions.Load() >= 2 && host.subscribers.Load() == 1 }, "MCP to subscribe again after the stream ended")
	closeAgent()
	waitFor(t, func() bool { return host.subscribers.Load() == 0 }, "the subscription to end when MCP closes")
	ended := host.subscriptions.Load()
	time.Sleep(600 * time.Millisecond)
	if got := host.subscriptions.Load(); got != ended {
		t.Fatalf("MCP subscribed again after closing (%d -> %d)", ended, got)
	}
}

// registerSubscribeHost serves host on loopback and records it as its
// profile's agent.
func registerSubscribeHost(t *testing.T, host *subscribeHost) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, host)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	if err := hostregistry.Write(paths.ProfileDir(host.profile), hostregistry.Record{
		Profile: host.profile, PID: os.Getpid(), Addr: lis.Addr().String(), InstanceID: host.instance, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
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

// specCallsIn counts calls to callee inside the initializer of the package
// variable varName of file — for test seams defined as function values.
func specCallsIn(t *testing.T, file, varName, callee string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	calls := 0
	for _, decl := range f.Decls {
		g, ok := decl.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, spec := range g.Specs {
			v, ok := spec.(*ast.ValueSpec)
			if !ok || len(v.Names) != 1 || v.Names[0].Name != varName {
				continue
			}
			found = true
			ast.Inspect(v, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == callee {
						calls++
					}
				}
				return true
			})
		}
	}
	if !found {
		t.Fatalf("var %s not found in %s", varName, file)
	}
	return calls
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
