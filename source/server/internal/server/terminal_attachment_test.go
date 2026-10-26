package server

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
	"context"
	"errors"
	"google.golang.org/grpc/metadata"
	"path/filepath"
	"testing"
)

func TestParseTerminalBinding(t *testing.T) {
	good := metadata.Pairs(agentclient.PinnedHostMetadata, "h", agentclient.PinnedConversationMetadata, "c", agentclient.PinnedEpochMetadata, "e", agentclient.PinnedTerminalMetadata, "t")
	ctx := metadata.NewIncomingContext(context.Background(), good)
	o, id, err := ParseTerminalBinding(ctx, "c")
	if err != nil || o.Host != "h" || o.Epoch != "e" || id != "t" {
		t.Fatalf("%+v %s %v", o, id, err)
	}
	if _, _, err = ParseTerminalBinding(ctx, "other"); err == nil {
		t.Fatal("mismatch accepted")
	}
	for key := range good {
		md := good.Copy()
		md.Append(key, md.Get(key)[0])
		if _, _, err := ParseTerminalBinding(metadata.NewIncomingContext(context.Background(), md), "c"); err == nil {
			t.Fatal("duplicate accepted", key)
		}
		md = good.Copy()
		md.Delete(key)
		if _, _, err := ParseTerminalBinding(metadata.NewIncomingContext(context.Background(), md), "c"); err == nil {
			t.Fatal("missing accepted", key)
		}
	}
	s := &Server{}
	if err := s.ValidateTerminalPane(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalPreparedBinding(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.db")
	if err := conversation.PrepareCoexistence(ctx, path); err != nil {
		t.Fatal(err)
	}
	s, err := NewPreparedServer(ctx, PreparedServerOptions{
		Coexistence: CoexistenceOptions{StorePath: path, Host: "h", Build: "build", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }},
		Config:      config.Config{OpenRuntime: "synthetic"}, ConfigPath: filepath.Join(dir, "config"), RoutingPath: filepath.Join(dir, "routing"), FailurePath: filepath.Join(dir, "failures"),
		Agent: func(st conversation.Store) (*agent.Agent, error) {
			return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
		},
	}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.closeTaskManager()
		for _, o := range s.coexistence.owners {
			_ = o.Close()
		}
		_ = s.coexistence.store.Close()
		if s.visionStore != nil {
			_ = s.visionStore.Close()
		}
	})
	owner, err := s.Retain(ctx, "c")
	if err != nil {
		t.Fatal(err)
	}
	binding := func(o broker.Owner) context.Context {
		return metadata.NewIncomingContext(ctx, metadata.Pairs(agentclient.PinnedHostMetadata, o.Host, agentclient.PinnedConversationMetadata, o.Conversation, agentclient.PinnedEpochMetadata, o.Epoch, agentclient.PinnedTerminalMetadata, "terminal"))
	}
	exact := binding(owner)
	if err := s.ValidateTerminalPane(exact, "c"); !errors.Is(err, broker.ErrAttachmentEligibility) {
		t.Fatal(err)
	}
	for _, bad := range []context.Context{ctx, binding(broker.Owner{Host: "wrong", Conversation: "c", Epoch: owner.Epoch}), binding(broker.Owner{Host: owner.Host, Conversation: "c", Epoch: "old"})} {
		if _, a, w, err := s.BeginTerminalStream(bad, "c", broker.ForegroundWork); err == nil || a != nil || w != nil {
			t.Fatal("invalid binding admitted", err)
		}
	}
	workctx, a, w, err := s.BeginTerminalStream(exact, "c", broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.resolvePreparedTaskOwner(workctx, "c"); err != nil {
		t.Fatal("exact capability missing", err)
	}
	_, b, _, err := s.BeginTerminalStream(exact, "c", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateTerminalPane(exact, "c"); err != nil {
		t.Fatal(err)
	}
	if err := w.Complete(); err != nil {
		t.Fatal(err)
	}
	closure, err := s.CloseTerminalOwner(exact, "c")
	if err != nil {
		t.Fatal(err)
	}
	if err := closure.Reopen(); err != nil {
		t.Fatal(err)
	}
	a.Detach()
	b.Detach()
	if err := s.ValidateTerminalPane(exact, "c"); !errors.Is(err, broker.ErrAttachmentEligibility) {
		t.Fatal(err)
	}
	if _, retained, open := s.coexistence.admission.CheckRetained(owner); !retained || !open {
		t.Fatal("owner removed")
	}
}
