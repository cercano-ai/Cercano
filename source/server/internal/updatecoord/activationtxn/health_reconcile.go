package activationtxn

import (
	"context"
	"errors"
	"path/filepath"

	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/internal/updatecoord/state"
)

// ReconcileHealth advances only the operation lifecycle from a durable
// health-verified journal to cleanup. It observes no candidate and performs no
// launch, adoption, stop, cleanup, rollback or overall completion. A matching
// retry explicitly returns AlreadyReconciled, not a new health observation.
// Request inputs must remain immutable during the call.
func ReconcileHealth(ctx context.Context, req Request) (state.HealthReconciliation, error) {
	if ctx == nil {
		return state.HealthReconciliation{}, ErrInvalidRequest
	}
	if err := validateRequest(req); err != nil {
		return state.HealthReconciliation{}, err
	}
	var result state.HealthReconciliation
	ran := false
	err := req.Lock.GuardUpdateSession(req.Directory, func(_ exclusion.GuardSession) error {
		ran = true
		if err := ctx.Err(); err != nil {
			return err
		}
		snap, found, err := state.NewAdapter(req.Store).Snapshot(ctx)
		if err != nil {
			return err
		}
		if !found || snap.ID != req.OpID {
			return ErrStaleOperation
		}
		op, opRev, err := req.Store.LoadOperationRecord(ctx, req.OpID)
		if err != nil {
			return err
		}
		j, jRev, err := req.Store.LoadActivationJournal(ctx, req.OpID)
		if err != nil {
			return err
		}
		if op.ID != j.OpID || op.TargetVersion != j.TargetVersion || op.RecoveryRequested || op.RecoveryNeeded || j.Checkpoint != state.JournalHealthVerified {
			return ErrHealthState
		}
		if !(op.State == operation.StateHealthCheck && !op.HealthVerified) && !(op.State == operation.StateCleanup && op.HealthVerified) {
			return ErrHealthState
		}
		target, err := targetSelection(j)
		if err != nil {
			return err
		}
		actual, err := activation.ReadSelection(filepath.Join(req.Directory, selection.FileName))
		if err != nil {
			return errors.Join(ErrReadbackMismatch, err)
		}
		if actual != target {
			return ErrReadbackMismatch
		}
		if err := verifyAndBindTargetImages(ctx, req, j); err != nil {
			return err
		}
		result, err = req.Store.ReconcileActivationHealth(ctx, opRev, jRev, j)
		return err
	})
	if err != nil {
		if !ran {
			err = errors.Join(ErrInvalidRequest, err)
		}
		return state.HealthReconciliation{}, err
	}
	return result, nil
}
