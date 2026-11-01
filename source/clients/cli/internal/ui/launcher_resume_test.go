package ui

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc"
)

type launcherResumeServer struct {
	proto.UnimplementedAgentServer
	requests chan *proto.LifecycleControlRequest
}

func (s *launcherResumeServer) LifecycleAcquire(_ context.Context, r *proto.LifecycleControlRequest) (*proto.LifecycleControlResponse, error) {
	s.requests <- r
	return &proto.LifecycleControlResponse{Identity: r.Target, Owner: &proto.LifecycleOwner{Conversation: r.Owner.Conversation, Host: r.Target.Host, Epoch: "existing-epoch"}}, nil
}

// Real authenticated lifecycle transport, disposable token/state, no agent,
// inference, user history or live configuration. Acquisition must use the picker
// selection, not the launcher's initially allocated conversation.
func TestLauncherResumeAcquiresSelectedConversation(t *testing.T) {
	root := t.TempDir()
	token := filepath.Join(root, "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("fixture-host", token)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(auth.ServerOptions()...)
	fixture := &launcherResumeServer{requests: make(chan *proto.LifecycleControlRequest, 1)}
	proto.RegisterAgentServer(server, fixture)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	initial := DevHostSelection{Endpoint: listener.Addr().String(), TokenFile: token, Conversation: "unrelated-startup", Epoch: "startup-epoch", ConfigAuthority: filepath.Join(root, "config.yaml"), Identity: &proto.LifecycleIdentity{Host: "fixture-host", Build: "test", Contract: "test", Schema: 2, Store: filepath.Join(root, "conversations.db")}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
	model := New(&agentclient.Client{}, true).WithDevHost(initial, "test")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(Model)
	if _, ok := model.content.(*historyView); !ok {
		t.Fatalf("resume startup did not open picker: %T", model.content)
	}
	picker := newTestHistoryView([]histRow{{id: "unrelated-startup"}, {id: "saved-conversation"}}, 100, 30)
	picker.cursor = 1
	command, closed := picker.Update(keyPress("enter"))
	if !closed || command == nil {
		t.Fatal("picker did not request resume")
	}
	resume := command().(resumeRequestedMsg)
	reopening, command := model.applyResume(resume.ConversationID)
	if command == nil || !reopening.devHost.busy || reopening.convID != initial.Conversation {
		t.Fatal("resume bypassed lifecycle ownership or did not start reopen")
	}
	selected, err := reopenDevHost(ctx, initial, resume.ConversationID)
	if err != nil {
		t.Fatal(err)
	}
	request := <-fixture.requests
	if request.Owner.Conversation != "saved-conversation" || selected.Conversation != "saved-conversation" || selected.Epoch != "existing-epoch" {
		t.Fatalf("wrong acquisition: request=%v selection=%v", request, selected)
	}
	if selected.ConfigAuthority != initial.ConfigAuthority || selected.Identity != initial.Identity {
		t.Fatal("lost pinned authority")
	}
}
