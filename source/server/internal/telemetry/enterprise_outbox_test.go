package telemetry

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/usage"
)

func TestEnterpriseOutboxCrashReplayAndScopedReceipts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telemetry.db")
	open := func() *SQLiteStore {
		s, err := NewSQLiteStore(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.InitializeAccounting(t.Context()); err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	one := accountingFixture("one")
	two := accountingFixture("two")
	two.SourceConnectionID = "source-subscription"
	if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{one, two}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{one, two}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = open() // crash after local commit, before remote response/ack
	defer s.Close()
	if foreign, err := s.PendingEnterpriseUsage(t.Context(), "foreign", "host", 64); err != nil || len(foreign) != 0 {
		t.Fatalf("foreign scope saw events: %d %v", len(foreign), err)
	}
	pending, err := s.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
	if err != nil || len(pending) != 2 {
		t.Fatalf("persisted events: %+v %v", pending, err)
	}
	bad := []EnterpriseUsageAck{{Index: 0, AttemptID: "wrong", Revision: pending[0].Revision, Status: "accepted"}, {Index: 1, AttemptID: pending[1].AttemptID, Revision: pending[1].Revision, Status: "accepted"}}
	if err := s.ApplyEnterpriseUsageAcks(t.Context(), "org", "host", pending, bad); err == nil {
		t.Fatal("wrong identity retired outbox")
	}
	if retry, err := s.PendingEnterpriseUsage(t.Context(), "org", "host", 64); err != nil || len(retry) != 2 {
		t.Fatalf("failed ack changed outbox: %d %v", len(retry), err)
	}
	acks := []EnterpriseUsageAck{{Index: 0, AttemptID: pending[0].AttemptID, Revision: pending[0].Revision, Status: "accepted"}, {Index: 1, AttemptID: pending[1].AttemptID, Revision: pending[1].Revision, Status: "rejected", ErrorCode: "unknown_source"}}
	if err := s.ApplyEnterpriseUsageAcks(t.Context(), "org", "host", pending, acks); err != nil {
		t.Fatal(err)
	}
	if retry, err := s.PendingEnterpriseUsage(t.Context(), "org", "host", 64); err != nil || len(retry) != 0 {
		t.Fatalf("accepted/rejected rows retried: %d %v", len(retry), err)
	}
	var errorCode string
	if err := s.db.QueryRow(`SELECT last_error FROM enterprise_usage_outbox WHERE attempt_id=?`, pending[1].AttemptID).Scan(&errorCode); err != nil || errorCode != "unknown_source" {
		t.Fatalf("rejection not retained: %q %v", errorCode, err)
	}
}

func TestManagedConversionFailurePreservesLocalAttempt(t *testing.T) {
	s := accountingTestStore(t)
	a := accountingFixture("custom-anthropic")
	a.Provider = ""
	a.RequestedProvider = ""
	if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{a}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.AccountingAttempt(t.Context(), a.ID)
	if err != nil || stored.ID != a.ID {
		t.Fatalf("local attempt was rolled back: %+v %v", stored, err)
	}
	pending, err := s.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
	if err != nil || len(pending) != 0 {
		t.Fatalf("invalid event was uploaded: %+v %v", pending, err)
	}
	var code string
	if err := s.db.QueryRowContext(t.Context(), `SELECT last_error FROM enterprise_usage_outbox WHERE attempt_id=?`, a.ID).Scan(&code); err != nil || code != "conversion_failed" {
		t.Fatalf("unreportable attempt not inspectable: %q %v", code, err)
	}
}

func TestManagedCustomAnthropicReportsSignedProvider(t *testing.T) {
	s := accountingTestStore(t)
	a := accountingFixture("custom-anthropic-signed")
	a.Provider = ""
	a.RequestedProvider = "anthropic"
	a.RequestedModel = "claude"
	if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{a}); err != nil {
		t.Fatal(err)
	}
	pending, err := s.PendingEnterpriseUsage(t.Context(), "org", "host", 64)
	if err != nil || len(pending) != 1 {
		t.Fatalf("signed provider event missing: %+v %v", pending, err)
	}
	var event struct {
		Provider string `json:"provider"`
		Source   string `json:"source_connection_id"`
	}
	if err := json.Unmarshal(pending[0].Payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.Provider != "anthropic" || event.Source != "source-api" {
		t.Fatalf("adapter label replaced signed identity: %+v", event)
	}
}
