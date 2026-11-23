package worker_test

import (
	"context"
	"net"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type historyBarrierWorker struct {
	proto.UnimplementedWorkerServer
	entered <-chan struct{}
}

func (w *historyBarrierWorker) RunTurn(stream proto.Worker_RunTurnServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if err := stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_HistoryRequest{HistoryRequest: &proto.ConversationHistoryRequest{Id: "history", Query: "needle", Limit: 1}}}); err != nil {
		return err
	}
	select {
	case <-w.entered:
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	// EOF terminates the turn while the host callback is still blocked.
	return nil
}

func TestWorkerRunnerJoinsHistoryCallback(t *testing.T) {
	entered, canceled, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	proto.RegisterWorkerServer(srv, &historyBarrierWorker{entered: entered})
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()
	dial := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.NewClient("passthrough:///history-barrier", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	cfg := cfgsvc.New("", config.Config{LocusMode: "open_primary"}, secrets.NewMemory())
	wr := worker.NewWorkerRunnerForTest(&fakeHistory{}, cfg, newTestBroker(), newHostSecrets(nil), dial)
	wr.(worker.HistorySetter).SetHistory(conversation.HistoryReaderFunc(func(ctx context.Context, _ string, _ conversation.HistoryQuery) ([]conversation.HistoryEntry, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release // deliberately require cleanup after cancellation
		close(exited)
		return nil, ctx.Err()
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = wr.RunTurn(ctx, runner.Request{ConversationID: "history", Input: "hi", WorkDir: t.TempDir()}, &sinkCollector{}, nil, nil)
	}()
	select {
	case <-canceled:
	case <-ctx.Done():
		close(release)
		t.Fatal("callback never reached cancellation barrier")
	}
	early := false
	select {
	case <-done:
		early = true
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("turn failed to join released callback")
	}
	<-exited
	if early {
		t.Fatal("RunTurn returned before history callback completed; response send escaped callback join")
	}
}
