package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/compaction"
	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/contextmeter"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/llm"
)

// Return the actual store error without ReportWriteFailure. Unknown commit
// outcome is simulated AFTER a real committed SQLite write, not by a ticket.
type compactionOutcomeStore struct {
	conversation.Store
	mode   string
	writes int
}

func (s *compactionOutcomeStore) SaveCompaction(ctx context.Context, c conversation.Compaction) error {
	s.writes++
	if s.mode == "ownership" {
		ctx = context.Background()
	}
	err := s.Store.SaveCompaction(ctx, c)
	if err == nil && s.mode == "commit_uncertain" {
		return errors.New("commit acknowledgement lost")
	}
	return err
}

func TestCompactionInferenceAndSQLiteOutcomes(t *testing.T) {
	for _, mode := range []string{"cancel", "provider", "success", "sqlite_abort", "ownership", "commit_uncertain", "cancel_with_write_failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "history.db")
			if err := conversation.PrepareCoexistence(ctx, path); err != nil {
				t.Fatal(err)
			}
			st, err := conversation.OpenCoexistence(ctx, path, conversation.CurrentCoexistenceDeclaration())
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			cap, err := conversation.AcquireOwnership(ctx, st, "c", "host")
			if err != nil {
				t.Fatal(err)
			}
			defer cap.Close()
			owned := conversation.WithOwnership(ctx, cap)
			if err := st.EnsureConversation(owned, "c", "", ""); err != nil {
				t.Fatal(err)
			}
			prior := conversation.Compaction{ConversationID: "c", FrozenThrough: 1, ConsolidatedJSON: `{"goal":"durable prior summary"}`}
			if err := st.SaveCompaction(owned, prior); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 12; i++ {
				if err := st.Append(owned, conversation.Turn{ID: fmt.Sprint(i), ConversationID: "c", Role: "user", Content: strings.Repeat("context evidence ", 500), CreatedAt: time.Unix(int64(100+i), 0)}); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "sqlite_abort" {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err := db.Exec("CREATE TRIGGER reject_compaction BEFORE INSERT ON conversation_compaction BEGIN SELECT RAISE(ABORT, 'compaction write rejected'); END"); err != nil {
					t.Fatal(err)
				}
			}
			owner := broker.Owner{Conversation: "c", Host: "host", Epoch: cap.Epoch()}
			a := broker.NewAdmission("host")
			if err := a.RegisterOwner(owner); err != nil {
				t.Fatal(err)
			}
			store := &compactionOutcomeStore{Store: st, mode: mode}
			g := compactiongen.New(store, func(ctx context.Context, _ []llm.Message) (compaction.StructuredSummary, error) {
				if mode == "cancel_with_write_failure" {
					// Independent real ownership refusal during the inference callback.
					ReportWriteFailure(ctx, st.SaveContextUsage(context.Background(), conversation.ContextUsage{ConversationID: "c", TokensUsed: 10}))
				}
				if mode == "cancel" || mode == "cancel_with_write_failure" {
					return compaction.StructuredSummary{}, context.Canceled
				}
				if mode == "provider" {
					return compaction.StructuredSummary{}, errors.New("provider failed")
				}
				return compaction.StructuredSummary{Goal: "g"}, nil
			}, compactor.Config{ActivationFloorTokens: 100, SegmentTokens: 1000, VerbatimRecent: 2}, contextmeter.Default(), 0)
			g.SetEnabled(true)
			defer g.Close(context.Background())
			w, err := ScheduleOwnedCompaction(ctx, g, owner, func(ctx context.Context, o broker.Owner, k broker.WorkKind) (context.Context, *broker.WorkTicket, error) {
				ticket, err := a.AdmitWork(o, k)
				return conversation.WithOwnership(ctx, cap), ticket, err
			})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-w.Done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			fenced := mode != "cancel" && mode != "provider" && mode != "success"
			if (a.Snapshot(owner).UnresolvedWork > 0) != fenced {
				t.Fatalf("snapshot=%+v err=%v", a.Snapshot(owner), w.Err)
			}
			if mode != "success" && w.Err == nil {
				t.Fatal("lost failure")
			}
			if mode == "success" && w.Err != nil {
				t.Fatal(w.Err)
			}
			state, err := st.GetCompaction(owned, "c")
			if err != nil {
				t.Fatal(err)
			}
			committed := mode == "success" || mode == "commit_uncertain"
			if (state.FrozenThrough > prior.FrozenThrough) != committed {
				t.Fatalf("durable state=%+v", state)
			}
			if mode == "cancel" || mode == "provider" || mode == "cancel_with_write_failure" {
				if state.ConsolidatedJSON != prior.ConsolidatedJSON || state.SegmentSummariesJSON != prior.SegmentSummariesJSON || state.FrozenThrough != prior.FrozenThrough {
					t.Fatalf("prior summary changed: %+v", state)
				}
				if store.writes != 0 {
					t.Fatal("unexpected write attempt")
				}
			}
			_, err = a.CloseOwner(owner, nil)
			if fenced {
				if !errors.Is(err, broker.ErrAdmissionBusy) {
					t.Fatalf("failure released: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
