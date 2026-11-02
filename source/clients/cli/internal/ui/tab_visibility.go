package ui

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Visibility intent is serialized on the reducer, including reconciliation.
// No retry or read can outlive a newer intent or a replacement client. A lost
// response is not proof of rollback: read the same owner's persisted open tabs.
func (m *Model) persistTabVisibility(id string, open bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	var err error
	if open {
		err = m.agent.ReopenSubAgent(ctx, id)
	} else {
		err = m.agent.DismissSubAgent(ctx, id)
	}
	cancel()
	if err == nil {
		m.errMsg = ""
		return nil
	}
	switch status.Code(err) {
	case codes.InvalidArgument, codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition, codes.Unimplemented:
		return fmt.Errorf("rejected (visibility unchanged locally): %w", err)
	}
	if m.convID != "" {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		children, readErr := m.agent.ListSubAgents(ctx, m.convID)
		cancel()
		if readErr == nil {
			visible := false
			for _, child := range children {
				if child.ID == id {
					visible = true
					break
				}
			}
			if visible == open {
				m.errMsg = ""
				return nil
			}
		}
	}
	return fmt.Errorf("save outcome uncertain; retry or resume to reconcile: %w", err)
}
