//go:build darwin && cgo

package brewrestart

import (
	"context"
	"fmt"
	"path/filepath"
	"syscall"
)

// VerifyPreparedWritersAbsent qualifies only canonical prepared Cercano hosts:
// the host owns persistence and independent tasks; workers call back to that
// host and execute the same generation binary. The caller holds that generation's
// launch lock through authenticated shutdown, this observation and recovery.
// Any remaining generation process (even an ambiguous role/root) refuses recovery.
// This is deliberately NOT a file-descriptor sweep or a fence against arbitrary
// same-user tools. SQLite transactions and ownership checks protect committed
// history; interrupted external effects remain uncertain and must not be replayed.
func VerifyPreparedWritersAbsent(ctx context.Context, generation, store string) error {
	if !filepath.IsAbs(generation) || !filepath.IsAbs(store) {
		return fmt.Errorf("absolute prepared generation and store required")
	}
	pids, err := ListCandidates()
	if err != nil {
		return err
	}
	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return err
		}
		executable, _, err := recoveryProcessArguments(pid)
		if err == syscall.ESRCH {
			continue
		}
		if err != nil {
			// A protected unrelated executable is not a plausible source just because
			// it shares our UID. A second kernel identity API may exclude it. Never use
			// argv[0], a basename, or a failed path lookup as negative evidence.
			id, inspectErr := Inspect(pid)
			if inspectErr != nil {
				return fmt.Errorf("prepared process attribution pid=%d: argv: %v; identity: %w", pid, err, inspectErr)
			}
			executable = id.Executable
		}
		if executable == "" || !filepath.IsAbs(executable) {
			return fmt.Errorf("uncertain kernel executable pid=%d", pid)
		}
		if executable == filepath.Join(generation, "cercano") {
			return fmt.Errorf("source generation process still present pid=%d", pid)
		}
	}
	return nil
}
