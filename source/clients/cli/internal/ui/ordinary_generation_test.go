package ui

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

func isolateOrdinaryUI(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(dir, "ui.yaml"))
	t.Setenv("CERCANO_THEMES_DIR", filepath.Join(dir, "themes"))
}

func TestOrdinaryGenerationSlashSurfaces(t *testing.T) {
	isolateOrdinaryUI(t)
	for _, mode := range []string{"ordinary", "fixture", "unpinned"} {
		for _, command := range []string{"/config", "/settings", "/theme", "/m", "/mcp", "/runtime", "/help"} {
			t.Run(mode+command, func(t *testing.T) {
				m := New(devResumeClient(t, filepath.Join(t.TempDir(), "metadata.json")), false)
				if mode != "unpinned" {
					m = m.WithDevHost(DevHostSelection{Conversation: "old", Fixture: mode == "fixture"}, "test")
				}
				next, _ := m.runSlash(command) // Only synthetic RPCs; returned asynchronous commands are not executed.
				m = next.(Model)
				if mode == "fixture" {
					if m.errMsg == "" || m.configSurface != nil {
						t.Fatal("fixture restriction lost")
					}
				} else if m.errMsg != "" || (command != "/help" && m.configSurface == nil) {
					t.Fatalf("ordinary command blocked: %s", m.errMsg)
				}
			})
		}
	}
}

type clearHost struct {
	proto.UnimplementedAgentServer
	identity *proto.LifecycleIdentity
	acquired string
	stale    bool
}

func (s *clearHost) LifecycleAcquire(_ context.Context, r *proto.LifecycleControlRequest) (*proto.LifecycleControlResponse, error) {
	s.acquired = r.Owner.Conversation
	return &proto.LifecycleControlResponse{Owner: &proto.LifecycleOwner{Conversation: s.acquired, Host: s.identity.Host, Epoch: "new-epoch"}}, nil
}
func (s *clearHost) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: s.identity.Host, Build: s.identity.Build, Contract: s.identity.Contract, SchemaVersion: s.identity.Schema}, nil
}
func (s *clearHost) CheckRetainedSession(_ context.Context, r *proto.CheckRetainedSessionRequest) (*proto.CheckRetainedSessionResponse, error) {
	if r.ConversationId == "old" && r.OwnershipEpoch == "old-epoch" {
		return &proto.CheckRetainedSessionResponse{State: proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
	}
	if r.ConversationId != s.acquired || r.OwnershipEpoch != "new-epoch" {
		return nil, errors.New("wrong rebind")
	}
	state := proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED
	if s.stale {
		state = proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE
	}
	return &proto.CheckRetainedSessionResponse{State: state}, nil
}

// Execute the actual /clear acquisition + authenticated pinned dial using only
// an ephemeral loopback service and private temporary token, never a live host.
func TestOrdinaryGenerationClearRebind(t *testing.T) {
	isolateOrdinaryUI(t)
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "stale"}[stale], func(t *testing.T) {
			dir := t.TempDir()
			addUnrelatedRecord(t, dir)
			token := filepath.Join(dir, "token")
			if err := devhostauth.Create(token); err != nil {
				t.Fatal(err)
			}
			auth, err := devhostauth.Load("host", token)
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			fake := &clearHost{identity: &proto.LifecycleIdentity{Host: "host", Build: "build", Contract: "contract", Schema: 2, Store: filepath.Join(dir, "conversations.db")}, stale: stale}
			server := grpc.NewServer(auth.ServerOptions()...)
			proto.RegisterAgentServer(server, fake)
			go server.Serve(listener)
			defer server.Stop()
			selection := DevHostSelection{Identity: fake.identity, Endpoint: listener.Addr().String(), TokenFile: token, Conversation: "old", Epoch: "old-epoch"}
			oldClient, err := selection.Dial(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer oldClient.Close()
			m := New(oldClient, false).WithDevHost(selection, "test").SeedAssistantMarkdown("preserve old")
			m.recap = "old recap"
			next, cmd := m.runSlash("/clear")
			m = next.(Model)
			if cmd == nil || m.convID != "old" || m.convRef.id != "old" {
				t.Fatal("clear bypassed acquisition")
			}
			result := cmd().(devHostResult)
			if result.client != nil {
				defer result.client.Close()
			}
			if fake.acquired == "" || fake.acquired == "old" {
				t.Fatal("fresh ID not acquired")
			}
			next, _ = m.Update(result)
			m = next.(Model)
			if stale {
				if result.err == nil || m.convID != "old" || m.recap != "old recap" || m.devHost.busy {
					t.Fatal("failed rebind damaged current session")
				}
				if m.agent != oldClient {
					t.Fatal("old client replaced")
				}
				if err := m.agent.ReconnectPinnedSession(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				if result.err != nil {
					t.Fatal(result.err)
				}
				if m.convID != fake.acquired || m.convRef.id != m.convID || m.devHost.selection.Epoch != "new-epoch" || m.recap != "" {
					t.Fatal("new owned session not rebound")
				}
			}
		})
	}
}

func TestOrdinaryGenerationClearBusyDetached(t *testing.T) {
	isolateOrdinaryUI(t)
	for _, detached := range []bool{false, true} {
		m := New(nil, false).WithDevHost(DevHostSelection{Conversation: "old"}, "test")
		m.devHost.detached, m.devHost.busy = detached, !detached
		next, cmd := m.runSlash("/clear")
		if cmd != nil || next.(Model).convID != "old" {
			t.Fatal("clear escaped lifecycle guard")
		}
	}
}
