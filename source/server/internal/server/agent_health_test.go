package server

import (
	"context"
	"net"
	"os"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"cercano/source/server/pkg/proto"
)

// ---------------------------------------------------------------------------
// Unit-level lifecycle probe semantics. Fakes only — no live agent, models,
// or network.
// ---------------------------------------------------------------------------

func healthReasonsContains(t *testing.T, reasons []string, want string) {
	t.Helper()
	for _, r := range reasons {
		if r == want {
			return
		}
	}
	t.Fatalf("reasons %v missing %q", reasons, want)
}

// A zero-value server (no MarkStartupComplete wired) must stay honestly
// not-ready: readiness is never defaulted to true.
func TestGetAgentHealthNotReadyBeforeStartupComplete(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	resp, err := s.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetReady() {
		t.Fatal("fresh server reported ready before MarkStartupComplete")
	}
	healthReasonsContains(t, resp.GetNotReadyReasons(), HealthReasonStartupIncomplete)
	if resp.GetVersion() != "dev" {
		t.Fatalf("version = %q; want default \"dev\"", resp.GetVersion())
	}
	if resp.GetPid() != int32(os.Getpid()) {
		t.Fatalf("pid = %d; want observational own pid %d", resp.GetPid(), os.Getpid())
	}
}

// After the entrypoint-owned MarkStartupComplete (and only then), the probe
// reports ready. One-way and idempotent: BeginShutdown never flips it back.
func TestGetAgentHealthReadyAfterStartupComplete(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	s.SetBuildVersion("v9.8.7")
	s.MarkStartupComplete()
	s.MarkStartupComplete() // idempotent
	resp, err := s.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetReady() {
		t.Fatalf("ready = false after startup complete; reasons %v", resp.GetNotReadyReasons())
	}
	if resp.GetVersion() != "v9.8.7" {
		t.Fatalf("version = %q; want v9.8.7", resp.GetVersion())
	}
	if len(resp.GetNotReadyReasons()) != 0 {
		t.Fatalf("ready server reported reasons: %v", resp.GetNotReadyReasons())
	}
}

// While update preparation holds the admission pause, health must report the
// pause — and must never mutate or release it (observational only).
func TestGetAgentHealthObservesAdmissionPause(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	s.MarkStartupComplete()
	release, err := s.updateWork.pauseWhenIdle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetReady() {
		t.Fatal("ready reported while update admission is paused")
	}
	healthReasonsContains(t, resp.GetNotReadyReasons(), HealthReasonAdmissionPaused)
	if !admissionPausedSnapshot(&s.updateWork) {
		t.Fatal("health probe released the admission pause")
	}
	// The probe left no work count behind, so admission stays pauseable.
	release()
	if admissionPausedSnapshot(&s.updateWork) {
		t.Fatal("release did not un-pause admission")
	}
}

// BeginShutdown flips the lifecycle flag health reports; the process is
// heading to its normal stop path and must not read as usable.
func TestGetAgentHealthReportsShutdownBegun(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	s.MarkStartupComplete()
	s.BeginShutdown()
	resp, err := s.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetReady() {
		t.Fatal("ready reported after BeginShutdown")
	}
	healthReasonsContains(t, resp.GetNotReadyReasons(), HealthReasonShutdownBegun)
}

// All three conditions at once produce all three reasons (and not-ready).
func TestGetAgentHealthCollectsAllReasons(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	s.BeginShutdown()
	if _, err := s.updateWork.pauseWhenIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	resp, err := s.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetReady() {
		t.Fatal("ready reported with all lifecycle gates failing")
	}
	if len(resp.GetNotReadyReasons()) != 3 {
		t.Fatalf("want 3 reasons, got %v", resp.GetNotReadyReasons())
	}
}

// ---------------------------------------------------------------------------
// Wire-level integration over bufconn: admission exemption and old-server
// Unimplemented behavior.
// ---------------------------------------------------------------------------

// startHealthTestServer runs a bufconn gRPC server like main wires it (same
// interceptors) and returns a connected client.
func startHealthTestServer(t *testing.T, srv *Server) proto.AgentClient {
	t.Helper()
	l := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(
		grpc.ChainUnaryInterceptor(srv.UpdateAdmissionUnaryInterceptor()),
		grpc.ChainStreamInterceptor(srv.UpdateAdmissionStreamInterceptor()),
	)
	proto.RegisterAgentServer(gs, srv)
	go gs.Serve(l)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return l.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial bufnet: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
		gs.Stop()
	})
	return proto.NewAgentClient(conn)
}

// GetAgentHealth must be servable over the wire WHILE update preparation holds
// the admission pause — proof it is exempt from work admission — and the call
// must leave the pause (and zero work count) untouched.
func TestGetAgentHealthServableWhileAdmissionPausedOverWire(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	s.MarkStartupComplete()
	release, err := s.updateWork.pauseWhenIdle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	client := startHealthTestServer(t, s)
	resp, err := client.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err != nil {
		t.Fatalf("health RPC refused while admission paused (not exempt?): %v", err)
	}
	if resp.GetReady() {
		t.Fatal("ready reported while admission paused")
	}
	healthReasonsContains(t, resp.GetNotReadyReasons(), HealthReasonAdmissionPaused)
	if got := updateAdmissionActive(s); got != 0 {
		t.Fatalf("health call pinned the work count: %d", got)
	}
	if !admissionPausedSnapshot(&s.updateWork) {
		t.Fatal("health call disturbed the admission pause")
	}
}

// A healthy server answers ready over the wire with its version.
func TestGetAgentHealthReadyOverWire(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, nil)
	s.SetBuildVersion("v1.2.3-test")
	s.MarkStartupComplete()
	client := startHealthTestServer(t, s)
	resp, err := client.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetReady() {
		t.Fatalf("ready = false; reasons %v", resp.GetNotReadyReasons())
	}
	if resp.GetVersion() != "v1.2.3-test" {
		t.Fatalf("version = %q; want v1.2.3-test", resp.GetVersion())
	}
	if resp.GetPid() != int32(os.Getpid()) {
		t.Fatalf("pid = %d; want %d", resp.GetPid(), os.Getpid())
	}
}

// An OLD server (a binary built before GetAgentHealth existed, represented by
// the generated unimplemented base the old builds embedded) must surface
// codes.Unimplemented — callers fail closed and treat the agent as unknown,
// never as ready.
func TestGetAgentHealthOldServerUnimplemented(t *testing.T) {
	l := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	proto.RegisterAgentServer(gs, &proto.UnimplementedAgentServer{})
	go gs.Serve(l)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return l.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial bufnet: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
		gs.Stop()
	})

	client := proto.NewAgentClient(conn)
	resp, err := client.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
	if err == nil {
		t.Fatalf("old server answered: %+v; want Unimplemented", resp)
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("status = %v; want Unimplemented", err)
	}
}

func TestGetAgentHealthCredentialPolicy(t *testing.T) {
	for _, value := range []string{"0", "1", "", "malformed"} {
		t.Run("policy_"+value, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", value)
			s := &Server{}
			response, err := s.GetAgentHealth(context.Background(), &proto.GetAgentHealthRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if response.GetCredentialsDisabled() != (value != "0") {
				t.Fatalf("incorrect explicit policy: %+v", response)
			}
		})
	}
}
