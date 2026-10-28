package main

import (
	"cercano/source/server/pkg/proto"
	"context"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The control handle's Status does not enter RPC/work admission. Each host has
// its own gate; closing it cannot fence a safe sibling generation.
type productionAdmissionGate struct {
	closed atomic.Bool
	active atomic.Int64
}

func newProductionAdmissionGate() *productionAdmissionGate { return &productionAdmissionGate{} }
func (g *productionAdmissionGate) close()                  { g.closed.Store(true) }
func (g *productionAdmissionGate) unary(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	if info != nil && info.FullMethod == proto.Agent_ShutdownAgent_FullMethodName {
		return next(ctx, req)
	}
	g.active.Add(1)
	defer g.active.Add(-1)
	if g.closed.Load() {
		return nil, status.Error(codes.Unavailable, "shutdown incomplete or complete; ordinary admission closed")
	}
	return next(ctx, req)
}
func (g *productionAdmissionGate) stream(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
	g.active.Add(1)
	defer g.active.Add(-1)
	if g.closed.Load() {
		return status.Error(codes.Unavailable, "shutdown incomplete or complete; ordinary admission closed")
	}
	return next(srv, stream)
}

// Drain keeps the status transport available while admitted ordinary RPCs finish.
func (g *productionAdmissionGate) drain() {
	for g.active.Load() != 0 {
		time.Sleep(time.Millisecond)
	}
}
