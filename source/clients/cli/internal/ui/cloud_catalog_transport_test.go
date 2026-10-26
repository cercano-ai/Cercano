package ui

import (
	"context"
	"net"
	"strings"
	"testing"

	"cercano/source/clients/cli/internal/overlay"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const catalogTransportReason = "fixture catalog transport disconnected"

type catalogTransportStub struct{ proto.UnimplementedAgentServer }

func (*catalogTransportStub) ListCloudProfileModels(context.Context, *proto.ListCloudProfileModelsRequest) (*proto.ListCloudProfileModelsResponse, error) {
	return nil, status.Error(codes.Unavailable, catalogTransportReason)
}
func (*catalogTransportStub) ListRuntimeModels(context.Context, *proto.ListRuntimeModelsRequest) (*proto.ListRuntimeModelsResponse, error) {
	return nil, status.Error(codes.Unavailable, catalogTransportReason)
}
func TestCatalogTransportErrorVisible(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	proto.RegisterAgentServer(server, &catalogTransportStub{})
	go server.Serve(listener)
	defer server.Stop()
	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, browse := range []bool{false, true} {
		t.Run(map[bool]string{false: "account", true: "provider"}[browse], func(t *testing.T) {
			p := &settingsPage{agent: client, cloudSelected: "profile:test", cloudDraft: cloudDraft{Name: "test"}}
			picker := overlay.NewCatalog("Models", "", false)
			p.catalogPicker = &picker
			cmd := p.startCatalog(false, browse)
			if cmd == nil || !p.catalogBusy {
				t.Fatal("async command not scheduled")
			}
			p.applyCatalogResult(cmd().(cloudCatalogResultMsg))
			if !strings.Contains(p.catalogPicker.State.Message, catalogTransportReason) {
				t.Fatalf("transport reason not visible: %+v", p.catalogPicker.State)
			}
			if p.catalogBusy || p.catalogPicker.State.Action != overlay.CatalogRefresh {
				t.Fatal("failure must allow retry")
			}
			retry := p.startCatalog(true, browse)
			if retry == nil {
				t.Fatal("retry not scheduled")
			}
			result := retry().(cloudCatalogResultMsg)
			result.err = ""
			p.applyCatalogResult(result)
			if p.cloudModelsError != "" || strings.Contains(p.catalogPicker.State.Message, catalogTransportReason) {
				t.Fatal("success retained error")
			}
			p.cloudModelsError = catalogTransportReason
			p.selectCloudRow("other")
			if p.cloudModelsError != "" {
				t.Fatal("account switch retained error")
			}
		})
	}
}

// catalogTransportStub uses only in-memory RPC fixtures, never host credentials.
func (*catalogTransportStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
