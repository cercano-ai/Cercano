package worker

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/pkg/proto"
)

func TestProfileMutationRequiresTurnConversation(t *testing.T) {
	for _, id := range []string{"", "another-conversation"} {
		t.Run(id, func(t *testing.T) {
			called := false
			w := &workerRunner{setProfile: func(context.Context, string, string) error { called = true; return nil }}
			response := w.handleProfileRequest(context.Background(), "admitted", &proto.SessionProfileRequest{Id: 7, ConversationId: id, Name: "developer"})
			if response.GetOk() || response.GetError() == "" || called {
				t.Fatalf("foreign profile mutation accepted: response=%v callback=%v", response, called)
			}
		})
	}
}

func TestProfileMutationUsesTurnConversation(t *testing.T) {
	called := false
	w := &workerRunner{setProfile: func(_ context.Context, id, name string) error {
		called = true
		if id != "admitted" || name != "developer" {
			t.Fatalf("unexpected callback %q %q", id, name)
		}
		return nil
	}}
	response := w.handleProfileRequest(context.Background(), "admitted", &proto.SessionProfileRequest{Id: 7, ConversationId: "admitted", Name: "developer"})
	if !response.GetOk() || !called || response.GetId() != 7 {
		t.Fatalf("valid request rejected: %v", response)
	}
}

func TestProfileMutationRejectsCanceledTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	w := &workerRunner{setProfile: func(context.Context, string, string) error { called = true; return nil }}
	response := w.handleProfileRequest(ctx, "admitted", &proto.SessionProfileRequest{ConversationId: "admitted"})
	if response.GetOk() || called {
		t.Fatalf("canceled mutation accepted: %v", response)
	}
}

func TestProfileMutationPreservesOwnerRejection(t *testing.T) {
	w := &workerRunner{setProfile: func(context.Context, string, string) error { return errors.New("stale epoch") }}
	response := w.handleProfileRequest(context.Background(), "admitted", &proto.SessionProfileRequest{ConversationId: "admitted"})
	if response.GetOk() || response.GetError() != "stale epoch" {
		t.Fatalf("ownership rejection lost: %v", response)
	}
}
