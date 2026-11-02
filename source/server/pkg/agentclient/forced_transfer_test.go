package agentclient

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

type forcedLifecycleFake struct {
	lifecycleFake
	force func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error)
}

func (f forcedLifecycleFake) LifecycleForceRelinquish(_ context.Context, r *pb.LifecycleControlRequest, _ ...grpc.CallOption) (*pb.LifecycleControlResponse, error) {
	return f.force(r)
}
func TestForcedTransferNeverFallsBackToOldProtocol(t *testing.T) {
	a := &LifecycleControls{AgentClient: forcedLifecycleFake{lifecycleFake: lifecycleFake{relinquish: func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		t.Fatal("fell back to old transfer")
		return nil, nil
	}}, force: func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		return nil, status.Error(codes.Unimplemented, "old host")
	}}}
	b := &LifecycleControls{AgentClient: lifecycleFake{claim: func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		t.Fatal("claimed without source fencing")
		return nil, nil
	}}}
	if _, err := a.ForceTransfer(context.Background(), b, &pb.LifecycleControlRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatal(err)
	}
}
func TestForcedTransferPreservesReceiptOnLostDestinationResponse(t *testing.T) {
	a := &LifecycleControls{AgentClient: forcedLifecycleFake{force: func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		return &pb.LifecycleControlResponse{Pending: true, Receipt: "exact-receipt"}, nil
	}}}
	b := &LifecycleControls{AgentClient: lifecycleFake{claim: func(r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		if r.Receipt != "exact-receipt" || r.Target.Host != "B" {
			t.Fatalf("wrong claim %+v", r)
		}
		return nil, status.Error(codes.Unavailable, "lost response")
	}}}
	response, err := a.ForceTransfer(context.Background(), b, &pb.LifecycleControlRequest{Destination: &pb.LifecycleIdentity{Host: "B"}})
	if err == nil || response.GetReceipt() != "exact-receipt" || !response.GetPending() {
		t.Fatalf("lost retry authority: %v %v", response, err)
	}
}
