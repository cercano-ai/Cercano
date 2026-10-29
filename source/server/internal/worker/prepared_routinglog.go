package worker

import "path/filepath"

// WithRoutingLogPath supplies constructor-pinned authority, never turn metadata
// or a HOME/environment default. Prepared workers require this option.
func WithRoutingLogPath(path string) Option {
	return func(w *WorkerServer) { w.routingLogPath = path }
}

// ValidatePreparedRoutingLog applies the same private namespace contract as the
// failure log, before dependency construction or log opens.
func ValidatePreparedRoutingLog(path string) error {
	return validatePreparedLog(path, "routing-log")
}

func (p SpawnPolicy) routingLogPath() string {
	if !p.pinned {
		return ""
	}
	return filepath.Join(p.dir, "turn-routing.jsonl")
}
