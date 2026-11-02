package agentclient

import (
	"context"
	"net"
	"testing"
	"time"

	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Reproduce the historical wire contract: the service has no forced RPC at all,
// rather than a fake client returning an unsupported error. No real host runs.
func TestForcedTransferHistoricalServiceMissingRPC(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	descriptor := pb.Agent_ServiceDesc
	descriptor.Methods = nil
	for _, method := range pb.Agent_ServiceDesc.Methods {
		if method.MethodName != "LifecycleForceRelinquish" {
			descriptor.Methods = append(descriptor.Methods, method)
		}
	}
	server.RegisterService(&descriptor, &pb.UnimplementedAgentServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "passthrough:///historical", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	source := &LifecycleControls{AgentClient: pb.NewAgentClient(conn)}
	destination := &LifecycleControls{AgentClient: lifecycleFake{claim: func(*pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
		t.Fatal("claimed without historical source fencing")
		return nil, nil
	}}}
	response, err := source.ForceTransfer(ctx, destination, &pb.LifecycleControlRequest{})
	if status.Code(err) != codes.Unimplemented || response != nil {
		t.Fatalf("historical response=%v error=%v", response, err)
	}
	t.Logf("historical missing-method response: %v", err)
}
