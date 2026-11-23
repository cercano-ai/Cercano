package worker

import (
	"context"
	"fmt"
	"os"
)

// ObserveWorkerExit captures the exact currently pooled child and an observer of
// its owned Wait. It neither retires the child nor changes pool membership. The
// receipt remains valid after detachment; context expiry does not abandon Wait.
func (w *workerRunner) ObserveWorkerExit(conversation string) (int, func(context.Context) (*os.ProcessState, error), error) {
	w.pool.mu.Lock()
	defer w.pool.mu.Unlock()
	e := w.pool.byConv[conversation]
	if e == nil || e.handle.cmd == nil || e.handle.reapDone == nil {
		return 0, nil, fmt.Errorf("no observable pooled child")
	}
	h := e.handle
	done := h.reapDone
	pid := h.cmd.Process.Pid
	return pid, func(ctx context.Context) (*os.ProcessState, error) {
		select {
		case <-done:
			return h.cmd.ProcessState, h.reapErr
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}, nil
}
