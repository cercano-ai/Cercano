package activationtxn

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"

	"cercano/source/server/internal/updatecoord/activation"
	"cercano/source/server/internal/updatecoord/candidate"
	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/internal/updatecoord/state"
)

// ErrHealthAlreadyRecorded names an intermediate durable checkpoint, NOT a
// successful observation of this candidate. Lifecycle reconciliation is separate.
var ErrHealthAlreadyRecorded = errors.New("activationtxn: health already recorded; lifecycle reconciliation required")

// HealthRequest names an already-owned candidate. Images is trusted packaging
// policy as in Switch; its manifest must equal the persisted manifest. No launch,
// adoption, stop, cleanup, rollback or lifecycle event is performed here.
// Callers must not mutate request inputs during the call.
type HealthRequest struct {
	Request
	Candidate *candidate.Candidate
	Endpoint  netip.AddrPort
}

type HealthResult struct {
	Observation     candidate.Observation
	Acknowledged    bool
	JournalRevision int64
}

// AcknowledgeCandidateHealth holds the directory-bound exclusive guard across
// authoritative reads, image/lifetime binding, bounded observation, revalidation
// and conditional journal write. It never applies EventHealthSuccess. A retry at
// health-verified returns ErrHealthAlreadyRecorded without claiming current
// health. An uncertain observation returns Acknowledged=false and leaves selected
// intact. Observations are not leases on future process survival.
func AcknowledgeCandidateHealth(ctx context.Context, req HealthRequest) (HealthResult, error) {
	return acknowledgeCandidateHealth(ctx, req, req.Candidate.SelectedIdentity, req.Candidate.ObserveHealth)
}

// Inject only observations in package tests; public callers cannot replace the
// owned Candidate methods or manufacture a PID authority.
func acknowledgeCandidateHealth(ctx context.Context, req HealthRequest, identity func() (nativeprocess.Identity, error), observe healthObserver) (HealthResult, error) {
	if ctx == nil {
		return HealthResult{}, ErrInvalidRequest
	}
	if err := validateRequest(req.Request); err != nil {
		return HealthResult{}, err
	}
	var result HealthResult
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
		if op.ID != j.OpID || op.TargetVersion != j.TargetVersion || op.State != operation.StateHealthCheck || op.RecoveryRequested || op.RecoveryNeeded {
			return ErrHealthState
		}
		if j.Checkpoint == state.JournalHealthVerified {
			return ErrHealthAlreadyRecorded
		}
		if j.Checkpoint != state.JournalSelected {
			return ErrHealthState
		}
		target, err := targetSelection(j)
		if err != nil {
			return err
		}
		checkSelection := func() error {
			actual, err := activation.ReadSelection(filepath.Join(req.Directory, selection.FileName))
			if err != nil {
				return errors.Join(ErrReadbackMismatch, err)
			}
			if actual != target {
				return ErrReadbackMismatch
			}
			return nil
		}
		if err := checkSelection(); err != nil {
			return err
		}
		if err := verifyAndBindTargetImages(ctx, req.Request, j); err != nil {
			return err
		}
		selectedPath := filepath.Join(req.Images.VersionsDirectory, j.StagedVersionDir, filepath.FromSlash(req.Images.AgentMember))
		selectedInfo, err := os.Stat(selectedPath)
		if err != nil {
			return err
		}
		bind := func() (nativeprocess.Identity, error) {
			id, err := identity()
			if err != nil {
				return id, err
			}
			if id.Executable != selectedPath {
				return id, candidate.ErrIdentity
			}
			actual, err := os.Stat(id.Executable)
			if err != nil {
				return id, err
			}
			if !os.SameFile(selectedInfo, actual) {
				return id, candidate.ErrIdentity
			}
			return id, nil
		}
		before, err := bind()
		if err != nil {
			return err
		}
		observation, healthy, err := observeCandidateHealth(ctx, req.Endpoint, op, j, observe)
		result.Observation = observation
		if err != nil || !healthy {
			return err
		}
		if err := checkSelection(); err != nil {
			return err
		}
		if err := verifyAndBindTargetImages(ctx, req.Request, j); err != nil {
			return err
		}
		after, err := bind()
		if err != nil {
			return err
		}
		if !before.SameProcess(after) {
			return candidate.ErrIdentity
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		revision, err := req.Store.AcknowledgeActivationHealth(ctx, opRev, jRev, j)
		if err != nil {
			return err
		}
		result.Acknowledged, result.JournalRevision = true, revision
		return nil
	})
	if err != nil && !ran {
		return HealthResult{}, errors.Join(ErrInvalidRequest, err)
	}
	return result, err
}
