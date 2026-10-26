package server

import (
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/proto"
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"reflect"
	"testing"
)

func TestPreparedAdminStaleAdmission(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	before, err := s.persistSvc.Store().Get(ctx, "parent")
	if err != nil {
		t.Fatal(err)
	}
	owner.Epoch = "stale"
	ctx = foregroundBinding(ctx, owner)
	if _, err := s.RenameConversation(ctx, &proto.RenameConversationRequest{ConversationId: "parent", Title: "wrong"}); err == nil {
		t.Error("stale rename accepted")
	}
	if _, err := s.DismissSubAgent(ctx, &proto.DismissSubAgentRequest{ConversationId: "parent"}); err == nil {
		t.Error("stale dismiss accepted")
	}
	if _, err := s.DeleteConversation(ctx, &proto.DeleteConversationRequest{ConversationId: "parent"}); err == nil {
		t.Error("stale delete accepted")
	}
	after, err := s.persistSvc.Store().Get(ctx, "parent")
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("stale request effects: %v %v", after, err)
	}
}

type blockedRename struct {
	persistsvc.Service
	entered, finish chan struct{}
	fail            bool
}

func (b blockedRename) RenameConversation(ctx context.Context, req *proto.RenameConversationRequest) (*proto.RenameConversationResponse, error) {
	close(b.entered)
	<-b.finish
	if b.fail {
		return nil, errors.New("injected write failure")
	}
	return b.Service.RenameConversation(ctx, req)
}
func TestPreparedAdminWorkAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			s, ctx, owner, _ := preparedWakeFixture(t)
			b := blockedRename{s.persistSvc, make(chan struct{}), make(chan struct{}), fail}
			s.persistSvc = b
			done := make(chan error, 1)
			go func() {
				resp, err := s.RenameConversation(foregroundBinding(ctx, owner), &proto.RenameConversationRequest{ConversationId: "parent", Title: "renamed"})
				if fail && resp != nil {
					done <- errors.New("published failed rename")
					return
				}
				done <- err
			}()
			<-b.entered
			if err := s.ReleaseRetained(ctx, owner, nil); err == nil {
				t.Fatalf("release during write: %v", err)
			}
			close(b.finish)
			err := <-done
			if (err != nil) != fail {
				t.Fatalf("rename error: %v", err)
			}
			err = s.ReleaseRetained(ctx, owner, nil)
			if fail && !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("failed write released: %v", err)
			}
			if !fail && err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestPreparedRegenerationFailsBeforeScheduling(t *testing.T) {
	for _, disabled := range []string{"0", "1"} {
		t.Run("credentials-disabled="+disabled, func(t *testing.T) {
			// The fixture supplies only an in-memory credential store.
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", disabled)
			s, ctx, _, _ := preparedWakeFixture(t)
			// Even explicit scheduler attachment must stay disabled. A zero-value
			// generator must never be configured or invoked in this mode.
			s.persistSvc.SetCompactionGenerator(&compactiongen.Generator{})
			if s.persistSvc.CompactionGen() != nil {
				t.Fatal("prepared service accepted a compaction scheduler")
			}
			before, err := s.persistSvc.Store().Get(ctx, "parent")
			if err != nil {
				t.Fatal(err)
			}
			for name, req := range map[string]*proto.RegenerateContextRequest{
				"regenerate": {ConversationId: "parent"},
				"compact":    {ConversationId: "parent", Incremental: true},
				"clear":      {ConversationId: "parent", ClearOnly: true},
				"invalid":    {},
			} {
				t.Run(name, func(t *testing.T) {
					// Nil stream proves refusal precedes publication and provider work.
					for entry, call := range map[string]func(*proto.RegenerateContextRequest, proto.Agent_RegenerateContextServer) error{
						"server": s.RegenerateContext, "service": s.persistSvc.RegenerateContext,
					} {
						if err := call(req, nil); status.Code(err) != codes.FailedPrecondition {
							t.Fatalf("%s: expected coexistence refusal, got %v", entry, err)
						}
					}
				})
			}
			if after, err := s.persistSvc.Store().Get(ctx, "parent"); err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("regeneration effects: %v %v", after, err)
			}
			want := secrets.CheckAllowed()
			if want == nil {
				want = secrets.ErrReadOnly
			}
			if err := s.cfgSvc.Credentials().CheckMutation(); !errors.Is(err, want) {
				t.Fatalf("credential refusal: got %v, want %v", err, want)
			}
		})
	}
}

func TestPreparedGlobalMutationsFailBeforeEffects(t *testing.T) {
	s, ctx, _, _ := preparedWakeFixture(t)
	tests := map[string]func() error{
		"reset":    func() error { _, err := s.ResetSetup(ctx, &proto.ResetSetupRequest{Confirmed: true}); return err },
		"restart":  func() error { _, err := s.RestartRuntime(ctx, nil); return err },
		"download": func() error { _, err := s.DownloadRuntimeModel(ctx, nil); return err },
		"stop":     func() error { _, err := s.StopRuntimeModel(ctx, nil); return err },
		"install":  func() error { return s.InstallOpenRuntime(nil, nil) },
		"login":    func() error { return s.StartChatGPTLogin(nil, nil) },
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			if err := run(); status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("expected coexistence refusal, got %v", err)
			}
		})
	}
}
