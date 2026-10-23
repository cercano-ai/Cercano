// Package server — server->client event push.
//
// Clients hold a single SubscribeEvents stream open for their whole session.
// The agent broadcasts unsolicited state changes (today: permission-mode
// changes) to every open stream, so clients never poll. The hub is a small
// fan-out: subscribe registers a buffered channel, broadcast does a
// non-blocking send to each. Mode changes are rare, so the buffer never
// realistically fills; a wedged-slow client is dropped a frame rather than
// stalling the broadcast for everyone.
package server

import (
	"fmt"
	"sync"

	"cercano/source/server/pkg/proto"
)

// eventHub fans server-originated ClientEvents out to all open subscriber
// streams. Safe for concurrent Subscribe / broadcast / unsubscribe.
type eventHub struct {
	mu                sync.Mutex
	readinessOwner    uint64
	readinessPhase    proto.UpdateReadiness_Phase
	readiness         *proto.ClientEvent
	readinessRevision uint64
	readinessChanged  chan struct{}
	next              int
	subs              map[int]chan *proto.ClientEvent
	onEmpty           func()
	latestTUI         *proto.ClientEvent
	tuiInstallID      string
	tuiChanged        chan struct{}
}

func newEventHub() *eventHub {
	return &eventHub{subs: make(map[int]chan *proto.ClientEvent), tuiChanged: make(chan struct{}), readinessChanged: make(chan struct{})}
}

// subscribe registers a new subscriber and returns its event channel plus an
// unsubscribe func the caller MUST invoke when its stream ends.
func (h *eventHub) subscribe() (<-chan *proto.ClientEvent, func()) {
	ch := make(chan *proto.ClientEvent, 16)
	h.mu.Lock()
	id := h.next
	h.next++
	h.subs[id] = ch
	h.mu.Unlock()
	return ch, func() {
		var onEmpty func()
		h.mu.Lock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
			if len(h.subs) == 0 {
				onEmpty = h.onEmpty
			}
		}
		h.mu.Unlock()
		if onEmpty != nil {
			onEmpty()
		}
	}
}

func (h *eventHub) setOnEmpty(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onEmpty = fn
}

// broadcast delivers ev to every current subscriber without blocking on any
// one of them.
func (h *eventHub) broadcast(ev *proto.ClientEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- ev:
		default: // subscriber is wedged; drop rather than stall the others
		}
	}
}

// closeAll closes every subscriber channel and empties the hub, ending each
// standing SubscribeEvents handler loop with a nil error. Shutdown-only:
// without this, GracefulStop blocks forever on attached clients. Previously
// returned unsubscribe funcs stay safe — they find their id already deleted.
func (h *eventHub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
}

// subscriberCount reports how many streams are currently open (test/observability).
func (h *eventHub) subscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// eventHub initializes the shared fan-out even when Server was zero-initialized.
func (s *Server) eventHub() *eventHub {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	if s.events == nil {
		s.events = newEventHub()
	}
	return s.events
}

