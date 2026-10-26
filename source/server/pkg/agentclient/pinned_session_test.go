package agentclient

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type pinnedFake struct {
	proto.UnimplementedAgentServer
	state                        atomic.Int32
	identities, checks, requests atomic.Int32
	host                         string
	mu                           sync.Mutex
	terminalIDs                  []string
}

func (f *pinnedFake) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	f.identities.Add(1)
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: f.host, Build: "build", Contract: "contract", SchemaVersion: 2}, nil
}
func (f *pinnedFake) CheckRetainedSession(ctx context.Context, r *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	f.checks.Add(1)
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get(PinnedTerminalMetadata)
	if len(ids) != 1 || ids[0] == "" {
		return nil, errors.New("missing terminal identity")
	}
	f.mu.Lock()
	f.terminalIDs = append(f.terminalIDs, ids[0])
	f.mu.Unlock()
	if r.HostInstance != "A" || r.ConversationId != "conversation" || r.OwnershipEpoch != "epoch" || len(md.Get(PinnedEpochMetadata)) != 1 || md.Get(PinnedEpochMetadata)[0] != "epoch" {
		return nil, errors.New("wrong binding")
	}
	return &proto.CheckRetainedSessionResponse{State: proto.RetainedSessionState(f.state.Load())}, nil
}
func (f *pinnedFake) ResetSetup(context.Context, *proto.ResetSetupRequest) (*proto.ResetSetupResponse, error) {
	f.requests.Add(1)
	return &proto.ResetSetupResponse{}, nil
}
func pinnedHost(t *testing.T, addr string, f *pinnedFake) (*grpc.Server, string) {
	t.Helper()
	l, e := net.Listen("tcp", addr)
	if e != nil {
		t.Fatal(e)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, f)
	go s.Serve(l)
	t.Cleanup(s.Stop)
	return s, l.Addr().String()
}
func pinAt(addr string) PinnedSessionIntent {
	return PinnedSessionIntent{Endpoint: addr, HostInstance: "A", Build: "build", Contract: "contract", SchemaVersion: 2, ConversationID: "conversation", OwnershipEpoch: "epoch"}
}
func forbidLaunch(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	old := launchServerProcess
	launchServerProcess = func(string) (string, error) { calls.Add(1); return "", errors.New("launch forbidden") }
	t.Cleanup(func() {
		launchServerProcess = old
		if calls.Load() != 0 {
			t.Errorf("launch called %d times", calls.Load())
		}
	})
	return &calls
}
func TestPinnedNoLaunchRefusals(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	forbidLaunch(t)
	for _, state := range []proto.RetainedSessionState{proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE, proto.RetainedSessionState_RETAINED_SESSION_STATE_RELEASED, proto.RetainedSessionState_RETAINED_SESSION_STATE_RETIRED, proto.RetainedSessionState_RETAINED_SESSION_STATE_CLOSED, proto.RetainedSessionState_RETAINED_SESSION_STATE_UNSPECIFIED} {
		t.Run(state.String(), func(t *testing.T) {
			f := &pinnedFake{host: "A"}
			f.state.Store(int32(state))
			_, addr := pinnedHost(t, "127.0.0.1:0", f)
			_, err := DialPinnedSession(context.Background(), pinAt(addr))
			var refusal *SessionCheckError
			if !errors.As(err, &refusal) || refusal.State != state {
				t.Fatalf("wrong refusal: %v", err)
			}
			f.state.Store(1)
			c, err := DialPinnedSession(context.Background(), pinAt(addr))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			f.state.Store(int32(state))
			if err = c.reconnect(context.Background()); !errors.As(err, &refusal) {
				t.Fatalf("retry: %v", err)
			}
		})
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := DialPinnedSession(ctx, pinAt(addr)); err == nil {
		t.Fatal("unreachable accepted")
	}
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	s, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.Stop()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if err := c.reconnect(ctx2); err == nil {
		t.Fatal("unreachable retry accepted")
	}
}
func TestPinnedTransportAndFreshValidation(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
	forbidLaunch(t)
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	s, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.ReconnectPinnedSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.identities.Load() != 2 || f.checks.Load() != 2 {
		t.Fatal("explicit ready reconnect did not revalidate")
	}
	old := c.readConn()
	s.Stop()
	replacement := &pinnedFake{host: "B"}
	replacement.state.Store(1)
	_, _ = pinnedHost(t, addr, replacement)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	err = old.Invoke(ctx, proto.Agent_ResetSetup_FullMethodName, &proto.ResetSetupRequest{}, &proto.ResetSetupResponse{}, grpc.WaitForReady(true))
	if err == nil || replacement.requests.Load() != 0 || replacement.identities.Load() != 0 {
		t.Fatal("unvalidated replacement reached")
	}
	if err := c.ReconnectPinnedSession(context.Background()); !errors.Is(err, ErrPinnedSession) {
		t.Fatalf("replacement: %v", err)
	}
	if replacement.checks.Load() != 0 {
		t.Fatal("session checked on wrong identity")
	}
}

func TestPinnedTerminalIdentityLifetime(t *testing.T) {
	forbidLaunch(t)
	f := &pinnedFake{host: "A"}
	f.state.Store(1)
	_, addr := pinnedHost(t, "127.0.0.1:0", f)
	intent := pinAt(addr)
	c, err := DialPinnedSession(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.ReconnectPinnedSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	d, err := DialPinnedSession(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := f.terminalIDs
	if len(ids) != 3 || ids[0] != ids[1] || ids[0] == ids[2] {
		t.Fatalf("terminal identity lifetime: %v", ids)
	}
}

func (*pinnedFake) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
