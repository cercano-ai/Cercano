package agentclient

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mismatchedDrainServer struct {
	proto.UnimplementedAgentServer
	lost chan struct{}
}

func (s *mismatchedDrainServer) DrainForUpdate(_ *proto.DrainForUpdateRequest, stream proto.Agent_DrainForUpdateServer) error {
	if err := stream.Send(&proto.UpdateDrainState{OperationToken: strings.Repeat("b", 32), Phase: proto.UpdateDrainState_SEALED}); err != nil {
		return err
	}
	<-stream.Context().Done()
	close(s.lost)
	return stream.Context().Err()
}
func TestUpdateDrainRejectsWrongTokenAndClosesStream(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	fixture := &mismatchedDrainServer{lost: make(chan struct{})}
	proto.RegisterAgentServer(server, fixture)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()
	conn, err := grpc.NewClient("passthrough:///buf", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	owner, err := OpenUpdateDrain(ctx, proto.NewAgentClient(conn), strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err = owner.Recv(); status.Code(err) != codes.DataLoss {
		t.Fatalf("mismatched operation accepted: %v", err)
	}
	select {
	case <-fixture.lost:
	case <-ctx.Done():
		t.Fatal("invalid response did not close owned stream")
	}
}
