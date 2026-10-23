package server

import (
	"cercano/source/server/pkg/proto"
	"errors"
)

// NotifyTUIUpdateInstalled is a trusted installer integration hook, not an RPC.
// Call only after client installation is verified. It does not authorize any
// executable or signal any process. The latest notice is replayed to subscribers
// reconnecting to this server; durable initialization after server replacement
// remains the installation controller's responsibility.
func (s *Server) NotifyTUIUpdateInstalled(version string, operationID int64) error {
	if operationID <= 0 || len(version) == 0 || len(version) > 128 {
		return errors.New("invalid installed TUI notice")
	}
	for _, c := range version {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '+' || c == '_') {
			return errors.New("invalid installed TUI version")
		}
	}
	h := s.eventHub()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.latestTUI != nil {
		previous := h.latestTUI.GetTuiUpdateInstalled()
		if previous.GetOperationId() > operationID {
			return errors.New("stale installed TUI notice")
		}
		if previous.GetOperationId() == operationID {
			if previous.GetVersion() != version {
				return errors.New("conflicting installed TUI notice")
			}
			return nil
		}
	}
	event := &proto.ClientEvent{Event: &proto.ClientEvent_TuiUpdateInstalled{TuiUpdateInstalled: &proto.TuiUpdateInstalled{Version: version, OperationId: operationID}}}
	h.latestTUI = event
	close(h.tuiChanged)
	h.tuiChanged = make(chan struct{})
	return nil
}
