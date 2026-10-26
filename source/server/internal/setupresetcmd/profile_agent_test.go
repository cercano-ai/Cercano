package setupresetcmd

import (
	"context"
	"errors"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/hostregistry"
	"cercano/source/server/pkg/paths"
	"cercano/source/server/pkg/proto"
)

type resetHost struct {
	proto.UnimplementedAgentServer
	profile, instance string
	resets            atomic.Int32
}

func (h *resetHost) GetHostIdentity(context.Context, *proto.GetHostIdentityRequest) (*proto.HostIdentity, error) {
	return &proto.HostIdentity{Profile: h.profile, InstanceId: h.instance, Pid: int32(os.Getpid())}, nil
}

func (h *resetHost) ResetSetup(context.Context, *proto.ResetSetupRequest) (*proto.ResetSetupResponse, error) {
	h.resets.Add(1)
	return &proto.ResetSetupResponse{Ok: true, ConfigWritten: true, LiveApplied: true, CredentialsRemoved: 2}, nil
}

func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CERCANO_PROFILE", "")
}

// registerHost serves h on loopback and records it as profile's agent.
func registerHost(t *testing.T, profile string, h *resetHost) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, h)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	if err := hostregistry.Write(paths.ProfileDir(profile), hostregistry.Record{
		Profile: h.profile, PID: os.Getpid(), Addr: lis.Addr().String(), InstanceID: h.instance, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

// Reset goes through the profile's own agent when it runs, and never starts
// one: with no agent the reset is done locally.
func TestTryProfileAgent(t *testing.T) {
	isolateHome(t)
	if _, err := TryProfileAgent("work")(context.Background()); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("no agent running: err = %v, want ErrNoAgent", err)
	}
	if entries, _ := os.ReadDir(paths.ProfileDir("work")); len(entries) != 0 {
		t.Fatalf("checking for the agent wrote %v (an agent may have been launched)", entries)
	}

	// Another profile's agent is never used.
	other := &resetHost{profile: "default", instance: "default-agent"}
	registerHost(t, "default", other)
	if _, err := TryProfileAgent("work")(context.Background()); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("only default running: err = %v, want ErrNoAgent", err)
	}

	work := &resetHost{profile: "work", instance: "work-agent"}
	registerHost(t, "work", work)
	outcome, err := TryProfileAgent("work")(context.Background())
	if err != nil {
		t.Fatalf("work agent running: %v", err)
	}
	if !outcome.UsedAgent || !outcome.LiveApplied || outcome.CredentialsRemoved != 2 {
		t.Errorf("outcome = %+v, want the agent's result", outcome)
	}
	if work.resets.Load() != 1 || other.resets.Load() != 0 {
		t.Errorf("resets: work=%d default=%d, want 1 and 0", work.resets.Load(), other.resets.Load())
	}
}
