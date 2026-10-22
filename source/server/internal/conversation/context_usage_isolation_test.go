package conversation

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestContextUsage_RequestSurvivesCompactionAndReopen(t *testing.T) {
	for _, requestFirst := range []bool{true, false} {
		name := "history-first"
		if requestFirst {
			name = "request-first"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "usage.db")
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := s.EnsureConversation(ctx, "c", "/p", "m"); err != nil {
				t.Fatal(err)
			}
			request := ContextUsage{ConversationID: "c", Source: "turn", TokensUsed: 864125, MessageTokens: 864125, EstimatedRequest: 887278, ContextWindow: 1050000, Model: "m", ComputedAt: time.Unix(100, 0)}
			history := ContextUsage{ConversationID: "c", Source: "compaction", TokensUsed: 4936958, RawTokens: 7472891, ComputedAt: time.Unix(200, 0)}
			writes := []ContextUsage{history, request}
			if requestFirst {
				writes = []ContextUsage{request, history}
			}
			for _, u := range writes {
				if err := s.SaveContextUsage(ctx, u); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			got, ok, err := s.GetContextUsage(ctx, "c")
			if err != nil || !ok {
				t.Fatalf("get: %v %v", ok, err)
			}
			if got.Source != "turn" || got.EstimatedRequest != 887278 || got.TokensUsed != 864125 || !got.ComputedAt.Equal(request.ComputedAt) {
				t.Fatalf("request replaced by history: %+v", got)
			}
			if got.RawTokens != history.RawTokens {
				t.Fatalf("raw history not preserved: %+v", got)
			}
		})
	}
}

func TestContextUsage_MigratesLegacyRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.EnsureConversation(ctx, "c", "/p", "m"); err != nil {
		t.Fatal(err)
	}
	// Emulate the pre-fix writer, including non-default runtime identity fields.
	_, err = s.(*sqliteStore).db.Exec(`INSERT INTO conversation_context_usage
 (conversation_id,source,tokens_used,estimated_request,computed_at,provider,runtime_instance_id)
 VALUES ('c','turn',864125,887278,100,'llama_server','runtime-1')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// A separate worker connection must not clobber the migrated request.
	worker, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	if err := worker.SaveContextUsage(ctx, ContextUsage{ConversationID: "c", Source: "compaction", TokensUsed: 4900000}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetContextUsage(ctx, "c")
	if err != nil || !ok || got.EstimatedRequest != 887278 || got.Provider != "llama_server" || got.RuntimeInstanceID != "runtime-1" {
		t.Fatalf("lost legacy request: %+v ok=%v err=%v", got, ok, err)
	}
}

// Reopening must not reimport an older legacy row over a current request, even
// when its timestamp is newer. This is the released row-priority contract.
func TestContextUsage_CurrentRequestWinsLegacyImport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-current.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.EnsureConversation(ctx, "c", "/p", "m"); err != nil {
		t.Fatal(err)
	}
	_, err = s.(*sqliteStore).db.Exec(`INSERT INTO conversation_context_usage
 (conversation_id,source,tokens_used,message_tokens,estimated_request,computed_at)
 VALUES ('c','turn',11,11,21,999)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetContextUsage(ctx, "c")
	if err != nil || !ok || got.TokensUsed != 11 {
		t.Fatalf("seed: %+v %v %v", got, ok, err)
	}
	want := ContextUsage{ConversationID: "c", Source: "turn", TokensUsed: 22, MessageTokens: 22, EstimatedRequest: 32, RawTokens: 44, ComputedAt: time.Unix(222, 0)}
	if err := s.SaveContextUsage(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, ok, err = s.GetContextUsage(ctx, "c")
	if err != nil || !ok || got != want {
		t.Fatalf("current request: got=%+v want=%+v ok=%v err=%v", got, want, ok, err)
	}
	// A history-only conversation remains a history result, not a fabricated turn.
	if err := s.EnsureConversation(ctx, "history", "/p", "m"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveContextUsage(ctx, ContextUsage{ConversationID: "history", Source: "compaction", TokensUsed: 55, RawTokens: 66}); err != nil {
		t.Fatal(err)
	}
	got, ok, err = s.GetContextUsage(ctx, "history")
	if err != nil || !ok || got.Source != "compaction" || got.TokensUsed != 55 || got.RawTokens != 66 {
		t.Fatalf("history: %+v %v %v", got, ok, err)
	}
}
