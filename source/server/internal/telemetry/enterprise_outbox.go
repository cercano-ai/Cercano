package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cercano/source/server/internal/usage"
)

type PendingEnterpriseEvent struct {
	AttemptID string
	Revision  uint64
	Payload   json.RawMessage
}

type EnterpriseUsageAck struct {
	Index     int    `json:"index"`
	AttemptID string `json:"attempt_id"`
	Revision  uint64 `json:"revision"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
}

func insertEnterpriseOutbox(ctx context.Context, tx *sql.Tx, a usage.AttemptObservation) error {
	raw, managed, conversionErr := usage.ReportEventV2(a)
	if !managed && conversionErr == nil {
		return nil
	}
	lastError := ""
	if conversionErr != nil || len(raw) > 16<<10 {
		// A conversion failure is a reporting gap, not permission to roll
		// back valid local physical-attempt history. Keep a fixed, inspectable
		// rejected row and never upload a guessed event.
		raw = json.RawMessage(`{}`)
		lastError = "conversion_failed"
	}
	var prior string
	err := tx.QueryRowContext(ctx, `SELECT event_json FROM enterprise_usage_outbox WHERE organization_id=? AND host_id=? AND attempt_id=? AND revision=?`, a.Report.OrganizationID, a.Report.HostID, a.ID, a.Revision).Scan(&prior)
	if err == nil {
		if prior != string(raw) {
			return errors.New("enterprise usage revision changed after persistence")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO enterprise_usage_outbox(organization_id,host_id,attempt_id,revision,event_json,last_error,created_at) VALUES(?,?,?,?,?,?,?)`, a.Report.OrganizationID, a.Report.HostID, a.ID, a.Revision, string(raw), lastError, time.Now().UTC().UnixMicro())
	return err
}

// PendingEnterpriseUsage returns only rows belonging to the currently
// authenticated organization and host. Rejected rows stay persisted for manual
// reconciliation and are not retried blindly.
func (s *SQLiteStore) PendingEnterpriseUsage(ctx context.Context, org, host string, limit int) ([]PendingEnterpriseEvent, error) {
	if org == "" || host == "" || limit < 1 || limit > 64 {
		return nil, errors.New("invalid enterprise outbox scope or limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT attempt_id,revision,event_json FROM enterprise_usage_outbox WHERE organization_id=? AND host_id=? AND last_error='' ORDER BY created_at,attempt_id,revision LIMIT ?`, org, host, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PendingEnterpriseEvent{}
	for rows.Next() {
		var e PendingEnterpriseEvent
		var raw string
		if err := rows.Scan(&e.AttemptID, &e.Revision, &raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 || len(raw) > 16<<10 || !json.Valid([]byte(raw)) {
			return nil, errors.New("corrupt enterprise outbox event")
		}
		e.Payload = json.RawMessage(raw)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ApplyEnterpriseUsageAcks retires only exact accepted receipts. A rejected
// event remains inspectable with its fixed server error code, and a missing or
// mismatched acknowledgment cannot delete another event's durable identity.
func (s *SQLiteStore) ApplyEnterpriseUsageAcks(ctx context.Context, org, host string, sent []PendingEnterpriseEvent, acks []EnterpriseUsageAck) error {
	if org == "" || host == "" || len(sent) == 0 || len(sent) != len(acks) {
		return errors.New("invalid enterprise usage acknowledgments")
	}
	seen := make([]bool, len(sent))
	for _, ack := range acks {
		if ack.Index < 0 || ack.Index >= len(sent) || seen[ack.Index] {
			return errors.New("duplicate or out-of-range enterprise acknowledgment")
		}
		seen[ack.Index] = true
		e := sent[ack.Index]
		if ack.AttemptID != e.AttemptID || ack.Revision != e.Revision {
			return errors.New("enterprise acknowledgment identity mismatch")
		}
		if ack.Status != "accepted" && ack.Status != "already_accepted" && ack.Status != "rejected" {
			return errors.New("unknown enterprise acknowledgment status")
		}
		if ack.Status == "rejected" && (ack.ErrorCode == "" || len(ack.ErrorCode) > 64) {
			return errors.New("invalid enterprise rejection code")
		}
		if ack.Status != "rejected" && ack.ErrorCode != "" {
			return errors.New("accepted enterprise receipt has error")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ack := range acks {
		e := sent[ack.Index]
		if ack.Status == "rejected" {
			_, err = tx.ExecContext(ctx, `UPDATE enterprise_usage_outbox SET last_error=? WHERE organization_id=? AND host_id=? AND attempt_id=? AND revision=? AND event_json=?`, ack.ErrorCode, org, host, e.AttemptID, e.Revision, string(e.Payload))
		} else {
			_, err = tx.ExecContext(ctx, `DELETE FROM enterprise_usage_outbox WHERE organization_id=? AND host_id=? AND attempt_id=? AND revision=? AND event_json=?`, org, host, e.AttemptID, e.Revision, string(e.Payload))
		}
		if err != nil {
			return fmt.Errorf("persist enterprise usage receipt: %w", err)
		}
	}
	return tx.Commit()
}
