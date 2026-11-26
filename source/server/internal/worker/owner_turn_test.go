package worker_test

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"cercano/source/server/internal/agenttools"
	providers "cercano/source/server/internal/hostsvc/providers"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/usage"
	"cercano/source/server/internal/worker"
	wire "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestOwnerTurnDescendantHelper(t *testing.T) {
	if os.Getenv("CERCANO_OWNER_TURN_CHILD") == "1" {
		time.Sleep(time.Hour)
	}
}

type ownerTurnProvider struct {
	*fixedProvider
	started  chan context.Context
	canceled chan struct{}
	reaped   chan error
	cleanup  <-chan struct{}
}

func (p *ownerTurnProvider) StreamChat(ctx context.Context, _ llm.ChatRequest) (llm.StreamReader, error) {
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOwnerTurnDescendantHelper$")
	child.Env = append(os.Environ(), "CERCANO_OWNER_TURN_CHILD=1")
	if err := child.Start(); err != nil {
		p.reaped <- err
		return nil, err
	}
	p.started <- ctx
	<-ctx.Done()
	close(p.canceled)
	p.reaped <- child.Wait()
	<-p.cleanup // deterministic producer cleanup/join barrier
	return nil, ctx.Err()
}

// Exercise the real RunTurn RPC, not direct registry registration. Accounting is
// legitimately absent when the host has no accounting manager.
func TestOwnerPipeMetadataFreeRunTurn(t *testing.T) {
	for _, stubborn := range []bool{false, true} {
		name := "cooperative"
		if stubborn {
			name = "noncooperative"
		}
		t.Run(name, func(t *testing.T) { testOwnerPipeMetadataFreeRunTurn(t, stubborn) })
	}
}

func testOwnerPipeMetadataFreeRunTurn(t *testing.T, stubborn bool) {
	cleanup := make(chan struct{})
	defer func() {
		select {
		case <-cleanup:
		default:
			close(cleanup)
		}
	}()
	p := &ownerTurnProvider{fixedProvider: &fixedProvider{}, started: make(chan context.Context, 2), canceled: make(chan struct{}), reaped: make(chan error, 2), cleanup: cleanup}
	ws := worker.NewWithFactories(func(*wire.StartTurn) (providers.Resolver, error) { return &fakeResolver{prov: p}, nil }, func(*wire.StartTurn) (runner.ToolSvc, error) { return &fakeToolSvc{reg: agenttools.NewRegistry()}, nil })
	listener := bufconn.Listen(1 << 20)
	defer listener.Close()
	server := grpc.NewServer()
	wire.RegisterWorkerServer(server, ws)
	go server.Serve(listener)
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///owner-turn", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	exited := make(chan int, 2)
	worker.WatchOwnerPipeForTest(r, func(code int) { exited <- code })(ws)
	start := func() wire.Worker_RunTurnClient {
		t.Helper()
		turn, err := wire.NewWorkerClient(conn).RunTurn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := turn.Send(&wire.HostToWorker{Msg: &wire.HostToWorker_Start{Start: &wire.StartTurn{Input: "hello", ConversationId: "owner", WorkDir: t.TempDir(), Config: &wire.ConfigSnapshot{LocusMode: "open_primary"}}}}); err != nil {
			t.Fatal(err)
		}
		return turn
	}
	turn := start()
	var turnCtx context.Context
	select {
	case turnCtx = <-p.started:
	case <-ctx.Done():
		t.Fatal("turn did not start")
	}
	if usage.AttemptsEnabled(turnCtx) {
		t.Fatal("invented accounting for metadata-free turn")
	}
	lostAt := time.Now()
	w.Close()
	select {
	case <-p.canceled:
	case code := <-exited:
		t.Fatalf("owner exited (%d) without canceling metadata-free RunTurn", code)
	case <-time.After(time.Second):
		t.Fatal("active turn not canceled")
	}
	select {
	case err := <-p.reaped:
		if err == nil {
			t.Fatal("descendant exited successfully instead of being canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("descendant not reaped")
	}
	late := start()
	for {
		_, err := late.Recv()
		if err != nil {
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("late turn: %v", err)
			}
			break
		}
	}
	select {
	case <-exited:
		t.Fatal("exit before producer cleanup joined")
	default:
	}
	if stubborn {
		select {
		case code := <-exited:
			if code != 1 {
				t.Fatalf("exit=%d", code)
			}
			if time.Since(lostAt) < time.Second {
				t.Fatal("cleanup budget was not exercised")
			}
		case <-time.After(4 * time.Second):
			t.Fatal("noncooperative turn prevented bounded exit")
		}
	}
	close(cleanup)
	for {
		_, err := turn.Recv()
		if err != nil {
			if err != io.EOF {
				t.Fatalf("active turn: %v", err)
			}
			break
		}
	}
	if stubborn {
		return
	}
	select {
	case code := <-exited:
		if code != 1 {
			t.Fatalf("exit=%d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup did not join")
	}
}
