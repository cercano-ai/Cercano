package ui

import (
	"cercano/source/clients/cli/internal/form"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"net"
	"sync/atomic"
	"testing"
)

type catalogSaveStub struct {
	proto.UnimplementedAgentServer
	key, reject atomic.Bool
	discoveries atomic.Int32
}

func (s *catalogSaveStub) UpsertCloudProfile(context.Context, *proto.UpsertCloudProfileRequest) (*proto.UpsertCloudProfileResponse, error) {
	return &proto.UpsertCloudProfileResponse{Ok: true}, nil
}
func (s *catalogSaveStub) SetCloudProfileKey(context.Context, *proto.SetCloudProfileKeyRequest) (*proto.SetCloudProfileKeyResponse, error) {
	if s.reject.Load() {
		return &proto.SetCloudProfileKeyResponse{Error: "fixture rejected key"}, nil
	}
	s.key.Store(true)
	return &proto.SetCloudProfileKeyResponse{Ok: true}, nil
}
func (s *catalogSaveStub) GetCloudProviders(context.Context, *proto.GetCloudProvidersRequest) (*proto.GetCloudProvidersResponse, error) {
	return &proto.GetCloudProvidersResponse{Providers: []*proto.CloudProvider{{Id: "mistral", Label: "Mistral", Flavor: "chat_completions", BaseUrl: "https://api.mistral.ai/v1", PrimaryProfile: "mistral", Profiles: []*proto.CloudProfileInfo{{Name: "mistral", Flavor: "chat_completions", BaseUrl: "https://api.mistral.ai/v1", HasKey: s.key.Load()}}}}}, nil
}
func (s *catalogSaveStub) ListCloudProfileModels(context.Context, *proto.ListCloudProfileModelsRequest) (*proto.ListCloudProfileModelsResponse, error) {
	s.discoveries.Add(1)
	return &proto.ListCloudProfileModelsResponse{Models: []*proto.CloudModelInfo{{Id: "available-model"}}}, nil
}
func TestCatalogSaveSchedulesFreshDiscovery(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := &catalogSaveStub{}
	proto.RegisterAgentServer(server, stub)
	go server.Serve(l)
	defer server.Stop()
	client, err := agentclient.DialExisting(context.Background(), l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	p := cloudSamplePage()
	p.scope = scopeCloud
	p.agent = client
	p.profilesLoaded = false
	p.snapshotSections()
	p.selectCloudRow("profile:mistral")
	p.form = form.New(p.snapshotSections())
	p.form.OnReload = p.snapshotSections
	if p.startCatalog(false, false) != nil || stub.discoveries.Load() != 0 {
		t.Fatal("missing saved key triggered discovery")
	}
	stub.reject.Store(true)
	_, cmd, err := p.onCommit("cloud-key", "synthetic-token")
	if err == nil || cmd != nil || !p.cloudDraft.apiKeyEdited {
		t.Fatal("failed key save lost draft or scheduled discovery")
	}
	stub.reject.Store(false)
	_, cmd, err = p.onCommit("cloud-save", form.ButtonActivate)
	if err != nil || cmd == nil {
		t.Fatal("successful save did not schedule discovery", err)
	}
	// Form.commit reloads the provider snapshot before executing its returned command.
	p.form.Reload()
	kick, ok := cmd().(cloudCatalogKickMsg)
	if !ok || kick.page != p {
		t.Fatal("missing page-scoped post-save command")
	}
	fetch := p.startCatalog(false, false)
	if fetch == nil {
		t.Fatal("saved credential still blocked discovery")
	}
	p.applyCatalogResult(fetch().(cloudCatalogResultMsg))
	if stub.discoveries.Load() != 1 || len(p.cloudModels) != 1 || p.cloudModels[0].ID != "available-model" {
		t.Fatal("fresh catalog did not populate after save")
	}
	if p.cloudDraft.apiKey != "" || p.cloudDraft.apiKeyEdited || p.cloudDirty {
		t.Fatal("save retained credential draft")
	}
}

// catalogSaveStub uses only in-memory RPC fixtures, never host credentials.
func (*catalogSaveStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
