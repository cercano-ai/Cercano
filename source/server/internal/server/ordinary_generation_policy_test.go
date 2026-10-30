package server

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/recap"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func ordinaryPolicyFixture(t *testing.T) (*Server, context.Context, broker.Owner, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "conversations.db")
	if err := conversation.PrepareCoexistence(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	l, err := NewCoexistenceServer(t.Context(), CoexistenceOptions{StorePath: path, Host: "A", Build: "A", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	st := l.coexistence.store
	s := NewServerWithLogs(agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil, nil, nil, nil, filepath.Join(dir, "routing"), filepath.Join(dir, "failures"))
	t.Cleanup(func() { _ = s.Shutdown(); _ = st.Close() })
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Watchdog.Enabled = false
	if err := config.Save(cfg, cfgPath); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetConfigPersistence(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := s.AttachGenerationLifecycle(l, func(conversation.Store) (*recap.Generator, *compactiongen.Generator) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	owner, err := s.Retain(t.Context(), "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	ctx, ticket, err := s.BeginRetainedWork(t.Context(), owner, broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureConversation(ctx, owner.Conversation, "", "ordinary"); err != nil {
		t.Fatal(err)
	}
	ticket.Complete()
	return s, foregroundBinding(t.Context(), owner), owner, cfgPath
}

func TestOrdinaryGenerationSettingsPolicy(t *testing.T) {
	s, ctx, owner, path := ordinaryPolicyFixture(t)
	// A normal setting beyond the prepared two-field subset, then a legitimate
	// externally published normal revision followed by an ordinary RPC update.
	if _, err := s.UpdateConfig(ctx, &proto.UpdateConfigRequest{OpenRuntime: "ollama"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.OpenRuntime = "llama_server"
	if err := config.Save(cfg, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateConfig(ctx, &proto.UpdateConfigRequest{ToolLoopMaxIterations: "12"}); err != nil {
		t.Fatal(err)
	}
	if s.cfgSvc.Get().OpenRuntime != "llama_server" {
		t.Fatal("external ordinary revision not adopted")
	}
	owner.Epoch = "stale"
	if _, err := s.UpdateConfig(foregroundBinding(t.Context(), owner), &proto.UpdateConfigRequest{OpenRuntime: "ollama"}); err == nil {
		t.Fatal("stale settings accepted")
	}
}

func TestOrdinaryGenerationOperationPolicy(t *testing.T) {
	s, ctx, owner, _ := ordinaryPolicyFixture(t)
	if _, err := s.RestartRuntime(ctx, &proto.RestartRuntimeRequest{}); err != nil {
		t.Fatalf("ordinary operation rejected before runtime manager: %v", err)
	}
	stale := owner
	stale.Epoch = "stale"
	if _, err := s.RestartRuntime(foregroundBinding(t.Context(), stale), &proto.RestartRuntimeRequest{}); err == nil {
		t.Fatal("stale operation admitted")
	}
	if _, err := s.DeleteConversation(ctx, &proto.DeleteConversationRequest{ConversationId: owner.Conversation}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteConversation(t.Context(), &proto.DeleteConversationRequest{ConversationId: owner.Conversation}); err == nil {
		t.Fatal("unbound deletion admitted")
	}
}

func TestPreparedGenerationRestrictionsRemain(t *testing.T) {
	s, ctx, owner, _ := preparedWakeFixture(t)
	ctx = foregroundBinding(ctx, owner)
	if _, err := s.UpdateConfig(ctx, &proto.UpdateConfigRequest{OpenRuntime: "ollama"}); err == nil {
		t.Fatal("prepared settings widened")
	}
	if _, err := s.RestartRuntime(ctx, &proto.RestartRuntimeRequest{}); err == nil {
		t.Fatal("prepared runtime mutation admitted")
	}
	if _, err := s.DeleteConversation(ctx, &proto.DeleteConversationRequest{ConversationId: owner.Conversation}); err == nil {
		t.Fatal("prepared deletion admitted")
	}
}

func TestOrdinaryGenerationProfileAndRollover(t *testing.T) {
	s, ctx, owner, _ := ordinaryPolicyFixture(t)
	s.rollover = newRolloverManager(rolloverConfig{RawTokenThreshold: 1000})
	if r, err := s.SetSessionProfile(ctx, &proto.SetSessionProfileRequest{ConversationId: owner.Conversation, Name: agent.DefaultProfileName}); err != nil || !r.GetOk() {
		t.Fatalf("profile: %v %v", r, err)
	}
	offer := s.rollover.NoteOffered(owner.Conversation, 1000)
	if r, err := s.DeclineRollover(ctx, &proto.DeclineRolloverRequest{ConversationId: owner.Conversation, OfferId: offer}); err != nil || !r.GetOk() {
		t.Fatalf("decline: %v %v", r, err)
	}
	offer = s.rollover.NoteOffered(owner.Conversation, 2000)
	r, err := s.AcceptRollover(ctx, &proto.AcceptRolloverRequest{ConversationId: owner.Conversation, OfferId: offer})
	if err != nil || !r.GetOk() {
		t.Fatalf("accept: %v %v", r, err)
	}
	st := s.GenerationStore()
	if precursor, err := st.Precursor(ctx, r.NewConversationId); err != nil || precursor != owner.Conversation {
		t.Fatalf("precursor: %q %v", precursor, err)
	}
	turns, err := st.GetTurns(ctx, r.NewConversationId)
	if err != nil || len(turns) != 1 {
		t.Fatalf("handoff: %v %v", turns, err)
	}
}

// Each RPC must reject stale/unbound authority and retain the prepared gate,
// including before validation or rollover-manager mutation.
func TestOrdinaryGenerationProfileRolloverAuthority(t *testing.T) {
	calls := map[string]func(*Server, context.Context, string) error{
		"profile": func(s *Server, ctx context.Context, id string) error {
			_, err := s.SetSessionProfile(ctx, &proto.SetSessionProfileRequest{ConversationId: id, Name: agent.DefaultProfileName})
			return err
		},
		"accept": func(s *Server, ctx context.Context, id string) error {
			_, err := s.AcceptRollover(ctx, &proto.AcceptRolloverRequest{ConversationId: id})
			return err
		},
		"decline": func(s *Server, ctx context.Context, id string) error {
			_, err := s.DeclineRollover(ctx, &proto.DeclineRolloverRequest{ConversationId: id})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			s, _, owner, _ := ordinaryPolicyFixture(t)
			stale := owner
			stale.Epoch = "stale"
			for _, ctx := range []context.Context{t.Context(), foregroundBinding(t.Context(), stale)} {
				if err := call(s, ctx, owner.Conversation); err == nil {
					t.Fatal("unbound/stale authority accepted")
				}
			}
			p, ctx, owner, _ := preparedWakeFixture(t)
			if err := call(p, foregroundBinding(ctx, owner), owner.Conversation); err == nil {
				t.Fatal("prepared operation enabled")
			}
		})
	}
}

func TestOrdinaryGenerationRolloverAtomicFailureBlockers(t *testing.T) {
	s, ctx, owner, cfgPath := ordinaryPolicyFixture(t)
	s.rollover = newRolloverManager(rolloverConfig{RawTokenThreshold: 1000})
	// Fail the final handoff insertion, after the conversation and route writes.
	db, err := sql.Open("sqlite", filepath.Join(filepath.Dir(cfgPath), "conversations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TRIGGER fail_rollover BEFORE INSERT ON turns BEGIN SELECT RAISE(ABORT, 'rollover handoff probe'); END`); err != nil {
		t.Fatal(err)
	}
	offer := s.rollover.NoteOffered(owner.Conversation, 1000)
	r, err := s.AcceptRollover(ctx, &proto.AcceptRolloverRequest{ConversationId: owner.Conversation, OfferId: offer})
	if err != nil || r.GetOk() || r.GetError() == "" {
		t.Fatalf("expected persisted creation failure: %v %v", r, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM conversations WHERE precursor_id = ?`, owner.Conversation).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial rollover: %d %v", count, err)
	}
	// The source foreground failure and destination acquisition failure both
	// survive deferred completion. Neither source release nor host retirement wins.
	if err := s.ReleaseRetained(t.Context(), owner, nil); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("source failure ticket lost: %v", err)
	}
	if err := s.RetireCoexistence(t.Context()); !errors.Is(err, broker.ErrAdmissionBusy) {
		t.Fatalf("host failure ticket lost: %v", err)
	}
}
