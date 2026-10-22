package state

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/internal/updatecoord/operation"
)

func TestHealthReconciliationTransaction(t *testing.T) {
	for _, scenario := range []string{"persist", "write-failure", "operation-revision", "journal-revision", "journal-facts", "recovery", "recovery-needed", "lifecycle", "wrong-id", "recovery-race", "journal-race", "duplicate-race"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			s, err := Open(root, "test-install")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			op := startCurrentOperation(t, s, "9.9.9")
			for _, e := range []operation.Event{operation.EventReady, operation.EventDownload, operation.EventDownloaded, operation.EventVerified, operation.EventDrain, operation.EventDrained, operation.EventInstalled, operation.EventRestarted} {
				if _, err := NewAdapter(s).Apply(ctx, operation.Input{OperationID: op.ID, Event: e}); err != nil {
					t.Fatal(err)
				}
			}
			j := journalFor(op.ID, "9.9.9")
			var jr int64
			for _, cp := range []JournalCheckpoint{JournalPrepared, JournalSwitchIntent, JournalSelected, JournalHealthVerified} {
				j.Checkpoint = cp
				jr, err = s.SaveActivationJournal(ctx, jr, j)
				if err != nil {
					t.Fatal(err)
				}
			}
			// Simulate interruption after acknowledgement, before lifecycle reconciliation.
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(root, "test-install")
			if err != nil {
				t.Fatal(err)
			}
			record, rev, err := s.LoadOperationRecord(ctx, op.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "write-failure":
				s.fault = func() error { return errors.New("injected write failure") }
			case "operation-revision", "recovery-race":
				other, err := Open(root, "test-install")
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "recovery-race" {
					record.RecoveryRequested = true
				}
				_, err = other.SaveOperationRecord(ctx, rev, record)
				other.Close()
				if err != nil {
					t.Fatal(err)
				}
			case "journal-race":
				next := j
				next.Checkpoint = JournalCleanupPending
				if _, err := s.SaveActivationJournal(ctx, jr, next); err != nil {
					t.Fatal(err)
				}
			case "duplicate-race":
				other, err := Open(root, "test-install")
				if err != nil {
					t.Fatal(err)
				}
				_, err = other.ReconcileActivationHealth(ctx, rev, jr, j)
				other.Close()
				if err != nil {
					t.Fatal(err)
				}
				record, _, err = s.LoadOperationRecord(ctx, op.ID)
				if err != nil {
					t.Fatal(err)
				}
			case "journal-revision":
				jr--
			case "journal-facts":
				j.StagedVersionDir = "other"
			case "wrong-id":
				j.OpID++
			case "recovery", "recovery-needed":
				record.RecoveryRequested = scenario == "recovery"
				record.RecoveryNeeded = scenario == "recovery-needed"
				rev, err = s.SaveOperationRecord(ctx, rev, record)
				if err != nil {
					t.Fatal(err)
				}
			case "lifecycle":
				record.State = operation.StateRestarting
				rev, err = s.SaveOperationRecord(ctx, rev, record)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, persistedRev, err := s.LoadOperationRecord(ctx, op.ID)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.ReconcileActivationHealth(ctx, rev, jr, j)
			if scenario == "persist" {
				if err != nil || got.OperationRevision != rev+1 || got.AlreadyReconciled || got.JournalRevision != 4 {
					t.Fatalf("%+v %v", got, err)
				}
			} else if err == nil || got != (HealthReconciliation{}) {
				t.Fatalf("refusal %+v %v", got, err)
			}
			s.fault = nil
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(root, "test-install")
			if err != nil {
				t.Fatal(err)
			}
			saved, savedRev, err := s.LoadActivationJournal(ctx, op.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantCP, wantRev := JournalHealthVerified, int64(4)
			if scenario == "journal-race" {
				wantCP, wantRev = JournalCleanupPending, 5
			}
			if saved.Checkpoint != wantCP || savedRev != wantRev {
				t.Fatalf("journal changed: %+v %d", saved, savedRev)
			}
			unchanged, actualRev, err := s.LoadOperationRecord(ctx, op.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "persist" {
				if unchanged.State != operation.StateCleanup || !unchanged.HealthVerified || actualRev != rev+1 {
					t.Fatalf("operation %+v rev %d", unchanged, actualRev)
				}
				for i := 0; i < 2; i++ {
					retry, err := s.ReconcileActivationHealth(ctx, actualRev, savedRev, saved)
					if err != nil || !retry.AlreadyReconciled || retry.OperationRevision != actualRev {
						t.Fatalf("retry %+v %v", retry, err)
					}
				}
			} else if unchanged != record || actualRev != persistedRev {
				t.Fatalf("operation mutated: %+v", unchanged)
			}

		})
	}
}
