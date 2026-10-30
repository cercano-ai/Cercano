package server

import (
	"context"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/recap"
)

// This exercises the ordinary constructor, not NewPreparedServer. Attachment
// must preserve its service graph and reject replacement or a split store.
func TestGenerationLifecycleAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conversations.db")
	if err := conversation.PrepareCoexistence(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewCoexistenceServer(t.Context(), CoexistenceOptions{
		StorePath: path, Host: "A", Build: "A", Contract: conversation.CurrentCoexistenceDeclaration(),
		Reload:             func(context.Context, conversation.Store, *conversation.Ownership) error { return nil },
		CloseParticipation: func(context.Context) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := lifecycle.coexistence.store
	t.Cleanup(func() { _ = st.Close() })
	a := agent.NewAgent(nil, nil, agent.WithPersistentStore(st))
	s := NewServerWithLogs(a, nil, nil, nil, nil, filepath.Join(dir, "routing"), filepath.Join(dir, "failures"))
	t.Cleanup(func() { _ = s.Shutdown() })
	cfg, persistence, tools := s.cfgSvc, s.persistSvc, s.toolSvc
	attach, ok := any(s).(interface {
		AttachGenerationLifecycle(*Server, PreparedSchedulingFactory) error
	})
	if !ok {
		t.Fatal("ordinary server has no generation lifecycle attachment")
	}
	if err := attach.AttachGenerationLifecycle(lifecycle, nil); err == nil {
		t.Fatal("missing scheduling accepted")
	}
	if s.coexistence != nil {
		t.Fatal("failed attachment mutated server")
	}
	// A nil-producing factory is sufficient for attachment validation; this is
	// not a feature parity or worker execution acceptance test.
	if err := attach.AttachGenerationLifecycle(lifecycle, func(conversation.Store) (*recap.Generator, *compactiongen.Generator) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if s.cfgSvc != cfg || s.persistSvc != persistence || s.toolSvc != tools || s.agent != a {
		t.Fatal("ordinary services replaced")
	}
	if s.coexistence == nil || lifecycle.coexistence != nil {
		t.Fatal("lifecycle disposal ownership not transferred")
	}
	if err := attach.AttachGenerationLifecycle(lifecycle, nil); err == nil {
		t.Fatal("replacement accepted")
	}
}
