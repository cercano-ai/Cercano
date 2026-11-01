package main

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	cliui "cercano/source/clients/cli/internal/ui"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc"
)

// This peer measures RPC/transport lifetime, not production broker accounting.
type shutdownPeer struct {
	proto.UnimplementedAgentServer
	releases atomic.Int32
	streams  atomic.Int32
	started  chan struct{}
	ended    chan struct{}
}

func (*shutdownPeer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
func (*shutdownPeer) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: "test", Build: "build", Contract: "contract", SchemaVersion: 2}, nil
}
func (*shutdownPeer) CheckRetainedSession(context.Context, *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	return &proto.CheckRetainedSessionResponse{State: proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
}
func (p *shutdownPeer) LifecycleRelease(context.Context, *proto.LifecycleControlRequest) (*proto.LifecycleControlResponse, error) {
	p.releases.Add(1)
	return &proto.LifecycleControlResponse{}, nil
}
func (p *shutdownPeer) StreamProcessRequest(_ *proto.ProcessRequestRequest, s proto.Agent_StreamProcessRequestServer) error {
	p.streams.Add(1)
	close(p.started)
	<-s.Context().Done()
	p.streams.Add(-1)
	close(p.ended)
	return s.Context().Err()
}

func TestCtrlCProgramShutdown(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("CERCANO_UI_CONFIG", root+"/ui.yaml")
	t.Setenv("CERCANO_THEMES_DIR", root+"/themes")
	for _, busy := range []bool{false, true} {
		for _, confirmed := range []bool{false, true} {
			name := "idle"
			if busy {
				name = "busy"
			}
			if confirmed {
				name += "/confirmed"
			} else {
				name += "/unconfirmed-interruption"
			}
			t.Run(name, func(t *testing.T) {
				peer := &shutdownPeer{started: make(chan struct{}), ended: make(chan struct{})}
				l, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				s := grpc.NewServer()
				proto.RegisterAgentServer(s, peer)
				go s.Serve(l)
				defer s.Stop()
				dial := func() *agentclient.Client {
					c, e := agentclient.DialPinnedSession(context.Background(), agentclient.PinnedSessionIntent{Endpoint: l.Addr().String(), HostInstance: "test", Build: "build", Contract: "contract", SchemaVersion: 2, ConversationID: "synthetic", OwnershipEpoch: "epoch"})
					if e != nil {
						t.Fatal(e)
					}
					return c
				}
				c := dial()
				defer c.Close()
				m := cliui.New(c, false).WithDevHost(cliui.DevHostSelection{Conversation: "synthetic", Epoch: "epoch", Identity: &proto.LifecycleIdentity{Host: "test"}}, "test")
				r, w := io.Pipe()
				defer r.Close()
				defer w.Close()
				p := tea.NewProgram(m, tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
				type result struct {
					model tea.Model
					err   error
				}
				done := make(chan result, 1)
				go func() { f, e := p.Run(); done <- result{f, e} }()
				defer p.Kill()
				if busy {
					// Send through the real UI update path; do not call its cleanup methods.
					p.Send(tea.KeyPressMsg{Code: 'h', Text: "hello"})
					p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
					select {
					case <-peer.started:
					case <-time.After(5 * time.Second):
						t.Fatal("UI did not start stream")
					}
				}
				// Input bytes are decoded by Bubble Tea, rather than calling handleCtrlCKey.
				if _, err = w.Write([]byte{3}); err != nil {
					t.Fatal(err)
				}
				select {
				case got := <-done:
					t.Fatalf("first Ctrl-C quit: %v", got.err)
				case <-time.After(100 * time.Millisecond):
				}
				if peer.releases.Load() != 0 {
					t.Fatal("unconfirmed exit released")
				}
				if confirmed {
					if _, err = w.Write([]byte{3}); err != nil {
						t.Fatal(err)
					}
				} else {
					p.Kill()
				}
				var got result
				select {
				case got = <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("program did not stop")
				}
				if confirmed && got.err != nil {
					t.Fatal(got.err)
				}
				// Same post-Run hook as both production CLI entry paths.
				if got.err == nil {
					if err := closeIntentionalSession(got.model); err != nil {
						t.Fatal(err)
					}
				}
				want := int32(0)
				if confirmed {
					want = 1
				}
				if peer.releases.Load() != want {
					t.Fatalf("release calls=%d want %d", peer.releases.Load(), want)
				}
				t.Logf("after Run/hook: release=%d active RPC streams=%d", peer.releases.Load(), peer.streams.Load())
				// Production defers transport Close until after the intentional-close hook.
				if err := c.Close(); err != nil {
					t.Fatal(err)
				}
				if busy {
					select {
					case <-peer.ended:
					case <-time.After(5 * time.Second):
						t.Fatal("transport close left stream")
					}
				}
				reopened := dial()
				defer reopened.Close()
				t.Logf("after transport close/reopen: release=%d active RPC streams=%d", peer.releases.Load(), peer.streams.Load())
				if peer.streams.Load() != 0 {
					t.Fatal("phantom RPC stream")
				}
			})
		}
	}
}
