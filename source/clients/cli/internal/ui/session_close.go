package ui

import "context"

// IntentionalExit distinguishes a user's quit from transport/program failure.
func (m Model) IntentionalExit() bool { return m.intentionalExit }

// CloseSession uses the final model's client, not the initial connection: history
// reopen and explicit reconnect can replace it. Detached sessions stay retained.
// No task cancellation is sent; independent work drains under server accounting.
func (m Model) CloseSession(ctx context.Context) error {
	if !m.intentionalExit || m.devHost == nil || m.devHost.detached {
		return nil
	}
	return m.agent.ReleaseSession(ctx, m.devHost.selection.Identity)
}
