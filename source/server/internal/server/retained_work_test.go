package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetainedWorkScope(t *testing.T) {
	a, _, _ := lifecycleFixture(t)
	owner, err := a.Retain(t.Context(), "scope")
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := a.AttachRetained(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer attachment.Detach()
	wrong := owner
	wrong.Epoch = "stale"
	if _, _, err := a.BeginRetainedWork(t.Context(), wrong, broker.ForegroundWork); !errors.Is(err, broker.ErrAdmissionStale) {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(t.Context())
	ctx, ticket, err := a.BeginRetainedWork(parent, owner, broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	writeCtx, done := persistsvc.BoundedWriteContext(ctx, time.Second)
	defer done()
	if err := a.coexistence.store.EnsureConversation(writeCtx, owner.Conversation, "", ""); err != nil {
		t.Fatalf("ownership lost on bounded write: %v", err)
	}
	if err := a.ReleaseRetained(t.Context(), owner, attachment); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("live work released: %v", err)
	}
	failure := errors.New("unresolved write")
	persistsvc.ReportWriteFailure(writeCtx, failure)
	if !errors.Is(persistsvc.WriteFailure(ctx), failure) {
		t.Fatal("descendant outcome lost")
	}
	if err := ticket.Complete(); err == nil {
		t.Fatal("failed persistence completed")
	}
	if err := a.ReleaseRetained(t.Context(), owner, attachment); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("unresolved work released: %v", err)
	}
}

func TestCoexistenceOrdinaryPathsRefuseBeforeMutation(t *testing.T) {
	a, _, _ := lifecycleFixture(t)
	req := &proto.ProcessRequestRequest{ConversationId: "unowned", DebugMode: true, WorkDir: "/not-used"}
	if _, err := a.ProcessRequest(t.Context(), req); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unary fallback: %v", err)
	}
	// Nil collaborators/streams deliberately prove refusal precedes access,
	// dev-mode persistence, broker supersession, or provider execution.
	if err := a.StreamProcessRequest(req, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if err := a.streamConversationTurn(req, nil, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if err := a.SubscribeTasks(nil, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	if err := a.AttachConversation(nil, nil); status.Code(err) != codes.FailedPrecondition {
		t.Fatal(err)
	}
	a.tryTaskCompletionWake(t.Context(), "unowned")
}
