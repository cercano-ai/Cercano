package main

import (
	"bytes"
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
// It reports progress only after receipts are persisted and at least one sent
// event is no longer eligible; a successful call alone does not imply progress.
func sendPendingEnterpriseUsage(ctx context.Context, store usageOutbox, sender usageSender) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	status := sender.Status()
	if !status.Managed || !status.Connected || status.OrganizationID == "" || status.HostID == "" {
		return false, nil
	}
	org, host := status.OrganizationID, status.HostID
	sent, err := store.PendingEnterpriseUsage(ctx, org, host, 64)
	if err != nil || len(sent) == 0 {
		return false, err
	}
	// The server caps the whole request at 1 MiB. Keep the content-free event
	// portion below half that limit so envelope overhead cannot turn a valid
	// persisted event into an indefinitely rejected oversized batch.
	events := make([]json.RawMessage, 0, len(sent))
	payloadBytes := 0
	for _, e := range sent {
		if len(events) > 0 && payloadBytes+len(e.Payload)+1 > 512<<10 {
			break
		}
		payloadBytes += len(e.Payload) + 1
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
		return false, err
	}
	digest := sha256.Sum256(identity)
	batchID := hex.EncodeToString(digest[:])
	response, err := sender.ReportUsage(ctx, org, host, batchID, events)
	if err != nil {
		return false, err
	}
	if response.BatchID != batchID || response.SchemaVersion != "cercano.usage-report.v2" || len(response.Acknowledgments) != len(sent) {
		return false, errors.New("enterprise usage response mismatch")
	}
	acks := make([]telemetry.EnterpriseUsageAck, len(response.Acknowledgments))
	for i, ack := range response.Acknowledgments {
		acks[i] = telemetry.EnterpriseUsageAck{Index: ack.Index, AttemptID: ack.AttemptID, Revision: ack.Revision, Status: ack.Status, ErrorCode: ack.ErrorCode}
	}
	if err := store.ApplyEnterpriseUsageAcks(ctx, org, host, sent, acks); err != nil {
		return false, err
	}
	// Inspect eligibility rather than inferring it from receipt statuses:
	// rejected rows are retained but excluded, while a no-op acknowledgment
	// must not cause an immediate retry of the same durable events.
	pending, err := store.PendingEnterpriseUsage(ctx, org, host, 64)
	if err != nil {
		return false, err
	}
	for _, event := range sent {
		stillPending := false
		for _, next := range pending {
			if event.AttemptID == next.AttemptID && event.Revision == next.Revision && bytes.Equal(event.Payload, next.Payload) {
				stillPending = true
				break
			}
		}
		if !stillPending {
			return true, nil
		}
	}
	return false, nil
}

func runEnterpriseUsage(ctx context.Context, store usageOutbox, sender usageSender) {
	// Uploads are independent of inference. Failures remain in the outbox and
	// are retried at the next interval or process restart.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		call, cancel := context.WithTimeout(ctx, enterprise.UsageReportTimeout)
		progress, err := sendPendingEnterpriseUsage(call, store, sender)
		cancel()
		if err == nil && progress && ctx.Err() == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
