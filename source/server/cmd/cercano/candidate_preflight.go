package main

import "io"

// candidatePreflightReport is deliberately constant and bounded. Reaching this
// entrypoint proves only that this image can execute the offline private path.
// It does not inspect configuration, open stores/keychains, reap workers, start
// retention, bind listeners, or launch children. It is not a health result or
// evidence that normal startup preserves backward compatibility of user state.
// Keep the existing updater capability document unchanged: execution is still
// unavailable. No production launcher consumes this preparatory response.
const candidatePreflightReport = `{"component":"cercano","protocol":1,"preflight":"entrypoint_reached","health":"not_checked","persistent_state_compatibility":"unknown","execution_ready":false}` + "\n"

// runCandidatePreflight accepts only __internal-update --preflight. It takes no
// paths, environment overrides, operation IDs, or commands. Exit zero means the
// report was written, not that activation or rollback is safe. Output is bounded
// in size, not in writer latency; a future process owner must bound execution
// time and prove completion using its exact owned child, as with other probes.
func runCandidatePreflight(rest []string, out, errOut io.Writer) int {
	if len(rest) != 0 {
		io.WriteString(errOut, "cercano: usage: __internal-update --preflight\n")
		return privateExitUsage
	}
	n, err := io.WriteString(out, candidatePreflightReport)
	if err != nil || n != len(candidatePreflightReport) {
		return privateExitExecution
	}
	return 0
}
