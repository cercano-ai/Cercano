package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"

	"cercano/source/server/internal/updatecoord/operation"
)

// AcknowledgeActivationHealth conditionally advances ONLY selected ->
// health-verified. The operation remains in health-check; this is not lifecycle
// completion. Both authority and journal CAS/facts are checked in one write
// transaction. Callers must separately prove selection and owned-process health
// under installation exclusion; database state cannot prove filesystem effects.
func (s *Store) AcknowledgeActivationHealth(ctx context.Context, operationRevision, journalRevision int64, selected ActivationJournal) (int64, error) {
	if operationRevision < 1 || journalRevision < 1 || journalRevision == math.MaxInt64 || selected.Checkpoint != JournalSelected {
		return 0, ErrInvalidRecord
	}
	if err := ValidateActivationJournal(selected, s.installID); err != nil {
		return 0, err
	}
	next := selected
	next.Checkpoint = JournalHealthVerified
	payload, err := json.Marshal(next)
	if err != nil {
		return 0, err
	}
	var revision int64
	err = s.inWriteTx(ctx, func(ctx context.Context, conn *sql.Conn) error {
		op, rev, found, err := s.loadCurrentOperation(ctx, conn)
		if err != nil {
			return err
		}
		if !found || op.ID != selected.OpID {
			return ErrNotCurrentOperation
		}
		if rev != operationRevision {
			return ErrStaleRevision
		}
		if op.State != operation.StateHealthCheck || op.RecoveryRequested || op.RecoveryNeeded || op.TargetVersion != selected.TargetVersion {
			return ErrInvalidRecord
		}
		// The existing conn-scoped primitive checks journal revision, immutable
		// facts and legal predecessor in this SAME transaction. Health-verified
		// has exactly one predecessor: selected. It also supplies write fault tests.
		revision, err = s.saveActivationJournalConn(ctx, conn, journalRevision, next, payload)
		return err
	})
	if err != nil {
		return 0, err
	}
	return revision, nil
}
