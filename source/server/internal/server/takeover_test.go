package server

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc/metadata"
	"testing"
)

func TestTakeoverMutationAdmission(t *testing.T) {
	s := permissionFixture(t)
	owner, err := s.Retain(context.Background(), "c")
	if err != nil {
		t.Fatal(err)
	}
	binding := func(id string, takeover bool) context.Context {
		md := metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", "c", "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", id)
		if takeover {
			md.Set("cercano-takeover-terminal", "old")
		}
		return metadata.NewIncomingContext(context.Background(), md)
	}
	old := binding("old", false)
	fresh := binding("new", false)
	_, attachment, work, err := s.BeginTerminalStream(old, "c", broker.ForegroundWork)
	if err != nil {
		t.Fatal(err)
	}
	_, replacement, _, err := s.BeginTerminalStream(binding("new", true), "c", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Detach()
	attachment.Detach()
	s.ordinaryGeneration = true
	for name, action := range map[string]func() error{
		"reconnect":  func() error { _, _, _, e := s.BeginTerminalStream(old, "c", 0); return e },
		"foreground": func() error { _, _, _, e := s.BeginTerminalStream(old, "c", broker.ForegroundWork); return e },
		"permission": func() error { _, e := s.resolvePermission(old, "c", "tool", agent.Decision{}); return e },
		"pane":       func() error { return s.ValidateTerminalPane(old, "c") },
		"runtime":    func() error { _, _, e := s.beginOrdinaryServiceWork(old, "c"); return e },
		"shared":     func() error { _, _, e := s.beginOrdinaryServiceWork(old, ""); return e },
		"release":    func() error { _, e := s.CloseTerminalOwner(old, "c"); return e },
		"retained": func() error {
			_, e := s.CheckRetainedSession(old, &proto.CheckRetainedSessionRequest{HostInstance: owner.Host, ConversationId: "c", OwnershipEpoch: owner.Epoch})
			return e
		},
	} {
		t.Run(name, func(t *testing.T) {
			if action() == nil {
				t.Fatal("stale mutation accepted")
			}
		})
	}
	if err := s.ValidateTerminalPane(fresh, "c"); err != nil {
		t.Fatal(err)
	}
	if err := work.Advance(broker.PersistenceWork); err != nil {
		t.Fatal(err)
	}
	if err := work.Complete(); err != nil {
		t.Fatal(err)
	}
}
