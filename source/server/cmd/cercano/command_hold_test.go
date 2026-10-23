package main

import (
	"bytes"
	"context"
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

// commandHost is a profile agent that serves one chat turn and enterprise
// status, recording how many event subscribers were open while it did.
type commandHost struct {
	*subscribeHost
	proto.UnimplementedEnterpriseServer
	during atomic.Int32
	served atomic.Bool
}

// recordSubscribers waits briefly for a subscriber, as the command opens its
// stream concurrently with its request, then records the count.
func (h *commandHost) recordSubscribers() {
	deadline := time.Now().Add(3 * time.Second)
	for h.subscribers.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	h.during.Store(h.subscribers.Load())
	h.served.Store(true)
}

func (h *commandHost) StreamProcessRequest(*proto.ProcessRequestRequest, grpc.ServerStreamingServer[proto.StreamProcessResponse]) error {
	h.recordSubscribers()
	return nil
}

func (h *commandHost) GetStatus(context.Context, *proto.EnterpriseControlRequest) (*proto.EnterpriseStatus, error) {
	h.recordSubscribers()
	return &proto.EnterpriseStatus{}, nil
}

func serveCommandHost(t *testing.T, profile string) *commandHost {
	t.Helper()
	host := &commandHost{subscribeHost: &subscribeHost{profile: profile, instance: profile + "-cmd"}}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, host)
	proto.RegisterEnterpriseServer(s, host)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	if err := hostregistry.Write(paths.ProfileDir(profile), hostregistry.Record{
		Profile: profile, PID: os.Getpid(), Addr: lis.Addr().String(), InstanceID: host.instance, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	return host
}

// `cercano run` holds the agent's event stream while it works, so an agent it
// auto-launched counts it as a client and idle-exits once it is done.
func TestRunHoldsAgentEventSubscription(t *testing.T) {
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	t.Cleanup(func() { signal.Reset(os.Interrupt) })
	host := serveCommandHost(t, "work")
	useDialProfile(t, agentclient.DialProfile)

	runHeadlessCommand([]string{"hello"})
	if !host.served.Load() {
		t.Fatal("run never reached the agent")
	}
	if got := host.during.Load(); got != 1 {
		t.Fatalf("event subscribers during the turn = %d, want 1", got)
	}
	waitFor(t, func() bool { return host.subscribers.Load() == 0 }, "run to release its event subscription")
}

func TestEnterpriseHoldsAgentEventSubscription(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("enterprise commands run on macOS only")
	}
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	host := serveCommandHost(t, "work")
	useDialProfile(t, agentclient.DialProfile)
	var notices bytes.Buffer
	prev := enterpriseNotices
	enterpriseNotices = &notices
	t.Cleanup(func() { enterpriseNotices = prev })

	var out strings.Builder
	if err := enterpriseCommand(context.Background(), []string{"status", "--json"}, &out); err != nil {
		t.Fatalf("enterprise status: %v", err)
	}
	if got := host.during.Load(); got != 1 {
		t.Fatalf("event subscribers during the request = %d, want 1", got)
	}
	waitFor(t, func() bool { return host.subscribers.Load() == 0 }, "enterprise to release its event subscription")
	if strings.Contains(out.String(), "cercano:") {
		t.Errorf("connection notices on stdout: %q", out.String())
	}
	if !strings.Contains(notices.String(), `profile "work"`) {
		t.Errorf("connection notice not shown: %q", notices.String())
	}
}
