package server

import (
	"context"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/broker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type permissionGateKey struct{ conversation, tool string }
type permissionGate struct {
	owner broker.Owner
	ctx   context.Context
}

// waitPermission binds only a registered per-turn barrier, never authority
// inferred from RPC metadata. The notify runs after the broker registers it.
func (s *Server) waitPermission(ctx context.Context, conv, tool string, notify func() error) (agent.Decision, error) {
	if s.coexistence == nil {
		return s.permBroker.WaitWithNotify(ctx, conv, tool, notify)
	}
	cap, err := s.resolvePreparedTaskOwner(ctx, conv)
	if err != nil {
		return agent.Decision{}, err
	}
	gate := &permissionGate{owner: ownerOf(cap), ctx: ctx}
	key := permissionGateKey{conv, tool}
	// Reserve the key before broker registration, so overlapping turns cannot
	// replace another epoch's pending decision under an existing gate.
	s.permissionMu.Lock()
	if s.permissionGates == nil {
		s.permissionGates = make(map[permissionGateKey]*permissionGate)
	}
	if s.permissionGates[key] != nil {
		s.permissionMu.Unlock()
		return agent.Decision{}, status.Error(codes.FailedPrecondition, "permission gate already pending")
	}
	s.permissionGates[key] = gate
	s.permissionMu.Unlock()
	defer func() {
		s.permissionMu.Lock()
		defer s.permissionMu.Unlock()
		if s.permissionGates[key] == gate {
			delete(s.permissionGates, key)
		}
	}()
	return s.permBroker.WaitWithNotify(ctx, conv, tool, notify)
}

func (s *Server) resolvePermission(ctx context.Context, conv, tool string, decision agent.Decision) (bool, error) {
	if s.coexistence == nil {
		if s.permBroker == nil {
			return false, nil
		}
		return s.permBroker.Resolve(conv, tool, decision), nil
	}
	// Prepared configuration remains frozen: no persisted or shared grants.
	if decision.Persist {
		return false, status.Error(codes.FailedPrecondition, "prepared permissions permit one-shot decisions only")
	}
	owner, id, err := ParseTerminalBinding(ctx, conv)
	if err != nil {
		return false, err
	}
	l, err := s.lifecycle()
	if err != nil {
		return false, err
	}
	if err = l.admission.ValidateTerminal(owner, id); err != nil {
		return false, err
	}
	// This is retained work, NOT beginForeground: approval must not supersede
	// the very turn that is blocked waiting for the user's answer.
	_, ticket, err := s.BeginRetainedWork(ctx, owner, broker.CallbackWork)
	if err != nil {
		return false, err
	}
	defer ticket.Complete()
	s.permissionMu.Lock()
	defer s.permissionMu.Unlock()
	gate := s.permissionGates[permissionGateKey{conv, tool}]
	if gate == nil || gate.owner != owner || gate.ctx.Err() != nil {
		return false, status.Error(codes.FailedPrecondition, "no live permission gate for retained owner")
	}
	if s.permBroker == nil || !s.permBroker.Resolve(conv, tool, decision) {
		return false, status.Error(codes.FailedPrecondition, "permission decision is no longer pending")
	}
	return true, nil
}
