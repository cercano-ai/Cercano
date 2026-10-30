package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// Run sequentially: signal.Notify captures the actual startup-installed legacy
// callback without replacing that hook or allowing SIGTERM to kill the tests.
// A canceled stream still registers and removes a subscriber synchronously.
type idleDisconnectedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s idleDisconnectedStream) Context() context.Context    { return s.ctx }
func (idleDisconnectedStream) Send(*proto.ClientEvent) error { panic("unexpected event") }

func checkStartupLastClientShutdown(t *testing.T, srv *server.Server, want bool) {
	t.Helper()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.SubscribeEvents(&proto.SubscribeEventsRequest{}, idleDisconnectedStream{ctx: ctx}); !errors.Is(err, context.Canceled) {
		t.Fatalf("disconnect: %v", err)
	}
	select {
	case <-signals:
		if !want {
			t.Error("last subscribed client triggered legacy SIGTERM on generation host")
		}
	case <-time.After(2500 * time.Millisecond):
		if want {
			t.Error("legacy singleton last-client shutdown did not signal")
		}
	}
}

func TestProductionGenerationLastClientDoesNotShutdown(t *testing.T) {
	for _, independent := range []bool{false, true} {
		name := "retained-session"
		if independent {
			name = "independent-work"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("CERCANO_AUTOLAUNCHED", "1")
			srv := startOrdinaryGenerationTest(t)
			if !srv.ConfigSnapshot().Agent.ShutdownOnLastClient {
				t.Fatal("shutdown configuration not exercised")
			}
			owner, err := srv.Retain(context.Background(), "idle-regression")
			if err != nil {
				t.Fatal(err)
			}
			var ticket *broker.WorkTicket
			if independent {
				_, ticket, err = srv.BeginRetainedWork(context.Background(), owner, broker.TaskAcceptanceWork)
				if err != nil {
					t.Fatal(err)
				}
			}
			checkStartupLastClientShutdown(t, srv, false)
			retained, err := srv.ListRetained()
			if err != nil || len(retained) != 1 {
				t.Fatalf("disconnect lost retained owner: %v %v", retained, err)
			}
			if err := srv.RetireCoexistence(context.Background()); !errors.Is(err, broker.ErrAdmissionBusy) {
				t.Fatalf("retired with retained session/work: %v", err)
			}
			if ticket != nil {
				if err := ticket.Complete(); err != nil {
					t.Fatal(err)
				}
			}
			if err := srv.ReleaseRetained(context.Background(), owner, nil); err != nil {
				t.Fatal(err)
			}
			if err := srv.RetireCoexistence(context.Background()); err != nil {
				t.Fatalf("retire after release and drain: %v", err)
			}
		})
	}
}
