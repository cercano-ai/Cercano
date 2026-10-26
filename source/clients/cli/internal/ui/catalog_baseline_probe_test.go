package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"net"
	"testing"
	"time"
)

type catalogBaselineStub struct {
	proto.UnimplementedAgentServer
	called  chan struct{}
	release chan struct{}
}

func (s *catalogBaselineStub) ListCloudProfileModels(ctx context.Context, _ *proto.ListCloudProfileModelsRequest) (*proto.ListCloudProfileModelsResponse, error) {
	close(s.called)
	select {
	case <-s.release:
	case <-ctx.Done():
	}
	return &proto.ListCloudProfileModelsResponse{Error: "no API key"}, nil
}
func TestCatalogDiscoveryIsAsyncAndIgnoresStaleResults(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	s := grpc.NewServer()
	stub := &catalogBaselineStub{called: make(chan struct{}), release: make(chan struct{})}
	proto.RegisterAgentServer(s, stub)
	go s.Serve(l)
	defer s.Stop()
	c, e := agentclient.DialExisting(context.Background(), l.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	p := &settingsPage{agent: c, cloudSelected: "profile:test", cloudDraft: cloudDraft{Name: "test"}}
	// Form options must be cache-only: no network activity before a command runs.
	p.cloudDetailFields(p.catalogRow())
	select {
	case <-stub.called:
		t.Fatal("form construction performed discovery")
	default:
	}
	cmd := p.startCatalog(false, false)
	if cmd == nil || !p.catalogBusy {
		t.Fatal("async command not scheduled")
	}
	if p.startCatalog(false, false) != nil {
		t.Fatal("duplicate request scheduled")
	}
	done := make(chan cloudCatalogResultMsg, 1)
	go func() { done <- cmd().(cloudCatalogResultMsg) }()
	select {
	case <-stub.called:
	case <-time.After(time.Second):
		t.Fatal("command never reached server")
	}
	// While the server is blocked, synchronous cached reads still finish.
	p.cloudDetailFields(p.catalogRow())
	p.selectCloudRow("other")
	close(stub.release)
	msg := <-done
	p.applyCatalogResult(msg)
	if p.cloudModelsFetched || p.cloudModelsError != "" {
		t.Fatal("stale result applied to another account")
	}
}
func TestCatalogPrerequisiteStates(t *testing.T) {
	p := cloudSamplePage()
	p.selectCloudRow("template:gemini")
	if text, field := p.catalogPrerequisite(); text != "Add an API key to load available models." || field != "cloud-key" {
		t.Fatalf("missing-key state: %q %q", text, field)
	}
	p.cloudDraft.apiKeyEdited = true
	p.cloudDraft.apiKey = "synthetic"
	if _, field := p.catalogPrerequisite(); field != "cloud-retry" {
		t.Fatal("unsaved credential treated as stored")
	}
	p.cloudDraft.apiKeyEdited = false
	p.cloudDraft.Route = "subscription"
	if _, field := p.catalogPrerequisite(); field != "cloud-signin-claude" {
		t.Fatal("subscription incorrectly requests key")
	}
	p.selectCloudRow("profile:work-openai")
	if text, _ := p.catalogPrerequisite(); text != "" {
		t.Fatal("stored account blocked", text)
	}
	p.cloudDraft.BaseURL = "https://changed.example"
	if _, field := p.catalogPrerequisite(); field != "cloud-retry" {
		t.Fatal("unsaved endpoint used for discovery")
	}
}

// catalogBaselineStub uses only in-memory RPC fixtures, never host credentials.
func (*catalogBaselineStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
