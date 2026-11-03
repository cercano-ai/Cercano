package ui

import (
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

type destinationResumeFixture struct {
	pb.UnimplementedAgentServer
	identity  *pb.LifecycleIdentity
	calls     int
	request   *pb.LifecycleControlRequest
	legacy    bool
	noAdapter bool
}

func (s *destinationResumeFixture) LifecycleResume(_ context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	s.calls++
	s.request = r
	if s.legacy && !r.ConfirmLegacyStop {
		identity := "fixture-process"
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
func TestResumeLegacyReturnsBoundOffer(t *testing.T) {
	dest, f := resumeFixture(t, true)
	got, err := reopenDevHost(context.Background(), dest, "history")
	offer, ok := err.(*forcedResumeOffer)
	if !ok || offer.owner.Epoch != "old" || got.Identity.Host != "B" || f.calls != 1 {
		t.Fatalf("missing destination legacy offer: %+v %v", got, err)
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
func TestLegacyResumePromptAndBoundAcceptance(t *testing.T) {
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
	dest, f := resumeFixture(t, true)
	_, err := reopenDevHost(context.Background(), dest, "history")
	offer, ok := err.(*forcedResumeOffer)
	if !ok {
		t.Fatal(err)
	}
	m := New(nil, false).WithDevHost(dest, "test")
	next, _ := m.offerForcedResume(offer)
	m = next.(Model)
	text := m.pendingConfirm.title + " " + strings.Join(m.pendingConfirm.details, " ")
	for _, word := range []string{"entire legacy source host", "A", "history", "inventory is incomplete", "ALL conversations and running tasks", "not just the selected conversation"} {
		if !strings.Contains(text, word) {
			t.Fatalf("warning lacks %q: %s", word, text)
		}
	}
	verifyProductionForcedResume(t, offer)
	if f.calls != 2 || !f.request.ConfirmLegacyStop || f.request.Source.Host != "A" || f.request.Owner.Epoch != "old" || f.request.HandoverId != "stable" || f.request.LegacyProcessIdentity != "fixture-process" || f.request.Target.Host != "B" || f.request.Requester.Host != "B" {
		t.Fatalf("unbound approval: %v calls=%d", f.request, f.calls)
	}
}

func TestLegacyNoAdapterHasNoActionableOffer(t *testing.T) {
	dest, f := resumeFixture(t, true)
	f.noAdapter = true
	_, err := reopenDevHost(context.Background(), dest, "history")
	if _, ok := err.(*forcedResumeOffer); ok || err == nil {
		t.Fatalf("unsafe actionable offer: %v", err)
	}
}
