package ui

import (
	"cercano/source/clients/cli/internal/overlay"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"context"
	"google.golang.org/grpc"
	"net"
	"sync"
	"testing"
)

type cloudSettingsStub struct {
	proto.UnimplementedAgentServer
	mu     sync.Mutex
	calls  []*proto.UpsertCloudProfileRequest
	reject bool
}

func (s *cloudSettingsStub) UpsertCloudProfile(_ context.Context, r *proto.UpsertCloudProfileRequest) (*proto.UpsertCloudProfileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, r)
	if s.reject {
		return &proto.UpsertCloudProfileResponse{Error: "fixture rejection"}, nil
	}
	return &proto.UpsertCloudProfileResponse{Ok: true}, nil
}
func TestCloudSaveRPCSmokeAndErrorRetention(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := &cloudSettingsStub{}
	proto.RegisterAgentServer(server, stub)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()
	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	page := draftTestPage()
	page.agent = client
	for _, edit := range []struct{ key, value string }{{"cloud-quality-premium", "custom"}, {"cloud-image", "custom-image"}, {"cloud-base-url", "https://example.invalid/v1"}} {
		if _, _, err := page.onCommit(edit.key, edit.value); err != nil {
			t.Fatal(err)
		}
		if page.cloudDirty {
			t.Fatal("successful edit left dirty state")
		}
	}
	page.cloudModelsFetched = true
	page.cloudModels = []agentclient.CloudModelInfo{{ID: "picked", Metadata: &agentclient.CatalogMetadata{Chat: agentclient.CatalogCapabilitySupported, Tools: agentclient.CatalogCapabilitySupported}}}
	page.openCatalog("cloud-quality-standard", false)
	page.catalogPicker.State = overlay.CatalogState{}
	page.catalogPicker.Paste("picked")
	page.updateCatalogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if page.cloudDirty || page.catalogPicker != nil || page.cloudDraft.Choices.TierOverrides["standard"] != "picked" {
		t.Fatal("picker did not autosave")
	}
	stub.mu.Lock()
	count := len(stub.calls)
	saved := stub.calls[count-1]
	stub.reject = true
	stub.mu.Unlock()
	if count != 4 || saved.GetModelChoices().GetImageModel() != "custom-image" || saved.GetModelChoices().GetTierOverrides()["premium"] != "custom" || saved.GetStructure() == nil {
		t.Fatal("each edit must persist the complete profile")
	}
	_, _, err = page.onCommit("cloud-quality-premium", "retry-me")
	if err == nil || !page.cloudDirty || page.cloudDraft.Choices.TierOverrides["premium"] != "retry-me" {
		t.Fatal("failed autosave lost editable input")
	}
	stub.mu.Lock()
	stub.reject = false
	stub.mu.Unlock()
	if _, _, err = page.onCommit("cloud-retry", ""); err != nil || page.cloudDirty {
		t.Fatal("retry failed", err)
	}
	for _, f := range page.cloudDetailFields(page.catalogRow()) {
		if f.Key() == "cloud-save" || f.Key() == "cloud-discard" {
			t.Fatal("explicit save/discard controls remain")
		}
	}
}

// cloudSettingsStub uses only in-memory RPC fixtures, never host credentials.
func (*cloudSettingsStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
