package ui

import (
	"context"
	"net"
	"sync"
	"testing"

	"cercano/source/clients/cli/internal/wizard"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type wizardApplyRPCStub struct {
	proto.UnimplementedAgentServer
	mu      sync.Mutex
	updates []*proto.UpdateConfigRequest
}

func (s *wizardApplyRPCStub) UpdateConfig(_ context.Context, req *proto.UpdateConfigRequest) (*proto.UpdateConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, req)
	return &proto.UpdateConfigResponse{Success: true}, nil
}
func newWizardApplyRPC(t *testing.T) (*wizardPage, *wizardApplyRPCStub) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := &wizardApplyRPCStub{}
	proto.RegisterAgentServer(server, stub)
	go server.Serve(listener)
	t.Cleanup(func() { server.Stop(); listener.Close() })
	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return &wizardPage{agent: client}, stub
}
func TestWizardApplyRejectsConflictingCloudChoicesBeforeWrites(t *testing.T) {
	wp, stub := newWizardApplyRPC(t)
	wp.state = wizard.State{LocusMode: "cloud_only", TierPicks: map[string]string{"fast_light.cloud": "one", "fast_light_text.cloud": "two"}, CloudPicksEdited: map[string]bool{"fast_light.cloud": true, "fast_light_text.cloud": true}}
	if err := wp.applyConfig(); err == nil {
		t.Fatal("expected conflicting Economy choices to fail")
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.updates) != 0 {
		t.Fatalf("invalid preview mutated configuration before validation: %d updates", len(stub.updates))
	}
}
func TestWizardApplyRPCScopesEachLocus(t *testing.T) {
	for _, mode := range []string{"cloud_only", "cloud_primary", "open_primary", "open_only"} {
		t.Run(mode, func(t *testing.T) {
			wp, stub := newWizardApplyRPC(t)
			wp.state = wizard.State{LocusMode: mode, TierPicks: map[string]string{"llama_server.everyday": "selected", "everyday.cloud": "autofilled", "unrelated.key": "stale"}}
			if err := wp.applyConfig(); err != nil {
				t.Fatal(err)
			}
			stub.mu.Lock()
			defer stub.mu.Unlock()
			want := 2
			if mode == "cloud_only" {
				want = 1
			}
			if len(stub.updates) != want {
				t.Fatalf("got %d writes; want %d", len(stub.updates), want)
			}
			if stub.updates[0].GetLocusMode() != mode {
				t.Fatal("wrong locus")
			}
			if want == 2 && (stub.updates[1].GetModelTierKey() != "llama_server.everyday" || stub.updates[1].GetModelTierValue() != "selected") {
				t.Fatal("wrong local assignment")
			}
		})
	}
}

func (s *wizardApplyRPCStub) GetConfig(context.Context, *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	return &proto.GetConfigResponse{LocusMode: "open_only", ModelTiers: map[string]string{"llama_server.everyday": "existing-custom"}}, nil
}
func TestWizardResumeDiscoversOverridesBeforeAutofill(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := wizard.Save(wizard.State{Step: wizard.StepOpen, LocusMode: "open_only"}); err != nil {
		t.Fatal(err)
	}
	base, _ := newWizardApplyRPC(t)
	wp := newWizardPage(base.agent, base.palette, base.styles, 80, 24)
	if got := wp.state.TierPicks["llama_server.everyday"]; got != "existing-custom" {
		t.Fatalf("resume lost configured assignment: %q", got)
	}
}

// wizardApplyRPCStub uses only in-memory RPC fixtures, never host credentials.
func (*wizardApplyRPCStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
