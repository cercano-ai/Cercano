package agentclient

import (
	pb "cercano/source/server/pkg/proto"
	"context"
	"errors"
	"google.golang.org/grpc"
	"testing"
)

type lifecycleFake struct {
	pb.AgentClient
	prepare    func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error)
	relinquish func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error)
	claim      func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error)
}

func (f lifecycleFake) LifecyclePrepare(_ context.Context, r *pb.LifecycleControlRequest, _ ...grpc.CallOption) (*pb.LifecycleControlResponse, error) {
	return f.prepare(r)
}
func (f lifecycleFake) LifecycleRelinquish(_ context.Context, r *pb.LifecycleControlRequest, _ ...grpc.CallOption) (*pb.LifecycleControlResponse, error) {
	return f.relinquish(r)
}
func (f lifecycleFake) LifecycleClaim(_ context.Context, r *pb.LifecycleControlRequest, _ ...grpc.CallOption) (*pb.LifecycleControlResponse, error) {
	return f.claim(r)
}
func TestLifecycleControlsTransferKeepsReceiptOnLostClaim(t *testing.T) {
	step := 0
	lost := errors.New("lost claim response")
	a := &LifecycleControls{AgentClient: lifecycleFake{relinquish: func(r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		if step != 0 || r.Receipt != "" {
			t.Fatal("commit before acceptance")
		}
		step++
		return &pb.LifecycleControlResponse{Receipt: "opaque", Pending: true}, nil
	}}}
	b := &LifecycleControls{AgentClient: lifecycleFake{prepare: func(r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		t.Fatal("client must not prepare destination")
		return nil, nil
	}, claim: func(r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		if step != 1 || r.Receipt != "opaque" {
			t.Fatal("claim order")
		}
		return nil, lost
	}}}
	r := &pb.LifecycleControlRequest{Destination: &pb.LifecycleIdentity{Host: "B"}}
	result, e := a.Transfer(context.Background(), b, r)
	if !errors.Is(e, lost) || result.GetReceipt() != "opaque" || r.Receipt != "" {
		t.Fatal(result, e)
	}
}
func TestLifecycleControlsRejectsImplicitOrRemoteEndpoint(t *testing.T) {
	for _, endpoint := range []string{"localhost:1", "example.com:1", "0.0.0.0:1", ""} {
		if _, e := DialLifecycleControls(context.Background(), endpoint, nil); e == nil {
			t.Fatal(endpoint)
		}
	}
}

func TestLifecycleControlsTransferPreservesProvidedReceipt(t *testing.T) {
	lost := errors.New("source outcome unknown")
	a := &LifecycleControls{AgentClient: lifecycleFake{relinquish: func(r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		if r.Receipt != "retry-receipt" {
			t.Fatal("receipt replaced")
		}
		return nil, lost
	}}}
	b := &LifecycleControls{AgentClient: lifecycleFake{prepare: func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		t.Fatal("unexpected prepare")
		return nil, nil
	}}}
	result, err := a.Transfer(context.Background(), b, &pb.LifecycleControlRequest{Receipt: "retry-receipt"})
	if !errors.Is(err, lost) || result.GetReceipt() != "retry-receipt" || !result.Pending {
		t.Fatal(result, err)
	}
}
