//go:build darwin && cgo

package brewrestart

import (
	"context"
	"errors"
	"fmt"
	"syscall"
)

// freezeUpgradeDescendants handles attached, same-user descendants of verified
// production workers. It does not claim to contain detached/reparented daemons.
// Each parent is stopped before enumerating children; no process-group signal or
// executable-name-only ownership is used. The caller resumes the recorded set on
// reversible refusal, or includes it in irreversible cleanup after TERM.
func freezeUpgradeDescendants(ctx context.Context, host Identity, workers map[int]Identity, frozen map[int]stoppedIdentity, rootChecks ...func(Identity) error) error {
	queue := make([]Identity, 0, len(workers))
	freeze := func(id Identity) error {
		var record stoppedIdentity
		err := freezeUpgradeHost(ctx, id, &record)
		frozen[id.PID] = record
		if err != nil {
			return err
		}
		queue = append(queue, id)
		return nil
	}
	for _, id := range workers {
		for _, check := range rootChecks {
			if err := check(id); err != nil {
				return err
			}
		}
		if err := freeze(id); err != nil {
			return err
		}
		for _, check := range rootChecks {
			if err := check(id); err != nil {
				return err
			}
		}
	}
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		parent := queue[0]
		queue = queue[1:]
		now, err := Inspect(parent.PID)
		if err != nil || !now.SameProcess(parent) {
			return fmt.Errorf("upgrade parent identity lost pid=%d: %v", parent.PID, err)
		}
		children, err := upgradeChildren(parent.PID)
		if err != nil {
			return err
		}
		for _, pid := range children {
			id, err := Inspect(pid)
			if errors.Is(err, syscall.ESRCH) {
				continue
			}
			if err != nil {
				return err
			}
			if id.ParentPID != parent.PID {
				return fmt.Errorf("upgrade child reparented pid=%d", pid)
			}
			if id.UID != host.UID || id.StartSeconds < parent.StartSeconds || (id.StartSeconds == parent.StartSeconds && id.StartMicroseconds < parent.StartMicroseconds) {
				return fmt.Errorf("upgrade child ownership unresolved pid=%d", pid)
			}
			shared, err := verifiedModelHost(id)
			if err != nil {
				return err
			}
			if shared {
				continue
			}

			if _, ok := frozen[pid]; ok {
				return fmt.Errorf("upgrade duplicate child pid=%d", pid)
			}
			if len(frozen) >= 4096 {
				return fmt.Errorf("upgrade descendant limit exceeded")
			}
			if err := freeze(id); err != nil {
				return err
			}
		}
	}
	return nil
}
