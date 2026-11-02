package agentclient

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"net"
)

// LifecycleControls uses only the explicitly selected literal loopback endpoint.
// It never discovers, launches, reconnects, or resumes implicitly. Dial MUST
// authenticate the selected peer: loopback alone does not establish trust.
// Startup/CLI must supply the authorization policy before enabling controls.
type LifecycleControls struct {
	pb.AgentClient
	conn *grpc.ClientConn
}

func DialLifecycleControls(ctx context.Context, endpoint string, authenticatedDial func(context.Context, string) (*grpc.ClientConn, error)) (*LifecycleControls, error) {
	host, _, err := net.SplitHostPort(endpoint)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || authenticatedDial == nil {
		return nil, fmt.Errorf("explicit loopback endpoint and authenticated dial required")
	}
	conn, err := authenticatedDial(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return &LifecycleControls{AgentClient: pb.NewAgentClient(conn), conn: conn}, nil
}

// Close releases transport only, never ownership or an incoming reservation.
func (c *LifecycleControls) Close() error { return c.conn.Close() }

// Transfer reports success only after the destination's durable claim. A lost
// response is retried explicitly with LifecycleClaim and the same receipt, not
// by acquiring a fresh owner or canceling the reservation.
func (c *LifecycleControls) Transfer(ctx context.Context, destination *LifecycleControls, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return c.transfer(ctx, destination, r, false)
}

// ForceTransfer cancels source execution before handing ownership to destination.
// An older host's Unimplemented response is returned without terminal-only fallback.
func (c *LifecycleControls) ForceTransfer(ctx context.Context, destination *LifecycleControls, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return c.transfer(ctx, destination, r, true)
}
func (c *LifecycleControls) transfer(ctx context.Context, destination *LifecycleControls, r *pb.LifecycleControlRequest, force bool) (*pb.LifecycleControlResponse, error) {
	source := proto.Clone(r).(*pb.LifecycleControlRequest)
	var acceptance *pb.LifecycleControlResponse
	var err error
	if force {
		acceptance, err = c.LifecycleForceRelinquish(ctx, source)
	} else {
		acceptance, err = c.LifecycleRelinquish(ctx, source)
	}
	if err != nil {
		for _, detail := range status.Convert(err).Details() {
			if response, ok := detail.(*pb.LifecycleControlResponse); ok {
				acceptance = response
			}
		}
		if acceptance == nil && r.Receipt != "" {
			acceptance = &pb.LifecycleControlResponse{Receipt: r.Receipt, Pending: true}
		}
		return acceptance, err
	}
	if acceptance.GetReceipt() == "" {
		return acceptance, fmt.Errorf("source returned no transfer receipt")
	}
	prepared := proto.Clone(r).(*pb.LifecycleControlRequest)
	prepared.Target = r.Destination
	prepared.Receipt = acceptance.Receipt
	claimed, err := destination.LifecycleClaim(ctx, prepared)
	if err != nil {
		return acceptance, err
	}
	if force && claimed != nil {
		// Keep retry authority even when claim succeeded but the subsequent
		// pinned client dial fails after the old host has retired.
		claimed = proto.Clone(claimed).(*pb.LifecycleControlResponse)
		claimed.Receipt = acceptance.Receipt
	}
	return claimed, nil
}
