package ui

import (
	"context"
	"net"
	"path/filepath"
	"reflect"
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

// Exercise command execution AND UI result application: helper-only tests miss
// the cached-offer shortcut in beginDevHostReopen.
func TestLegacyResumeRejectedApprovalRequeriesDestination(t *testing.T) {
	for _, sourceDies := range []bool{false, true} {
		name := "fresh-offer"
		if sourceDies {
			name = "source-died"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("CERCANO_UI_CONFIG", filepath.Join(t.TempDir(), "ui.yaml"))
			dest, f := resumeFixture(t, true)
			dest.ConfigAuthority, dest.Conversation = "launcher-authority", "current"
			m := New(nil, false).WithDevHost(dest, "test")
			apply := func(r devHostResult) {
				t.Helper()
				next, _ := m.Update(r)
				m = next.(Model)
			}
			reopen := func() devHostResult {
				t.Helper()
				next, action := m.beginDevHostReopen("history")
				m = next
				if action == nil {
					t.Fatal("reopen replayed cached approval instead of querying destination")
				}
				r := action().(devHostResult)
				apply(r)
				return r
			}
			reopen()
			if m.pendingConfirm == nil {
				t.Fatal("missing initial consent")
			}
			f.mu.Lock()
			f.snapshot = "-changed"
			f.mu.Unlock()
			var cmdResult devHostResult
			next, command := m.resolveConfirmKey("y")
			m = next
			if command == nil {
				t.Fatal("missing confirmed command")
			}
			cmdResult = command().(devHostResult)
			if cmdResult.err == nil || cmdResult.client != nil {
				t.Fatal("stale snapshot approval was not rejected")
			}
			apply(cmdResult)
			if m.devHost.busy || m.pendingConfirm != nil || m.convID != "current" {
				t.Fatal("failed attempt changed attachment or left confirmation active")
			}
			f.mu.Lock()
			if f.calls != 2 || f.rejected != 1 {
				t.Fatalf("calls=%d rejected=%d", f.calls, f.rejected)
			}
			f.sourceDead = sourceDies
			f.mu.Unlock()
			refreshed := reopen()
			if sourceDies {
				if refreshed.err != nil || refreshed.client == nil || m.convID != "history" || m.pendingConfirm != nil {
					t.Fatalf("guarded recovery failed: %v", refreshed.err)
				}
				defer refreshed.client.Close()
			} else {
				offer, ok := refreshed.err.(*forcedResumeOffer)
				if !ok || offer.processIdentity != "fixture-process-changed" || m.pendingConfirm == nil {
					t.Fatalf("missing fresh offer: %v", refreshed.err)
				}
				// Every cancellation is local; the next reopen must fetch a new offer.
				for _, key := range []string{"n", "esc", "enter"} {
					f.mu.Lock()
					before := f.calls
					f.mu.Unlock()
					next, command := m.resolveConfirmKey(key)
					m = next
					if command != nil || m.convID != "current" || m.devHost.selection.Identity.Host != "B" {
						t.Fatal("cancel mutated attachment")
					}
					f.mu.Lock()
					after := f.calls
					f.mu.Unlock()
					if before != after {
						t.Fatal("cancel sent RPC")
					}
					reopen()
				}
				next, command := m.resolveConfirmKey("y")
				m = next
				if command == nil {
					t.Fatal("fresh offer requires fresh acceptance")
				}
				accepted := command().(devHostResult)
				if accepted.err != nil || accepted.client == nil {
					t.Fatalf("stable fresh offer refused: %v", accepted.err)
				}
				defer accepted.client.Close()
				apply(accepted)
			}
			if m.devHost.selection.Identity.Host != "B" || !reflect.DeepEqual(m.devHost.defaultSelection, dest) {
				t.Fatal("reopen changed launcher default or attached old source")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if sourceDies && (f.calls != 3 || f.request.ConfirmLegacyStop) {
				t.Fatal("recovery retried destructive approval")
			}
			if !sourceDies && (f.calls != 7 || !f.request.ConfirmLegacyStop || f.request.LegacyProcessIdentity != "fixture-process-changed") {
				t.Fatalf("fresh consent not bound: calls=%d request=%v", f.calls, f.request)
			}
		})
	}
}

func TestResumeHelperRefreshesRejectedLegacySnapshot(t *testing.T) {
	dest, f := resumeFixture(t, true)
	ctx := context.Background()
	_, err := reopenDevHost(ctx, dest, "history")
	old, ok := err.(*forcedResumeOffer)
	if !ok {
		t.Fatalf("missing initial offer: %v", err)
	}
	f.mu.Lock()
	f.snapshot = "-changed"
	f.mu.Unlock()
	controls, err := dest.controls(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer controls.Close()
	_, err = controls.LifecycleResume(ctx, &pb.LifecycleControlRequest{Target: dest.Identity, Source: old.source, Owner: old.owner, HandoverId: old.operation, DestinationEndpoint: dest.Endpoint, Requester: old.requester, ConfirmLegacyStop: true, LegacyProcessIdentity: old.processIdentity})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected stale-snapshot refusal: %v", err)
	}
	_, err = reopenDevHost(ctx, dest, "history")
	fresh, ok := err.(*forcedResumeOffer)
	if !ok || fresh.processIdentity == old.processIdentity {
		t.Fatalf("missing refreshed snapshot: %v", err)
	}
	f.mu.Lock()
	if f.calls != 3 || f.request.ConfirmLegacyStop {
		t.Fatal("refresh auto-authorized old approval")
	}
	f.sourceDead = true
	f.mu.Unlock()
	recovered, err := reopenDevHost(ctx, dest, "history")
	if err != nil || recovered.Identity.Host != "B" || recovered.Epoch != "new" {
		t.Fatalf("destination recovery: %+v %v", recovered, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != 4 || f.request.ConfirmLegacyStop {
		t.Fatal("source death retried destructive approval")
	}
}
