package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"sync/atomic"
	"testing"
)

func TestIntentionalQuitIsDistinctFromDisconnect(t *testing.T) {
	m := New(nil, false)
	if marked, ok := any(m).(interface{ IntentionalExit() bool }); ok && marked.IntentionalExit() {
		t.Fatal("new/disconnected model must retain ownership")
	}
	m.ctrlCArmed = true
	next, cmd := m.handleCtrlCKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected quit")
	}
	marked, ok := any(next).(interface{ IntentionalExit() bool })
	if !ok || !marked.IntentionalExit() {
		t.Fatal("intentional quit is indistinguishable from unexpected disconnect; ownership remains retained")
	}
}

func TestIntentionalSlashQuitAndExit(t *testing.T) {
	for _, command := range []string{"/quit", "/exit"} {
		t.Run(command, func(t *testing.T) {
			m := New(nil, false)
			m.devHost = &devHostState{}
			next, cmd := m.runSlash(command)
			if cmd == nil {
				t.Fatal("quit refused")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("expected quit")
			}
			if !next.(Model).IntentionalExit() {
				t.Fatal("missing explicit exit intent")
			}
		})
	}
}

// Loopback-only synthetic RPC peer: no launcher, credentials, or runtime state.
type closeSessionPeer struct {
	proto.UnimplementedAgentServer
	releases atomic.Int32
	refuse   bool
}

func (*closeSessionPeer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
func (*closeSessionPeer) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: "test", Build: "build", Contract: "contract", SchemaVersion: 2}, nil
}
func (*closeSessionPeer) CheckRetainedSession(context.Context, *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	return &proto.CheckRetainedSessionResponse{State: proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
}
func (p *closeSessionPeer) LifecycleRelease(context.Context, *proto.LifecycleControlRequest) (*proto.LifecycleControlResponse, error) {
	p.releases.Add(1)
	if p.refuse {
		return nil, status.Error(codes.FailedPrecondition, "synthetic refusal")
	}
	return &proto.LifecycleControlResponse{}, nil
}
func sessionPeerClient(t *testing.T, peer *closeSessionPeer) *agentclient.Client {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, peer)
	go s.Serve(l)
	t.Cleanup(s.Stop)
	c, err := agentclient.DialPinnedSession(context.Background(), agentclient.PinnedSessionIntent{Endpoint: l.Addr().String(), HostInstance: "test", Build: "build", Contract: "contract", SchemaVersion: 2, ConversationID: "synthetic", OwnershipEpoch: "epoch"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func TestCloseSessionFinalClientDetachedAndFailure(t *testing.T) {
	initial, replacement := &closeSessionPeer{}, &closeSessionPeer{refuse: true}
	m := New(sessionPeerClient(t, initial), false)
	m.intentionalExit = true
	m.devHost = &devHostState{selection: DevHostSelection{Identity: &proto.LifecycleIdentity{Host: "test"}}}
	// Reopen/reconnect replaces the model client after startup.
	m.agent = sessionPeerClient(t, replacement)
	m.devHost.detached = true
	if err := m.CloseSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if replacement.releases.Load() != 0 {
		t.Fatal("detached exit released ownership")
	}
	m.devHost.detached = false
	if err := m.CloseSession(context.Background()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("lost release failure: %v", err)
	}
	if initial.releases.Load() != 0 || replacement.releases.Load() != 1 {
		t.Fatal("release did not use final model client")
	}
}
