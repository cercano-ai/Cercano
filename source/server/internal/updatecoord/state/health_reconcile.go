package state

import (
	"context"
	"database/sql"

	"cercano/source/server/internal/updatecoord/operation"
)

// HealthReconciliation is durable lifecycle reconciliation, never a fresh health
// observation. AlreadyReconciled means no write occurred.
type HealthReconciliation struct {
	AlreadyReconciled bool
	OperationRevision int64
	JournalRevision   int64
}

// ReconcileActivationHealth consumes a previously durable health acknowledgement.
// Callers must validate exact selection and persisted manifest/image facts under
// directory-bound exclusion. All database authority is rechecked in ONE write
// transaction; the journal remains health-verified, including on duplicate calls.
func (s *Store) ReconcileActivationHealth(ctx context.Context, operationRevision, journalRevision int64, acknowledged ActivationJournal) (HealthReconciliation, error) {
	if operationRevision < 1 || journalRevision < 1 || acknowledged.Checkpoint != JournalHealthVerified {
		return HealthReconciliation{}, ErrInvalidRecord
	}
	if err := ValidateActivationJournal(acknowledged, s.installID); err != nil {
		return HealthReconciliation{}, err
	}
	var result HealthReconciliation
	err := s.inWriteTx(ctx, func(ctx context.Context, conn *sql.Conn) error {
		op, rev, found, err := s.loadCurrentOperation(ctx, conn)
		if err != nil {
			return err
		}
		if !found || op.ID != acknowledged.OpID {
			return ErrNotCurrentOperation
		}
		if rev != operationRevision {
			return ErrStaleRevision
		}
		if op.TargetVersion != acknowledged.TargetVersion || op.RecoveryRequested || op.RecoveryNeeded {
			return ErrInvalidRecord
		}
		j, jr, err := s.loadActivationJournalConn(ctx, conn, op.ID)
		if err != nil {
			return err
		}
		if jr != journalRevision {
			return ErrStaleRevision
		}
		if j.Checkpoint != JournalHealthVerified || j.facts() != acknowledged.facts() {
			return ErrInvalidRecord
		}
		restored, err := restoreOperationRecord(op, true)
		if err != nil {
			return err
		}
		result = HealthReconciliation{OperationRevision: rev, JournalRevision: jr}
		if op.State == operation.StateCleanup && op.HealthVerified {
			result.AlreadyReconciled = true
			return nil
		}
		if op.State != operation.StateHealthCheck || op.HealthVerified {
			return ErrInvalidRecord
		}
		model := operation.NewStore()
		if err := model.Restore(restored); err != nil {
			return err
		}
		next, err := model.Apply(s.installID, operation.Input{OperationID: op.ID, Event: operation.EventHealthSuccess})
		if err != nil {
			return err
		}
		if err := NewAdapter(s).persist(ctx, conn, rev, next); err != nil {
			return err
		}
		result.OperationRevision++
		return nil
	})
	if err != nil {
		return HealthReconciliation{}, err
	}
	return result, nil
}
