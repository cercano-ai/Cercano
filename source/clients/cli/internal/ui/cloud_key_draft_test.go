package ui

import (
	"cercano/source/clients/cli/internal/form"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"context"
	"google.golang.org/grpc"
	"net"
	"sync/atomic"
	"testing"
)

type keyDraftStub struct {
	proto.UnimplementedAgentServer
	events        chan string
	rejectKey     atomic.Bool
	rejectProfile atomic.Bool
}

func (s *keyDraftStub) UpsertCloudProfile(context.Context, *proto.UpsertCloudProfileRequest) (*proto.UpsertCloudProfileResponse, error) {
	s.events <- "profile"
	if s.rejectProfile.Load() {
		return &proto.UpsertCloudProfileResponse{Error: "fixture profile failure"}, nil
	}
	return &proto.UpsertCloudProfileResponse{Ok: true}, nil
}
func (s *keyDraftStub) SetCloudProfileKey(_ context.Context, r *proto.SetCloudProfileKeyRequest) (*proto.SetCloudProfileKeyResponse, error) {
	s.events <- "key"
	if s.rejectKey.Load() {
		return &proto.SetCloudProfileKeyResponse{Error: "fixture key failure"}, nil
	}
	return &proto.SetCloudProfileKeyResponse{Ok: r.Name == "deepinfra" && r.ApiKey == "synthetic-token"}, nil
}
func TestCloudKeyAutosavesOnCommit(t *testing.T) {
	for _, fresh := range []bool{true, false} {
		t.Run(map[bool]string{true: "new", false: "existing"}[fresh], func(t *testing.T) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			s := grpc.NewServer()
			stub := &keyDraftStub{events: make(chan string, 20)}
			proto.RegisterAgentServer(s, stub)
			go s.Serve(l)
			defer s.Stop()
			c, err := agentclient.DialExisting(context.Background(), l.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			p := draftTestPage()
			p.agent = c
			p.cloudDraft.Name = "deepinfra"
			p.cloudDraftNew = fresh
			field := form.NewMasked("cloud-key", "API key", !fresh)
			p.form = form.New([]form.Section{{Fields: []form.Field{field}}})
			p.form.OnCommit = func(k, v string) (string, tea.Cmd, error) { return p.commitCloud(classifyCloudCommit(k, v)) }
			p.form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			p.handlePaste("synthetic-token")
			p.form.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			// Typing alone is local; leaving the editor commits and persists the key.
			if len(stub.events) != 0 {
				t.Fatal("typing persisted an uncommitted key")
			}
			if field.Editing() {
				p.form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
			if <-stub.events != "profile" || <-stub.events != "key" || p.cloudDirty || p.cloudDraft.apiKeyEdited {
				t.Fatal("committed key did not autosave profile then credential")
			}
			stub.rejectProfile.Store(true)
			if _, _, err = p.commitCloud(classifyCloudCommit("cloud-key", "synthetic-token")); err == nil {
				t.Fatal("profile failure ignored")
			}
			if <-stub.events != "profile" || len(stub.events) != 0 || !p.cloudDirty {
				t.Fatal("profile failure lost draft or stored key")
			}
			stub.rejectProfile.Store(false)
			stub.rejectKey.Store(true)
			if _, _, err = p.commitCloud(classifyCloudCommit("cloud-save", "")); err == nil {
				t.Fatal("key failure ignored")
			}
			if <-stub.events != "profile" || <-stub.events != "key" || !p.cloudDirty {
				t.Fatal("key save ordering or retention failed")
			}
			stub.rejectKey.Store(false)
			p.cloudModels = []agentclient.CloudModelInfo{{ID: "old-private-model"}}
			p.cloudModelsFetched = true
			p.cloudModelsError = "old discovery error"
			if _, _, err = p.commitCloud(classifyCloudCommit("cloud-save", "")); err != nil {
				t.Fatal(err)
			}
			if <-stub.events != "profile" || <-stub.events != "key" || p.cloudDirty {
				t.Fatal("retry did not save draft")
			}
			if len(p.cloudModels) != 0 || p.cloudModelsFetched || p.cloudModelsError != "" {
				t.Fatal("saved account retained stale model cache")
			}
			p.commitCloud(classifyCloudCommit("cloud-save", ""))
			if <-stub.events != "profile" || len(stub.events) != 0 {
				t.Fatal("saved secret retained")
			}

		})
	}
}

// keyDraftStub uses only in-memory RPC fixtures, never host credentials.
func (*keyDraftStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
