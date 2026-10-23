package ui

import (
	"context"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"cercano/source/server/pkg/agentclient"
)

// permissionModeChangedMsg is delivered when the agent pushes a permission-mode
// change over the standing SubscribeEvents stream — a /strict-style command from
// any client, or an out-of-band edit to permissions.yaml. next re-arms the drain
// loop, mirroring chatStreamMsg so the host stays a pure router.
type permissionModeChangedMsg struct {
	mode string
	next tea.Cmd
}

// openRuntimeStatusChangedMsg is delivered when the agent pushes a
// OpenRuntimeStatusChanged event — the local-runtime detection outcome
// after a config-driven runtime swap. Non-ok statuses drive the chip and
// the install modal; ok statuses clear both.
type openRuntimeStatusChangedMsg struct {
	status *agentclient.OpenRuntimeStatus
	next   tea.Cmd
}

// sessionProfileChangedMsg is delivered when the agent pushes a
// SessionProfileChanged event — the active capability profile (planning fence /
// future modes) flipped for some conversation, via /plan, an approved
// suggest_plan, plan_exit, or request_plan_approval. convID scopes it: the model
// updates its footer chip only when convID matches the active conversation,
// since the active profile is per-conversation.
type sessionProfileChangedMsg struct {
	convID  string
	profile string
	next    tea.Cmd
}

// configChangedMsg is delivered when the agent pushes a ConfigChanged event
// over the standing SubscribeEvents stream. It keeps header/status chips fresh
// after settings/profile changes without waiting for an explicit config reload.
type configChangedMsg struct {
	field string
	value string
	next  tea.Cmd
}

// subscribeEventsCmd opens the standing server->client event stream and returns
// a cmd that drains one event and re-arms itself. This is how the status bar
// learns about mode changes and Meridian state without polling. On stream
// error/close it stops (returns a nil msg) so we don't spin; chips simply
// keep their last value.
var eventStreamGeneration atomic.Uint64

func subscribeEventsCmd(ag *agentclient.Client) tea.Cmd {
	return func() tea.Msg {
		generation := eventStreamGeneration.Add(1)
		var revision uint64
		ch, err := ag.SubscribeEvents(context.Background())
		if err != nil {
			return nil
		}
		var wait tea.Cmd
		wait = func() tea.Msg {
			ev, ok := <-ch
			revision++
			if !ok || ev.Err != nil {
				return updateReadinessMsg{generation: generation, revision: revision} // withdraw live state; stop draining
			}
			if ev.UpdateReadiness != nil {
				return updateReadinessMsg{generation: generation, revision: revision, notice: *ev.UpdateReadiness, next: wait}
			}
			if ev.TUIUpdate != nil {
				return tuiUpdateNoticeMsg{version: ev.TUIUpdate.Version, operationID: ev.TUIUpdate.OperationID, next: wait}
			}
			if ev.OpenRuntimeStatus != nil {
				return openRuntimeStatusChangedMsg{status: ev.OpenRuntimeStatus, next: wait}
			}
			if ev.ConfigChanged != nil {
				return configChangedMsg{field: ev.ConfigChanged.Field, value: ev.ConfigChanged.Value, next: wait}
			}
			if ev.SessionProfile != nil {
				return sessionProfileChangedMsg{convID: ev.SessionProfile.ConversationID, profile: ev.SessionProfile.Profile, next: wait}
			}
			return permissionModeChangedMsg{mode: ev.Mode, next: wait}
		}
		return wait()
	}
}

// connStateChangedMsg is delivered when the SDK's reconnect loop
// transitions the gRPC connection between healthy / reconnecting /
// terminally-failed. Drives the status-bar chip and the prompt-
// rehydration flow (see Model.handleConnStateChanged).
type connStateChangedMsg struct {
	state        agentclient.ConnState
	attempt      int
	errMsg       string
	crashSummary string // populated on Connected → Reconnecting when the crash log has a fresh entry
	next         tea.Cmd
}

// subscribeConnStateCmd hooks the SDK's connection-state channel into
// the bubbletea event loop. Mirrors the shape of subscribeEventsCmd:
// one message per state transition, with next re-arming the drain.
// When the channel closes (Client.Close) the drain stops.
func subscribeConnStateCmd(ag *agentclient.Client) tea.Cmd {
	return func() tea.Msg {
		ch, _ := ag.ConnStateChanges()
		var wait tea.Cmd
		wait = func() tea.Msg {
			ev, ok := <-ch
			if !ok {
				return nil
			}
			msg := connStateChangedMsg{
				state:        ev.State,
				attempt:      ev.Attempt,
				crashSummary: ev.CrashSummary,
				next:         wait,
			}
			if ev.Err != nil {
				msg.errMsg = ev.Err.Error()
			}
			return msg
		}
		return wait()
	}
}

// Installed metadata only: no executable path or instruction from the stream.
type tuiUpdateNoticeMsg struct {
	version     string
	operationID int64
	next        tea.Cmd
}

type updateReadinessMsg struct {
	generation, revision uint64
	notice               agentclient.UpdateReadiness
	next                 tea.Cmd
}
