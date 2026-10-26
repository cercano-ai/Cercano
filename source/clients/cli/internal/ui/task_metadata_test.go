package ui

import (
	"context"
	"net"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc"
)

func TestTaskLifecycleHelper(t *testing.T) {
	for _, tc := range []struct {
		status       string
		done, failed bool
	}{
		{"accepted", false, false}, {"running", false, false}, {"completed", true, false}, {"failed", true, true}, {"stopped", true, false}, {"interrupted", true, false}, {"unknown", false, false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			tab := &chatTab{}
			if !tab.applyTaskLifecycle("task", tc.status) {
				t.Fatal("metadata not applied")
			}
			if tab.done != tc.done || tab.errored != tc.failed {
				t.Fatalf("done=%v error=%v", tab.done, tab.errored)
			}
			if tab.applyTaskLifecycle("task", tc.status) {
				t.Fatal("identical metadata should be a no-op")
			}
		})
	}
	tab := &chatTab{}
	if tab.applyTaskLifecycle("", "") || tab.done {
		t.Fatal("absent metadata changed legacy tab")
	}
}

func TestTaskTerminalStatusClearsStreaming(t *testing.T) {
	m := New(nil, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.applySubAgentEvent(subAgentEventMsg{id: "child", kind: "started"})
	m.applySubAgentEvent(subAgentEventMsg{id: "child", kind: "token", inner: chatAssistantDeltaMsg{token: "partial"}})
	tab := m.chatTabs.tabs["child"]
	tab.applyTaskLifecycle("task", "failed")
	if tab.view.streaming || (tab.view.lastAssistantEntry() != nil && tab.view.lastAssistantEntry().Streaming) {
		t.Fatal("terminal task retained streaming spinner")
	}
}

func TestFinishStaleSubAgentTabsSkipsTaskOwned(t *testing.T) {
	for _, notice := range []bool{false, true} {
		m := New(nil, false)
		m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m.streaming = true
		m.cancelStream = func() {}
		for _, id := range []string{"owned", "legacy"} {
			m.applySubAgentEvent(subAgentEventMsg{id: id, kind: "started"})
			m.applySubAgentEvent(subAgentEventMsg{id: id, kind: "token", inner: chatAssistantDeltaMsg{token: "partial"}})
		}
		owned := m.chatTabs.tabs["owned"]
		owned.applyTaskLifecycle("task", "running")
		m.cancelCurrentStreamWithNotice(notice)
		if owned.done || owned.errored || !owned.view.streaming {
			t.Fatalf("parent ending notice=%v changed owned child", notice)
		}
		if legacy := m.chatTabs.tabs["legacy"]; legacy != nil && !legacy.done {
			t.Fatalf("parent ending notice=%v left legacy child live", notice)
		}
	}
}

type taskResumeServer struct{ proto.UnimplementedAgentServer }

func (*taskResumeServer) ListSubAgents(context.Context, *proto.ListSubAgentsRequest) (*proto.ListSubAgentsResponse, error) {
	return &proto.ListSubAgentsResponse{Subagents: []*proto.SubAgentConversation{
		{Id: "running", ParentId: "owner", TaskId: "task-running", TaskStatus: "running"},
		{Id: "failed", ParentId: "owner", TaskId: "task-failed", TaskStatus: "failed"},
		{Id: "unknown", ParentId: "owner", TaskId: "task-unknown", TaskStatus: "future-status"},
		{Id: "legacy", ParentId: "owner"},
	}}, nil
}
func (*taskResumeServer) ResumeConversation(context.Context, *proto.ResumeConversationRequest) (*proto.ResumeConversationResponse, error) {
	return &proto.ResumeConversationResponse{}, nil
}
func (*taskResumeServer) DismissSubAgent(context.Context, *proto.DismissSubAgentRequest) (*proto.DismissSubAgentResponse, error) {
	return &proto.DismissSubAgentResponse{}, nil
}

func TestRestoreSubAgentTabsWithTaskMetadata(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, &taskResumeServer{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := agentclient.DialExisting(ctx, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	m := New(nil, false)
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.agent = client
	m.restoreSubAgentTabs(ctx, "owner")
	for _, id := range []string{"running", "unknown"} {
		tab := m.chatTabs.tabs[id]
		if tab == nil || tab.done || tab.taskID != "task-"+id {
			t.Fatalf("live task %s lost or marked terminal: %+v", id, tab)
		}
	}
	if tab := m.chatTabs.tabs["failed"]; tab == nil || !tab.done || !tab.errored {
		t.Fatalf("failed task not restored: %+v", tab)
	}
	if m.chatTabs.tabs["legacy"] != nil {
		t.Fatal("empty legacy transcript was not dismissed")
	}
}

// taskResumeServer uses only in-memory RPC fixtures, never host credentials.
func (*taskResumeServer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
