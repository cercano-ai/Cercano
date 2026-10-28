package agentclient

import (
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"testing"
)

type shutdownStatusFixture struct{ proto.UnimplementedAgentServer }

func (*shutdownStatusFixture) ShutdownAgent(_ context.Context, r *proto.ShutdownAgentRequest) (*proto.ShutdownAgentResponse, error) {
	return &proto.ShutdownAgentResponse{Accepted: true, Phase: "draining", IncompleteReason: "callbacks or resource disposal pending"}, nil
}
func TestShutdownStatusDoesNotTreatAcceptanceAsCompletion(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	proto.RegisterAgentServer(gs, &shutdownStatusFixture{})
	go gs.Serve(listener)
	defer gs.GracefulStop()
	conn, err := grpc.NewClient("passthrough:///fixture", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := &Client{conn: conn, agent: proto.NewAgentClient(conn)}
	r, err := c.ShutdownStatus(t.Context())
	if err != nil || r.Complete || !r.Accepted || r.Phase != "draining" {
		t.Fatalf("status: %v %v", r, err)
	}
	if err := c.ShutdownAgent(t.Context(), "test"); err == nil {
		t.Fatal("acceptance reported as completion")
	}
}
