package telemetry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cercano/source/server/internal/usage"
)

func TestCollectorOwnsAccountingLane(t *testing.T) {
	s := accountingTestStore(t)
	c := NewCollector(s, 8)
	if err := c.EnableAccounting(testAccountingOptions()); err != nil {
		t.Fatal(err)
	}
	first := c.attempts
	if err := c.EnableAccounting(testAccountingOptions()); err != nil || c.attempts != first {
		t.Fatal("accounting initialized twice")
	}
	c.Emit(NewEvent("legacy", "fake"))
	if !c.EmitAttempt(accountingFixture("actual")) {
		t.Fatal("attempt rejected")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := c.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	h := c.AccountingHealth()
	if h.Persisted != 1 || h.Pending != 0 || !h.Closed {
		t.Fatalf("lane not drained: %+v", h)
	}
	stats, err := s.GetStats(t.Context())
	if err != nil || stats.TotalRequests != 1 {
		t.Fatalf("legacy changed: %+v %v", stats, err)
	}
	if _, err = s.AccountingAttempt(t.Context(), "actual"); err != nil {
		t.Fatal(err)
	}
	if c.EmitAttempt(accountingFixture("late")) {
		t.Fatal("accepted after shutdown")
	}
	if err = c.EnableAccounting(testAccountingOptions()); err == nil {
		t.Fatal("enabled closed collector")
	}
}

func TestCollectorCloseContextCanceled(t *testing.T) {
	s := accountingTestStore(t)
	c := NewCollector(s, 8)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Either the empty legacy lane already drained or cancellation wins. Neither
	// path may wait on a timeout or panic; subsequent Close waits for its exit.
	_ = c.CloseContext(ctx)
	c.Close()
}

func TestProducerCoverageGapSurvivesSuccessfulWrites(t *testing.T) {
	s := accountingTestStore(t)
	c := NewCollector(s, 8)
	if err := c.EnableAccounting(testAccountingOptions()); err != nil {
		t.Fatal(err)
	}
	c.MarkAccountingIncomplete("producer shutdown incomplete")
	c.EmitAttempt(accountingFixture("known"))
	c.Close()
	h := c.AccountingHealth()
	if !h.CoverageIncomplete || h.Lost != 0 || h.Persisted != 1 || h.LastError == "" {
		t.Fatalf("coverage gap hidden or fabricated count: %+v", h)
	}
	var raw string
	if err := s.db.QueryRow(`SELECT snapshot FROM accounting_health`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var persisted AccountingHealth
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		t.Fatal(err)
	}
	if !persisted.CoverageIncomplete {
		t.Fatal("coverage gap not persisted")
	}
}

func TestManagedOldWorkerBatchMarksCoverageIncomplete(t *testing.T) {
	s := accountingTestStore(t)
	c := NewCollector(s, 8)
	if err := c.EnableAccounting(testAccountingOptions()); err != nil {
		t.Fatal(err)
	}
	c.SetManagedIdentityRequired(func() bool { return true })
	old := accountingFixture("old-worker")
	old.Report = usage.ReportIdentity{}
	old.RouteID, old.SourceConnectionID = "", ""
	receipt, err := c.TryAttemptBatch([]usage.AttemptObservation{old})
	if err != nil || !<-receipt {
		t.Fatalf("legacy batch not persisted: %v", err)
	}
	c.Close()
	if h := c.AccountingHealth(); !h.CoverageIncomplete {
		t.Fatalf("managed old worker gap hidden: %+v", h)
	}
	if _, err := s.AccountingAttempt(t.Context(), old.ID); err != nil {
		t.Fatalf("old worker local history lost: %v", err)
	}
	var raw string
	if err := s.db.QueryRow(`SELECT snapshot FROM accounting_health`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var persisted AccountingHealth
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil || !persisted.CoverageIncomplete {
		t.Fatalf("old worker gap not durable: %+v %v", persisted, err)
	}
}
