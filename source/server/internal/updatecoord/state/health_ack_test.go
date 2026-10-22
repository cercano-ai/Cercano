package state

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/internal/updatecoord/operation"
)

func TestHealthAcknowledgementTransaction(t *testing.T) {
	for _, scenario := range []string{"persist", "write-failure", "operation-revision", "journal-revision", "journal-facts", "recovery", "recovery-needed", "lifecycle", "wrong-id"} {
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
			for _, cp := range []JournalCheckpoint{JournalPrepared, JournalSwitchIntent, JournalSelected} {
				j.Checkpoint = cp
				jr, err = s.SaveActivationJournal(ctx, jr, j)
				if err != nil {
					t.Fatal(err)
				}
			}
			record, rev, err := s.LoadOperationRecord(ctx, op.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "write-failure":
				s.fault = func() error { return errors.New("injected write failure") }
			case "operation-revision":
				rev--
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
			got, err := s.AcknowledgeActivationHealth(ctx, rev, jr, j)
			if scenario == "persist" {
				if err != nil || got != 4 {
					t.Fatalf("%d %v", got, err)
				}
			} else if err == nil || got != 0 {
				t.Fatalf("refusal %d %v", got, err)
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
			want := JournalSelected
			wantRev := int64(3)
			if scenario == "persist" {
				want = JournalHealthVerified
				wantRev = 4
			}
			if saved.Checkpoint != want || savedRev != wantRev {
				t.Fatalf("reopened %+v %d", saved, savedRev)
			}
			unchanged, actualRev, err := s.LoadOperationRecord(ctx, op.ID)
			if err != nil || unchanged != record {
				t.Fatalf("operation mutated: %+v %v", unchanged, err)
			}
			if scenario == "persist" && actualRev != rev {
				t.Fatal("operation revision advanced")
			}
		})
	}
}
