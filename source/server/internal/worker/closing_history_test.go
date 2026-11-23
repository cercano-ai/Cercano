package worker_test

import (
	"context"
	"errors"
	"net"
	"sync"
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
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type closingHistoryWorker struct {
	proto.UnimplementedWorkerServer
	entered <-chan struct{}
	mode    string
}

func (w *closingHistoryWorker) RunTurn(stream proto.Worker_RunTurnServer) error {
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
	if w.mode == "authority-cancel" {
		return stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_DispatchEvent{DispatchEvent: &proto.DispatchEventRequest{ConversationId: "unauthorized"}}})
	}
	if w.mode == "execution-error" {
		return errors.New("fixture execution failed")
	}
	if w.mode == "cancel" || w.mode == "cancel-budget" {
		<-stream.Context().Done()
		return stream.Context().Err()
	}
	// A clean TurnDone + EOF while the read-only callback remains in flight.
	return stream.Send(&proto.WorkerToHost{Msg: &proto.WorkerToHost_Done{Done: &proto.TurnDone{}}})
}

func TestClosingTurnJoinsReadOnlyHistoryWithoutCleanupFailure(t *testing.T) {
	for _, mode := range []string{"complete", "execution-error", "cancel", "cancel-budget", "authority-cancel"} {
		t.Run(mode, func(t *testing.T) {
			entered, canceled, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseHistory := func() { releaseOnce.Do(func() { close(release) }) }
			lis := bufconn.Listen(1 << 20)
			srv := grpc.NewServer()
			proto.RegisterWorkerServer(srv, &closingHistoryWorker{entered: entered, mode: mode})
			served := make(chan struct{})
			go func() { defer close(served); _ = srv.Serve(lis) }()
			defer func() { srv.Stop(); _ = lis.Close(); <-served }()
			connections := make(chan *grpc.ClientConn, 1)
			dial := func(ctx context.Context) (*grpc.ClientConn, error) {
				conn, err := grpc.NewClient("passthrough:///history-barrier", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()))
				connections <- conn
				return conn, err
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
			a := &closingHistoryAuthority{exited: exited}
			wr.(worker.TurnAuthoritySetter).SetTurnAuthority(func(context.Context, string) (worker.TurnAuthority, error) { return a, nil })
			deadline, finish := context.WithTimeout(t.Context(), 7*time.Second)
			defer finish()
			ctx, cancel := context.WithCancel(deadline)
			defer cancel()
			a.reject = func() { cancel() }
			done := make(chan struct{})
			dir := t.TempDir()
			var turnErr error
			go func() {
				defer close(done)
				_, turnErr = wr.RunTurn(ctx, runner.Request{ConversationID: "history", Input: "hi", WorkDir: dir}, &sinkCollector{}, nil, nil)
			}()
			defer func() {
				cancel()
				releaseHistory()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("turn goroutine failed to join")
				}
			}()
			if mode == "cancel" || mode == "cancel-budget" {
				select {
				case <-entered:
					cancel()
				case <-deadline.Done():
					t.Fatal("history not entered")
				}
			}
			select {
			case <-canceled:
			case <-deadline.Done():
				releaseHistory()
				t.Fatal("callback never reached cancellation barrier")
			}
			conn := <-connections
			if mode == "execution-error" || mode == "cancel-budget" || mode == "authority-cancel" {
				// Containment must finish while the deliberately uncooperative history
				// callback still owns authority. Ordinary cancellation gets the drain budget.
				budget := 5 * time.Second
				if mode == "authority-cancel" {
					budget = 200 * time.Millisecond
				}
				limit := time.After(budget)
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
			waitRetirement:
				for conn.GetState() != connectivity.Shutdown {
					select {
					case <-ticker.C:
					case <-limit:
						t.Error("worker not retired with callback blocked")
						break waitRetirement
					}
				}
			}

			if mode == "complete" && conn.GetState() == connectivity.Shutdown {
				t.Error("clean transport destroyed before callback join")
			}
			early := false
			select {
			case <-done:
				early = true
			default:
			}
			releaseHistory()
			select {
			case <-done:
			case <-deadline.Done():
				t.Fatal("turn failed to join released callback")
			}
			<-exited
			if conn.GetState() != connectivity.Shutdown {
				t.Error("transport not destroyed after callback join")
			}
			var cleanup *worker.CallbackCleanupError
			if errors.As(a.err, &cleanup) {
				t.Fatalf("response delivery is not cleanup uncertainty: %v", a.err)
			}
			if mode == "complete" && turnErr != nil {
				t.Fatalf("completed turn: %v", turnErr)
			}
			if mode != "complete" && turnErr == nil {
				t.Fatal("lost execution/cancellation failure")
			}
			if early || a.closedEarly {
				t.Fatal("RunTurn returned before history callback completed; response send escaped callback join")
			}
		})
	}
}

type closingHistoryAuthority struct {
	err         error
	exited      <-chan struct{}
	closedEarly bool
	reject      func()
}

func (a *closingHistoryAuthority) Context(ctx context.Context, cid string) (context.Context, error) {
	if cid == "unauthorized" {
		a.reject()
		return nil, errors.New("authority scope violation")
	}
	return ctx, nil
}
func (a *closingHistoryAuthority) Close(err error) {
	a.err = err
	select {
	case <-a.exited:
	default:
		a.closedEarly = true
	}
}
