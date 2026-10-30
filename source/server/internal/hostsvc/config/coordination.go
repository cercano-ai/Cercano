package config

import cfg "cercano/source/server/pkg/config"

// PrepareDiskCoordination explicitly opts into disk-only coordinated mutations
// at the service's existing authority. It does not change Service, Set, Mutate,
// Persist, or current state, nor does it activate a prepared writable config.
// Callers must reconcile runtime state and notify other hosts separately after
// publication. Legacy Persist must not be mixed with coordinated writers.
// A subsequent SetPath does not retarget this prepared coordinator.
func PrepareDiskCoordination(s Service) (*cfg.ConfigCoordinator, error) {
	if s.Frozen() {
		return nil, ErrFrozen
	}
	return s.Get().PrepareConfigCoordination(s.Path())
}
