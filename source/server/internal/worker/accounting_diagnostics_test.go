package worker

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/telemetry"
	"cercano/source/server/internal/usage"
	wire "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type diagnosticAccountingServer struct {
	wire.UnimplementedWorkerServer
	calls atomic.Int32
}

func (s *diagnosticAccountingServer) Accounting(stream wire.Worker_AccountingServer) error {
	if s.calls.Add(1)%2 == 1 {
		return status.Error(codes.Unavailable, "SECRET credential target /private/path")
	}
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if err := stream.Send(&wire.WorkerAccountingBatch{}); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != nil {
		return err
	}
	return stream.Send(&wire.WorkerAccountingBatch{DrainFinished: true, DrainError: "SECRET credential receipt"})
}

type diagnosticReceiver struct{ gaps int }

func (*diagnosticReceiver) TryAttemptBatch([]usage.AttemptObservation) (<-chan bool, error) {
	return nil, nil
}
func (*diagnosticReceiver) TryRemoteAccountingHealth(string, telemetry.AccountingHealth) (<-chan bool, error) {
	return nil, nil
}
func (r *diagnosticReceiver) MarkAccountingIncomplete(string) { r.gaps++ }

func TestAccountingDiagnosticCorrelationAndPrivacy(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	wire.RegisterWorkerServer(server, &diagnosticAccountingServer{})
	go server.Serve(listener)
	defer server.Stop()
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	sources := []string{usage.NewIdentity(), usage.NewIdentity()}
	for _, source := range sources {
		// Each terminal accounting session owns and closes its transport.
		conn, err := grpc.NewClient("passthrough:///SECRET-target", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		receiver := &diagnosticReceiver{}
		s := &hostAccountingSession{conn: conn, sink: receiver, source: source, ctx: ctx, cancel: cancel, drain: make(chan struct{}), done: make(chan struct{})}
		s.requestDrain()
		s.run()
		cancel()
		s.gap("must not log twice")
		if s.err != errAccountingDrainIncomplete {
			t.Fatalf("drain result: %v", s.err)
		}
		// The receiver also marks the protocol drain failure; gapOnce itself is
		// still exactly once, despite repeated diagnostic requests.
		if receiver.gaps != 2 {
			t.Fatalf("gapOnce changed: %d", receiver.gaps)
		}
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("unexpected warning count: %s", output.String())
	}
	for i, source := range sources {
		for j, reason := range []string{"delivery_unavailable", "recovered", "worker accounting drain incomplete"} {
			line := lines[i*3+j]
			if !strings.Contains(line, "reason=\""+reason+"\"") || !strings.Contains(line, "worker_connection_id=\"worker/"+source+"\"") {
				t.Fatalf("bad correlation: %s", line)
			}
		}
	}
	for _, bad := range []string{"SECRET", "credential", "receipt", "/private/path", "target=", "conversation_id", "ownership_epoch", "host_id", "must not log twice"} {
		if strings.Contains(output.String(), bad) {
			t.Fatalf("unsafe accounting log: %s", output.String())
		}
	}
}
