package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"cercano/source/server/internal/enterprise"
	"cercano/source/server/internal/telemetry"
)

type usageOutbox interface {
	PendingEnterpriseUsage(context.Context, string, string, int) ([]telemetry.PendingEnterpriseEvent, error)
	ApplyEnterpriseUsageAcks(context.Context, string, string, []telemetry.PendingEnterpriseEvent, []telemetry.EnterpriseUsageAck) error
}

type usageSender interface {
	Status() enterprise.HostStatus
	ReportUsage(context.Context, string, string, string, []json.RawMessage) (enterprise.UsageResponse, error)
}

// sendPendingEnterpriseUsage never guesses source identity from routing or
// credentials. A network failure leaves the exact durable events for replay.
func sendPendingEnterpriseUsage(ctx context.Context, store usageOutbox, sender usageSender) error {
	status := sender.Status()
	if !status.Managed || !status.Connected || status.OrganizationID == "" || status.HostID == "" {
		return nil
	}
	org, host := status.OrganizationID, status.HostID
	sent, err := store.PendingEnterpriseUsage(ctx, org, host, 64)
	if err != nil || len(sent) == 0 {
		return err
	}
	// The server caps the whole request at 1 MiB. Keep the content-free event
	// portion below half that limit so envelope overhead cannot turn a valid
	// persisted event into an indefinitely rejected oversized batch.
	events := make([]json.RawMessage, 0, len(sent))
	bytes := 0
	for _, e := range sent {
		if len(events) > 0 && bytes+len(e.Payload)+1 > 512<<10 {
			break
		}
		bytes += len(e.Payload) + 1
		events = append(events, e.Payload)
	}
	sent = sent[:len(events)]
	// The batch ID is stable for identical bytes and scope after a lost reply.
	identity, err := json.Marshal(struct {
		Org    string            `json:"org"`
		Host   string            `json:"host"`
		Events []json.RawMessage `json:"events"`
	}{org, host, events})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(identity)
	batchID := hex.EncodeToString(digest[:])
	response, err := sender.ReportUsage(ctx, org, host, batchID, events)
	if err != nil {
		return err
	}
	if response.BatchID != batchID || response.SchemaVersion != "cercano.usage-report.v2" || len(response.Acknowledgments) != len(sent) {
		return errors.New("enterprise usage response mismatch")
	}
	acks := make([]telemetry.EnterpriseUsageAck, len(response.Acknowledgments))
	for i, ack := range response.Acknowledgments {
		acks[i] = telemetry.EnterpriseUsageAck{Index: ack.Index, AttemptID: ack.AttemptID, Revision: ack.Revision, Status: ack.Status, ErrorCode: ack.ErrorCode}
	}
	return store.ApplyEnterpriseUsageAcks(ctx, org, host, sent, acks)
}

func runEnterpriseUsage(ctx context.Context, store usageOutbox, sender usageSender) {
	// Uploads are independent of inference. Failures remain in the outbox and
	// are retried at the next interval or process restart.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		call, cancel := context.WithTimeout(ctx, enterprise.UsageReportTimeout)
		_ = sendPendingEnterpriseUsage(call, store, sender)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
