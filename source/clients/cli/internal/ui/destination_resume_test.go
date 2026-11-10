package ui

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type destinationResumeFixture struct {
	pb.UnimplementedAgentServer
	mu         sync.Mutex
	snapshot   string
	sourceDead bool
	rejected   int
	identity   *pb.LifecycleIdentity
	calls      int
	request    *pb.LifecycleControlRequest
	legacy     bool
	noAdapter  bool
}

func (s *destinationResumeFixture) LifecycleResume(_ context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.request = r
	if s.legacy && r.ConfirmLegacyStop && r.LegacyProcessIdentity != "fixture-process"+s.snapshot {
		s.rejected++
		return nil, status.Error(codes.FailedPrecondition, "source snapshot changed")
	}
	if s.legacy && !s.sourceDead && !r.ConfirmLegacyStop {
		identity := "fixture-process" + s.snapshot
		if s.noAdapter {
			identity = ""
		}
		return &pb.LifecycleControlResponse{Identity: s.identity, Source: &pb.LifecycleIdentity{Host: "A", Build: "old", Store: s.identity.Store}, Owner: &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: "A", Epoch: "old"}, OperationId: "stable", LegacyProcessIdentity: identity, ConfirmationRequired: true, BlockedReason: "busy"}, nil
	}
	return &pb.LifecycleControlResponse{Identity: s.identity, Owner: &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: s.identity.Host, Epoch: "new"}, Ready: true}, nil
}
func resumeFixture(t *testing.T, legacy bool) (DevHostSelection, *destinationResumeFixture) {
	t.Helper()
	token := filepath.Join(t.TempDir(), "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("B", token)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	id := &pb.LifecycleIdentity{Host: "B", Build: "fixture", Contract: "fixture", Schema: 2, Store: "fixture.db"}
	f := &destinationResumeFixture{identity: id, legacy: legacy}
	g := grpc.NewServer(auth.ServerOptions()...)
	pb.RegisterAgentServer(g, f)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	return DevHostSelection{Identity: id, Endpoint: lis.Addr().String(), TokenFile: token}, f
}
func TestResumeUsesSelectedDestinationRPC(t *testing.T) {
	dest, f := resumeFixture(t, false)
	got, err := reopenDevHost(context.Background(), dest, "history")
	if err != nil {
		t.Fatal(err)
	}
	if got.Identity.Host != "B" || got.Epoch != "new" || f.calls != 1 || f.request.DestinationEndpoint != dest.Endpoint {
		t.Fatalf("not selected destination resume: %+v %+v", got, f.request)
	}
}
func (s *destinationResumeFixture) GetDevelopmentHostIdentity(context.Context, *pb.GetDevelopmentHostIdentityRequest) (*pb.GetDevelopmentHostIdentityResponse, error) {
	return &pb.GetDevelopmentHostIdentityResponse{HostInstance: s.identity.Host, Build: s.identity.Build, Contract: s.identity.Contract, SchemaVersion: s.identity.Schema}, nil
}
func (s *destinationResumeFixture) CheckRetainedSession(context.Context, *pb.CheckRetainedSessionRequest) (*pb.CheckRetainedSessionResponse, error) {
	return &pb.CheckRetainedSessionResponse{State: pb.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
}
func (s *destinationResumeFixture) GetCredentialPolicy(context.Context, *pb.GetCredentialPolicyRequest) (*pb.GetCredentialPolicyResponse, error) {
	return &pb.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
func TestResumeLegacyOfferRequiresUpgrade(t *testing.T) {
	for _, noAdapter := range []bool{false, true} {
		dest, f := resumeFixture(t, true)
		f.noAdapter = noAdapter
		_, err := reopenDevHost(context.Background(), dest, "history")
		if _, ok := err.(*forcedResumeOffer); ok || err == nil || !strings.Contains(strings.ToLower(err.Error()), "upgrade") {
			t.Fatalf("legacy offer actionable: %v", err)
		}
		if f.calls != 1 || f.request.ConfirmLegacyStop {
			t.Fatal("legacy confirmation sent")
		}
	}
}
