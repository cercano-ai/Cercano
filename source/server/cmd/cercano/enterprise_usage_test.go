package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/telemetry"
	"cercano/source/server/internal/usage"
)

type fakeUsageOutbox struct {
	events  []telemetry.PendingEnterpriseEvent
	retired bool
	scope   [2]string
}

func (f *fakeUsageOutbox) PendingEnterpriseUsage(_ context.Context, org, host string, _ int) ([]telemetry.PendingEnterpriseEvent, error) {
	f.scope = [2]string{org, host}
	if f.retired {
		return nil, nil
	}
	return f.events, nil
}
func (f *fakeUsageOutbox) ApplyEnterpriseUsageAcks(_ context.Context, org, host string, sent []telemetry.PendingEnterpriseEvent, acks []telemetry.EnterpriseUsageAck) error {
	if [2]string{org, host} != f.scope || !reflect.DeepEqual(sent, f.events) || len(acks) != len(sent) {
		return errors.New("wrong receipt scope")
	}
	for i, a := range acks {
		if a.Index != i || a.AttemptID != sent[i].AttemptID || a.Revision != sent[i].Revision || a.Status != "accepted" {
			return errors.New("wrong receipt")
		}
	}
	f.retired = true
	return nil
}

type fakeUsageSender struct {
	status     enterprise.HostStatus
	calls      int
	batchIDs   []string
	lastEvents int
	fail       bool
	wrong      bool
	oldSchema  bool
}

func (f *fakeUsageSender) Status() enterprise.HostStatus { return f.status }
func (f *fakeUsageSender) ReportUsage(_ context.Context, _, _, batch string, events []json.RawMessage) (enterprise.UsageResponse, error) {
	f.calls++
	f.lastEvents = len(events)
	f.batchIDs = append(f.batchIDs, batch)
	if f.fail {
		return enterprise.UsageResponse{}, errors.New("lost response")
	}
	out := enterprise.UsageResponse{SchemaVersion: "cercano.usage-report.v2", BatchID: batch}
	if f.oldSchema {
		out.SchemaVersion = "cercano.usage-report.v1"
	}
	if f.wrong {
		out.BatchID = "different"
	}
	for i := range events {
		out.Acknowledgments = append(out.Acknowledgments, enterprise.UsageReceipt{Index: i, AttemptID: "attempt", Revision: 1, Status: "accepted"})
	}
	return out, nil
}

func TestEnterpriseUsageBoundsWholeRequest(t *testing.T) {
	payload := json.RawMessage(`{"blob":"` + strings.Repeat("x", 12000) + `"}`)
	rows := make([]telemetry.PendingEnterpriseEvent, 64)
	for i := range rows {
		rows[i] = telemetry.PendingEnterpriseEvent{AttemptID: "attempt", Revision: 1, Payload: payload}
	}
	outbox := &fakeUsageOutbox{events: rows}
	sender := &fakeUsageSender{status: enterprise.HostStatus{Managed: true, Status: enterprise.Status{Connected: true, OrganizationID: "org", HostID: "host"}}, fail: true}
	if err := sendPendingEnterpriseUsage(t.Context(), outbox, sender); err == nil {
		t.Fatal("expected synthetic network failure")
	}
	if sender.lastEvents < 1 || sender.lastEvents >= 64 {
		t.Fatalf("whole-request cap did not split batch: %d", sender.lastEvents)
	}
}
func TestEnterpriseUsageLostResponseReplayAndScope(t *testing.T) {
	outbox := &fakeUsageOutbox{events: []telemetry.PendingEnterpriseEvent{{AttemptID: "attempt", Revision: 1, Payload: json.RawMessage(`{"attempt_id":"attempt","revision":1}`)}}}
	sender := &fakeUsageSender{status: enterprise.HostStatus{Managed: true, Status: enterprise.Status{Connected: true, OrganizationID: "org", HostID: "host"}}, fail: true}
	if err := sendPendingEnterpriseUsage(t.Context(), outbox, sender); err == nil || outbox.retired {
		t.Fatal("lost response retired event")
	}
	if outbox.scope != [2]string{"org", "host"} {
		t.Fatalf("wrong scope: %v", outbox.scope)
	}
	sender.fail = false
	sender.wrong = true
	if err := sendPendingEnterpriseUsage(t.Context(), outbox, sender); err == nil || outbox.retired {
		t.Fatal("wrong batch retired event")
	}
	sender.wrong = false
	sender.oldSchema = true
	if err := sendPendingEnterpriseUsage(t.Context(), outbox, sender); err == nil || outbox.retired {
		t.Fatal("old-server reply retired v2 event")
	}
	sender.oldSchema = false
	if err := sendPendingEnterpriseUsage(t.Context(), outbox, sender); err != nil || !outbox.retired {
		t.Fatalf("replay not retired: %v", err)
	}
	if sender.batchIDs[0] != sender.batchIDs[1] || sender.batchIDs[1] != sender.batchIDs[2] || sender.batchIDs[2] != sender.batchIDs[3] {
		t.Fatal("batch identity changed across replay")
	}
	if err := sendPendingEnterpriseUsage(t.Context(), outbox, sender); err != nil || sender.calls != 4 {
		t.Fatalf("empty outbox resent: %v %d", err, sender.calls)
	}
	sender.status.Status.OrganizationID = "foreign"
	if err := sendPendingEnterpriseUsage(t.Context(), outbox, sender); err != nil || outbox.scope != [2]string{"foreign", "host"} {
		t.Fatalf("foreign scope bypass: %v %v", err, outbox.scope)
	}
}

func TestEnterpriseUsageSQLiteRestartAfterLostReply(t *testing.T) {
	const org = "11111111-1111-1111-1111-111111111111"
	const host = "22222222-2222-2222-2222-222222222222"
	path := filepath.Join(t.TempDir(), "usage.db")
	open := func() *telemetry.SQLiteStore {
		store, err := telemetry.NewSQLiteStore(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.InitializeAccounting(t.Context()); err != nil {
			t.Fatal(err)
		}
		return store
	}
	store := open()
	started := time.Now().UTC()
	attempt := usage.AttemptObservation{ID: "attempt", Revision: 1, Provider: "openai", Model: "test-model", RequestedProvider: "openai", RequestedModel: "test-model", RouteID: "route", SourceConnectionID: "connection-a", Report: usage.ReportIdentity{OrganizationID: org, HostID: host, ProfileVersion: 1, TaskType: "chat", RequestedRouteID: "route", RequestedSourceConnectionID: "connection-a"}, Attribution: usage.Attribution{Source: "main"}, StartedAt: started, Outcome: usage.Started}
	if err := store.WriteAttempts(t.Context(), []usage.AttemptObservation{attempt}); err != nil {
		t.Fatal(err)
	}
	sender := &fakeUsageSender{status: enterprise.HostStatus{Managed: true, Status: enterprise.Status{Connected: true, OrganizationID: org, HostID: host}}, fail: true}
	if err := sendPendingEnterpriseUsage(t.Context(), store, sender); err == nil {
		t.Fatal("expected lost response")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = open()
	defer store.Close()
	sender.fail = false
	if err := sendPendingEnterpriseUsage(t.Context(), store, sender); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 2 || sender.batchIDs[0] != sender.batchIDs[1] {
		t.Fatal("durable retry changed batch")
	}
	pending, err := store.PendingEnterpriseUsage(t.Context(), org, host, 64)
	if err != nil || len(pending) != 0 {
		t.Fatalf("receipt not durable: %d %v", len(pending), err)
	}
}
