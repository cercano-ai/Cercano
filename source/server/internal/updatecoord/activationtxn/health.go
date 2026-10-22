package activationtxn

import (
	"context"
	"errors"
	"net/netip"

	"cercano/source/server/internal/updatecoord/candidate"
	"cercano/source/server/internal/updatecoord/operation"
	"cercano/source/server/internal/updatecoord/state"
)

var ErrHealthState = errors.New("activationtxn: health requires matching selected journal and health-check operation")

// ObserveCandidateHealth evaluates one bounded health attempt using the approved
// operational policy. It does not persist success, roll back, stop processes or
// delete files. The caller must supply the exact selected, verified owned launch
// and current records under installation exclusion. Before durable advancement
// it must revalidate selection and record revisions; this result is not a lease.
// Previously-absent-agent probe completion still requires separate exit proof.
// Terminals reconnect independently and are deliberately not inputs to this gate.
func ObserveCandidateHealth(ctx context.Context, c *candidate.Candidate, endpoint netip.AddrPort, op operation.Record, journal state.ActivationJournal) (candidate.Observation, bool, error) {
	return observeCandidateHealth(ctx, endpoint, op, journal, c.ObserveHealth)
}

type healthObserver func(context.Context, netip.AddrPort, string) (candidate.Observation, error)

func observeCandidateHealth(ctx context.Context, endpoint netip.AddrPort, op operation.Record, journal state.ActivationJournal, observe healthObserver) (candidate.Observation, bool, error) {
	if state.ValidateActivationJournal(journal, op.InstallationID) != nil || op.ID != journal.OpID || op.TargetVersion != journal.TargetVersion || op.State != operation.StateHealthCheck || op.RecoveryRequested || op.RecoveryNeeded || journal.Checkpoint != state.JournalSelected {
		return candidate.Observation{}, false, ErrHealthState
	}
	if ctx == nil {
		return candidate.Observation{}, false, candidate.ErrObservationRequest
	}
	if err := ctx.Err(); err != nil {
		return candidate.Observation{}, false, err
	}
	// ObserveHealth supplies its existing five-second cap (or earlier caller
	// deadline), exact lifetime/listener samples and expected-version comparison.
	observation, err := observe(ctx, endpoint, journal.TargetVersion)
	if err != nil {
		return observation, false, err
	}
	if err := ctx.Err(); err != nil {
		return observation, false, err
	}
	success := observation.Status == candidate.Consistent && observation.Version == journal.TargetVersion && observation.Ready && len(observation.NotReadyReasons) == 0
	return observation, success, nil
}
