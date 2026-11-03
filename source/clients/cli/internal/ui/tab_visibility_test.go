package ui

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type blockedDismissServer struct {
	proto.UnimplementedAgentServer
	entered, release chan struct{}
}

func (s *blockedDismissServer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
func (s *blockedDismissServer) DismissSubAgent(ctx context.Context, _ *proto.DismissSubAgentRequest) (*proto.DismissSubAgentResponse, error) {
	close(s.entered)
	select {
	case <-s.release:
		return &proto.DismissSubAgentResponse{}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestCloseWaitsForPersistedDismissal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &blockedDismissServer{entered: make(chan struct{}), release: make(chan struct{})}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, service)
	go server.Serve(listener)
	defer server.Stop()
	client, err := agentclient.DialExisting(t.Context(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	m := New(client, false)
	m.ensureSubAgentTab("child", "", "", nil)
	done := make(chan bool, 1)
	go func() { done <- m.closeSubAgentTab("child") }()
	<-service.entered
	select {
	case <-done:
		close(service.release)
		t.Fatal("close returned before persisted dismissal; immediate quit/resume can resurrect child")
	case <-time.After(30 * time.Millisecond):
	}
	close(service.release)
	if !<-done {
		t.Fatal("close failed")
	}
}
func TestLateHistoryReopenDoesNotUndoClose(t *testing.T) {
	m := New(nil, false)
	m.ensureSubAgentTab("child", "", "", nil)
	m.closeSubAgentTab("child")
	m.applySubAgentReopen(subAgentReopenMsg{id: "child"})
	if m.chatTabs.tabs["child"] != nil {
		t.Fatal("late history reopen resurrected closed child")
	}
}

// File-backed transport fixture exercises real CLI commands across fresh model
// lifetimes. The server package independently tests the same contract on SQLite.
type visibilityServer struct {
	persistedDevResumeServer
	mu           sync.Mutex
	statePath    string
	fail         bool
	loseResponse bool
	unsupported  bool
}

func (s *visibilityServer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
func (s *visibilityServer) ListSubAgents(context.Context, *proto.ListSubAgentsRequest) (*proto.ListSubAgentsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.statePath)
	if err != nil {
		return nil, err
	}
	var dismissed bool
	if err := json.Unmarshal(raw, &dismissed); err != nil {
		return nil, err
	}
	out := &proto.ListSubAgentsResponse{Subagents: []*proto.SubAgentConversation{{Id: "active", TaskId: "live-task", TaskStatus: "running"}, {Id: "finished", TaskId: "finished-task", TaskStatus: "completed"}}}
	if !dismissed {
		out.Subagents = append(out.Subagents, &proto.SubAgentConversation{Id: "child", TaskId: "child-task", TaskStatus: "completed"})
	}
	return out, nil
}
func (s *visibilityServer) DismissSubAgent(context.Context, *proto.DismissSubAgentRequest) (*proto.DismissSubAgentResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unsupported {
		return nil, status.Error(codes.Unimplemented, "older host")
	}
	if s.fail {
		return nil, status.Error(codes.Canceled, "injected write cancellation")
	}
	err := os.WriteFile(s.statePath, []byte("true"), 0600)
	if s.loseResponse {
		return nil, status.Error(codes.Canceled, "response lost after commit")
	}
	return &proto.DismissSubAgentResponse{}, err
}
func (s *visibilityServer) ReopenSubAgent(context.Context, *proto.ReopenSubAgentRequest) (*proto.ReopenSubAgentResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unsupported {
		return nil, status.Error(codes.Unimplemented, "older host")
	}
	if s.fail {
		return nil, status.Error(codes.Canceled, "injected write cancellation")
	}
	err := os.WriteFile(s.statePath, []byte("false"), 0600)
	if s.loseResponse {
		return nil, status.Error(codes.Canceled, "response lost after commit")
	}
	return &proto.ReopenSubAgentResponse{}, err
}
func TestTabVisibilityAcrossResumeEntryPoints(t *testing.T) {
	for _, entry := range []string{"picker", "-r", "/resume", "pinned-replacement"} {
		t.Run(entry, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("XDG_CONFIG_HOME", root)
			t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
			fixture := &visibilityServer{persistedDevResumeServer: persistedDevResumeServer{metadataPath: filepath.Join(root, "metadata.json")}, statePath: filepath.Join(root, "dismissed.json")}
			if err := os.WriteFile(fixture.metadataPath, []byte(`{"id":"saved"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixture.statePath, []byte("false"), 0600); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			proto.RegisterAgentServer(server, fixture)
			go server.Serve(listener)
			t.Cleanup(server.Stop)
			dial := func() *agentclient.Client {
				t.Helper()
				c, err := agentclient.DialExisting(t.Context(), listener.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { c.Close() })
				return c
			}
			resume := func() Model {
				c := dial()
				m := New(c, entry == "-r")
				var command tea.Cmd
				if entry == "pinned-replacement" {
					old := New(nil, false).WithDevHost(DevHostSelection{Conversation: "saved"}, "test")
					next, _ := old.applyDevHost(devHostResult{action: "reopen", client: c})
					m = next.(Model)
					batch := m.Init()().(tea.BatchMsg)
					msg := batch[len(batch)-1]().(devHostHydrateMsg)
					next, cmd := m.Update(msg)
					m = next.(Model)
					command = cmd
				} else if entry == "/resume" {
					next, cmd := m.runSlash("/resume saved")
					m = next.(Model)
					command = cmd
				} else {
					picker := newTestHistoryView([]histRow{{id: "saved"}}, 80, 24)
					cmd, _ := picker.Update(keyPress("enter"))
					next, resumeCmd := m.Update(cmd())
					m = next.(Model)
					command = resumeCmd
				}
				var consume func(tea.Msg)
				consume = func(msg tea.Msg) {
					switch msg := msg.(type) {
					case tea.BatchMsg:
						for _, cmd := range msg {
							if cmd != nil {
								consume(cmd())
							}
						}
					case resumeMetadataMsg:
						next, _ := m.Update(msg)
						m = next.(Model)
					case resumeViewportStreamMsg:
						m, _ = m.applyProgressiveResumeEvent(msg)
						if !msg.done && msg.ch != nil {
							consume(waitProgressiveResumeCmd(msg.ch)())
						}
					}
				}
				consume(command())
				return m
			}
			assertTabs := func(m Model, child bool) {
				t.Helper()
				if (m.chatTabs.tabs["child"] != nil) != child || m.chatTabs.tabs["active"] == nil || m.chatTabs.tabs["active"].done || m.chatTabs.tabs["finished"] == nil || !m.chatTabs.tabs["finished"].done {
					t.Fatalf("wrong restored visibility child=%v tabs=%v", child, m.chatTabs.order)
				}
			}
			m := resume()
			assertTabs(m, true)
			fixture.mu.Lock()
			fixture.unsupported = true
			fixture.mu.Unlock()
			if m.closeSubAgentTab("child") || m.chatTabs.tabs["child"] == nil {
				t.Fatal("unsupported host changed visibility")
			}
			m.width = 100
			m.height = 30
			if !strings.Contains(m.View().Content, "rejected") {
				t.Fatal("unsupported failure invisible")
			}
			fixture.mu.Lock()
			fixture.unsupported = false
			fixture.mu.Unlock()

			fixture.mu.Lock()
			fixture.fail = true
			fixture.mu.Unlock()
			if m.closeSubAgentTab("child") || m.chatTabs.tabs["child"] == nil {
				t.Fatal("canceled write falsely closed tab")
			}
			fixture.mu.Lock()
			fixture.fail = false
			fixture.mu.Unlock()
			if !m.closeSubAgentTab("child") {
				t.Fatal("close failed")
			}
			m.agent.Close()
			m = resume()
			assertTabs(m, false)
			fixture.mu.Lock()
			fixture.fail = true
			fixture.mu.Unlock()
			if cmd := m.reopenSubAgentTabCmd("child"); cmd != nil || m.chatTabs.tabs["child"] != nil {
				t.Fatal("canceled reopen falsely opened tab")
			}
			fixture.mu.Lock()
			fixture.fail = false
			fixture.mu.Unlock()
			fixture.mu.Lock()
			fixture.loseResponse = true
			fixture.mu.Unlock()
			if cmd := m.reopenSubAgentTabCmd("child"); cmd == nil {
				t.Fatal("committed reopen response loss not reconciled")
			}
			if !m.closeSubAgentTab("child") {
				t.Fatal("committed close response loss not reconciled")
			}
			fixture.mu.Lock()
			fixture.loseResponse = false
			fixture.mu.Unlock()
			// Finish old hydration after a close/new explicit reopen: identity fencing
			// must reject it without clearing the newer tab or its persisted state.
			oldCmd := m.reopenSubAgentTabCmd("child")
			oldMsg := oldCmd().(subAgentReopenMsg)
			if !m.closeSubAgentTab("child") {
				t.Fatal("close during hydration failed")
			}
			cmd := m.reopenSubAgentTabCmd("child")
			tab := m.chatTabs.tabs["child"]
			entriesBefore := len(tab.view.entries)
			m.applySubAgentReopen(oldMsg)
			if m.chatTabs.tabs["child"] != tab || len(tab.view.entries) != entriesBefore {
				t.Fatal("stale hydration overwrote new tab")
			}
			m.applySubAgentReopen(cmd().(subAgentReopenMsg))
			m.agent.Close()
			m = resume()
			assertTabs(m, true)
			if !m.closeSubAgentTab("child") {
				t.Fatal("second close failed")
			}
			m.agent.Close()
			assertTabs(resume(), false)
		})
	}
}

// A history reopen must write visibility before its transcript command runs.
func TestExplicitHistoryReopenWritesVisibility(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	fixture := &visibilityServer{statePath: filepath.Join(root, "dismissed.json")}
	if err := os.WriteFile(fixture.statePath, []byte("true"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, fixture)
	go server.Serve(listener)
	defer server.Stop()
	client, err := agentclient.DialExisting(t.Context(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	m := New(client, false)
	if cmd := m.reopenSubAgentTabCmd("child"); cmd == nil {
		t.Fatal("missing transcript hydration command")
	}
	// Intentionally do not run hydration: merely reading the transcript is not
	// what persists visibility, and exit may happen before hydration completes.
	client.Close()
	raw, err := os.ReadFile(fixture.statePath)
	if err != nil || string(raw) != "false" {
		t.Fatalf("explicit reopen was not persisted: dismissed=%s err=%v", raw, err)
	}
}

func TestTabFailureIsRendered(t *testing.T) {
	m := New(nil, false)
	m.width = 100
	m.height = 30
	m.errMsg = "tab reopen rejected: unsupported host"
	if !strings.Contains(m.View().Content, "tab reopen rejected") {
		t.Fatal("tab failure invisible")
	}
}

func TestTabCommittedResponseLoss(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	fixture := &visibilityServer{statePath: filepath.Join(root, "dismissed.json"), loseResponse: true}
	if err := os.WriteFile(fixture.statePath, []byte("true"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, fixture)
	go server.Serve(listener)
	defer server.Stop()
	client, err := agentclient.DialExisting(t.Context(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	m := New(client, false)
	if cmd := m.reopenSubAgentTabCmd("child"); cmd == nil {
		t.Fatal("missing transcript hydration command")
	}
	// Intentionally do not run hydration: merely reading the transcript is not
	// what persists visibility, and exit may happen before hydration completes.
	raw, err := os.ReadFile(fixture.statePath)
	if err != nil || string(raw) != "false" {
		t.Fatalf("explicit reopen was not persisted: dismissed=%s err=%v", raw, err)
	}

	if !m.closeSubAgentTab("child") {
		t.Fatal("committed close response loss not reconciled")
	}
	raw, err = os.ReadFile(fixture.statePath)
	if err != nil || string(raw) != "true" {
		t.Fatalf("close state=%s err=%v", raw, err)
	}
}