// SubscribeEvents implements proto.AgentServer — a standing server->client
// stream. It blocks, forwarding hub events to the client until the client
// disconnects or the server shuts the stream down.
func (s *Server) SubscribeEvents(_ *proto.SubscribeEventsRequest, stream proto.Agent_SubscribeEventsServer) error {
	hub := s.eventHub()
	ch, unsubscribe := hub.subscribe()
	defer unsubscribe()

	ctx := stream.Context()
	var lastTUI int64
	var lastReadiness uint64
	var lastLiveToken string
	initial := true
	for {
		hub.mu.Lock()
		latest, changed := hub.latestTUI, hub.tuiChanged
		ready, revision, readinessChanged := hub.readiness, hub.readinessRevision, hub.readinessChanged
		hub.mu.Unlock()
		if revision != lastReadiness {
			if ready != nil && (!initial || ready.GetUpdateReadiness().GetPhase() != proto.UpdateReadiness_RELEASED) {
				// Coalescing may skip an entire newer owner. Withdraw the last
				// live owner this stream actually saw, not an unseen token that
				// a token-fenced UI would (correctly) ignore.
				if ready.GetUpdateReadiness().GetPhase() == proto.UpdateReadiness_RELEASED {
					if lastLiveToken != "" {
						ready = &proto.ClientEvent{Event: &proto.ClientEvent_UpdateReadiness{UpdateReadiness: &proto.UpdateReadiness{OperationToken: lastLiveToken, Phase: proto.UpdateReadiness_RELEASED}}}
					}
					lastLiveToken = ""
				} else {
					lastLiveToken = ready.GetUpdateReadiness().GetOperationToken()
				}
				if err := stream.Send(ready); err != nil {
					return err
				}
			}
			lastReadiness = revision
		}
		initial = false
		if latest != nil && latest.GetTuiUpdateInstalled().GetOperationId() > lastTUI {
			if err := stream.Send(latest); err != nil {
				return err
			}
			lastTUI = latest.GetTuiUpdateInstalled().GetOperationId()
		}
		select {
		case <-readinessChanged:
			continue
		case <-changed:
			continue
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}

// broadcastConfigChanged pushes a ConfigChanged event for a single field to all
// clients. Both the UpdateConfig RPC and the config-file watcher route through
// the same UpdateConfig path, so this is the one place a value-actually-changed
// event is emitted. No dedupe: UpdateConfig only calls in for fields that
// passed validation and were actually applied, so a repeat broadcast for the
// same value can't happen from the agent side.
func (s *Server) broadcastConfigChanged(field, value string) {
	s.eventHub().broadcast(&proto.ClientEvent{
		Event: &proto.ClientEvent_ConfigChanged{
			ConfigChanged: &proto.ConfigChanged{Field: field, Value: value},
		},
	})
}

// broadcastPermissionMode pushes a PermissionModeChanged event to all clients.
// Deduplication is handled by the permissions.Broker (both the SetMode RPC path
// and the file-watcher path call through the broker before arriving here).
func (s *Server) broadcastPermissionMode(mode string) {
	s.eventHub().broadcast(&proto.ClientEvent{
		Event: &proto.ClientEvent_PermissionModeChanged{
			PermissionModeChanged: &proto.PermissionModeChanged{Mode: mode},
		},
	})
}

// setSessionProfile flips a conversation's active capability profile via the
// broker and, on success, broadcasts the resulting active name to all clients.
// This is the single choke point for profile changes: the /plan RPC, the
// suggest_plan/plan_exit EnterProfile hook, and the worker setProfile bridge all
// route through here, so the footer chip can never drift from the broker state.
// It broadcasts the broker's *resolved* active name (e.g. "" and "default" both
// collapse to the unrestricted posture) rather than the raw requested name.
func (s *Server) setSessionProfile(convID, name string) error {
	if s.profileBroker == nil {
		return fmt.Errorf("profile broker not configured")
	}
	if err := s.profileBroker.SetActive(convID, name); err != nil {
		return err
	}
	s.broadcastSessionProfile(convID, s.profileBroker.ActiveName(convID))
	return nil
}

// broadcastSessionProfile pushes a SessionProfileChanged event to all clients
// after a conversation's active capability profile flips (via /plan, an approved
// suggest_plan, plan_exit, or request_plan_approval). The event carries the
// conversation ID so each client updates its footer chip only for its own
// conversation — the active profile is per-conversation, so a bare broadcast
// without the ID would leak one conversation's planning state onto every footer.
func (s *Server) broadcastSessionProfile(convID, profile string) {
	s.eventHub().broadcast(&proto.ClientEvent{
		Event: &proto.ClientEvent_SessionProfileChanged{
			SessionProfileChanged: &proto.SessionProfileChanged{
				ConversationId: convID,
				Profile:        profile,
			},
		},
	})
}

// Open-runtime status is now built by the single runtime-agnostic readiness
// path in open_runtime_readiness.go (openRuntimeStatus / openRuntimeStatusFrom),
// which replaced the per-runtime buildOpenRuntimeStatus + buildMistralRSStatus
// forks and their `runtime != "llama_server"` gates.

// broadcastOpenRuntimeStatus pushes a OpenRuntimeStatusChanged event for
// the current state of the active local inference runtime. Called from the
// runtime-swap path in UpdateConfig — one event per swap attempt (ok or not)
// so clients always have a fresh chip regardless of whether the runtime
// works or is broken.
//
// status is passed pre-built so callers can populate fields from a
// llamaserver.DetectError, from a healthy post-detect config, or from a
// fresh install-completion event without a helper here having to know all
// three shapes.
func (s *Server) broadcastOpenRuntimeStatus(status *proto.OpenRuntimeStatus) {
	if status == nil {
		return
	}
	s.eventHub().broadcast(&proto.ClientEvent{
		Event: &proto.ClientEvent_OpenRuntimeStatusChanged{
			OpenRuntimeStatusChanged: &proto.OpenRuntimeStatusChanged{
				Status: status,
			},
		},
	})
}

// Generation comes from admission, never from the caller token. Release is
// terminal for that generation, even if its delayed seal callback arrives later.
// Separate live-state wakeups never consume/drop ordinary queued events.
func (h *eventHub) setUpdateReadiness(owner uint64, token string, phase proto.UpdateReadiness_Phase) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if owner < h.readinessOwner || (owner == h.readinessOwner && phase <= h.readinessPhase) {
		return
	}
	h.readinessOwner, h.readinessPhase = owner, phase
	h.readiness = &proto.ClientEvent{Event: &proto.ClientEvent_UpdateReadiness{UpdateReadiness: &proto.UpdateReadiness{OperationToken: token, Phase: phase}}}
	h.readinessRevision++
	close(h.readinessChanged)
	h.readinessChanged = make(chan struct{})
}
