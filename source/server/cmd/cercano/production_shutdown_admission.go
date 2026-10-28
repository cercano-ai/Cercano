package main

import (
	"context"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The control handle's Status does not enter RPC/work admission. Each host has
// its own gate; closing it cannot fence a safe sibling generation.
type productionAdmissionGate struct{ closed atomic.Bool }

func newProductionAdmissionGate() *productionAdmissionGate { return &productionAdmissionGate{} }
func (g *productionAdmissionGate) close()                  { g.closed.Store(true) }
func (g *productionAdmissionGate) unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	if g.closed.Load() {
		return nil, status.Error(codes.Unavailable, "shutdown incomplete or complete; ordinary admission closed")
	}
	return next(ctx, req)
}
func (g *productionAdmissionGate) stream(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
	if g.closed.Load() {
		return status.Error(codes.Unavailable, "shutdown incomplete or complete; ordinary admission closed")
	}
	return next(srv, stream)
}
