package server

import (
	"errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
)

func assertReconnectRefused(t *testing.T, s *Server, owner broker.Owner) {
	t.Helper()
	ctx := reproLoopbackContext()
	r := &pb.LifecycleControlRequest{Target: s.coexistence.controlIdentity(), Owner: wireOwner(owner)}
	acquired, err := s.LifecycleAcquire(ctx, r)
	if err == nil || acquired.GetReady() || acquired.GetCurrentOwner() {
		t.Fatalf("closed acquire: %v %v", acquired, err)
	}
	resumed, err := s.LifecycleResume(ctx, r)
	if !errors.Is(err, broker.ErrAdmissionBusy) || resumed.GetReady() || resumed.GetCurrentOwner() {
		t.Fatalf("closed resume: %v %v", resumed, err)
	}
	if _, _, open := s.coexistence.admission.CheckRetained(owner); open {
		t.Fatal("reconnect reopened admission")
	}
	v, err := conversation.ReadOwnership(ctx, s.coexistence.store, owner.Conversation)
	if err != nil || v.State != "owned" || v.Owner != owner.Host || v.Epoch != owner.Epoch {
		t.Fatalf("durable ownership changed: %+v %v", v, err)
	}
}

func TestResumeReconnectAdmission(t *testing.T) {
	for _, mode := range []string{"ready", "closed", "draining"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _ := lifecycleFixture(t)
			serveCutoverFixture(t, s)
			ctx := reproLoopbackContext()
			owner, err := s.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			l := s.coexistence
			l.mu.Lock()
			wrong := owner
			wrong.Epoch += "-stale"
			wrongHost := owner
			wrongHost.Host += "-foreign"
			if !l.currentAndAdmissibleLocked(owner) || l.currentAndAdmissibleLocked(wrong) || l.currentAndAdmissibleLocked(wrongHost) {
				t.Error("readiness did not fence exact owner/epoch")
			}
			l.mu.Unlock()
			switch mode {
			case "closed":
				if _, err := l.admission.CloseOwner(owner, nil); err != nil {
					t.Fatal(err)
				}
			case "draining":
				attachment, err := l.admission.AttachTerminal(owner, "terminal")
				if err != nil {
					t.Fatal(err)
				}
				defer attachment.Detach()
				work, err := s.AdmitRetainedWork(owner, broker.ForegroundWork)
				if err != nil {
					t.Fatal(err)
				}
				defer work.Complete()
				if _, err := l.admission.DrainTerminal(owner, "terminal"); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "ready" {
				assertReconnectRefused(t, s, owner)
				return
			}
			r := &pb.LifecycleControlRequest{Target: l.controlIdentity(), Owner: wireOwner(owner)}
			for i := 0; i < 2; i++ {
				acquired, err := s.LifecycleAcquire(ctx, r)
				if err != nil || controlOwner(&pb.LifecycleControlRequest{Owner: acquired.GetOwner()}) != owner {
					t.Fatalf("acquire: %v %v", acquired, err)
				}
				resumed, err := s.LifecycleResume(ctx, r)
				if err != nil || !resumed.GetReady() || !resumed.GetCurrentOwner() || resumed.GetOwner().GetEpoch() != owner.Epoch {
					t.Fatalf("resume: %v %v", resumed, err)
				}
			}
			ticket, err := s.AdmitRetainedWork(owner, broker.ForegroundWork)
			if err != nil {
				t.Fatal(err)
			}
			ticket.Complete()
		})
	}
}

func TestResumeClaimClosedAdmission(t *testing.T) {
	ctx := reproLoopbackContext()
	a, b, _ := lifecycleFixture(t)
	serveCutoverFixture(t, b)
	source, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := b.AcceptIncoming(source, "closed-claim", "build-A", conversation.CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.HandoverRetained(ctx, source, nil, acceptance); err != nil {
		t.Fatal(err)
	}
	r := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), Owner: wireOwner(source), HandoverId: acceptance.h.HandoverID, Receipt: acceptance.receipt, Management: true}
	first, err := b.resumeClaim(ctx, r)
	if err != nil || !first.GetReady() || !first.GetCurrentOwner() {
		t.Fatalf("first claim: %v %v", first, err)
	}
	owner := controlOwner(&pb.LifecycleControlRequest{Owner: first.Owner})
	if _, err := b.coexistence.admission.CloseOwner(owner, nil); err != nil {
		t.Fatal(err)
	}
	for _, claim := range []func() (*pb.LifecycleControlResponse, error){
		func() (*pb.LifecycleControlResponse, error) { return b.LifecycleClaim(ctx, r) },
		func() (*pb.LifecycleControlResponse, error) { return b.resumeClaim(ctx, r) },
	} {
		got, err := claim()
		if status.Code(err) != codes.FailedPrecondition || got.GetReady() || got.GetCurrentOwner() {
			t.Fatalf("closed claim: %v %v", got, err)
		}
	}
	assertReconnectRefused(t, b, owner)
}
