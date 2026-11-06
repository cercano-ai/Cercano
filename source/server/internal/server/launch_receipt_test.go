package server

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/peer"
	"net"
	"testing"

	"cercano/source/server/internal/broker"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func launchFixture(t *testing.T) (*Server, context.Context, *pb.LifecycleControlRequest) {
	t.Helper()
	a, _, _ := lifecycleFixture(t)
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}})
	a.coexistence.options.ControlAuthorize = func(context.Context) error { return nil }
	return a, ctx, &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "new"}, LaunchOperation: "operation", LaunchAction: "acquire"}
}
func acquireLaunch(t *testing.T, a *Server, ctx context.Context, r *pb.LifecycleControlRequest) *pb.LifecycleControlRequest {
	t.Helper()
	out, err := a.LifecycleAcquire(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Pending || out.Receipt != r.LaunchOperation || out.Owner.Epoch == "" {
		t.Fatalf("invalid receipt: %v", out)
	}
	next := proto.Clone(r).(*pb.LifecycleControlRequest)
	next.Owner = out.Owner
	return next
}
func TestLaunchReceiptAbortRetryAndJournal(t *testing.T) {
	a, ctx, r := launchFixture(t)
	exact := acquireLaunch(t, a, ctx, r)
	// Losing an acquire response can only recover the SAME epoch/operation.
	again, err := a.LifecycleAcquire(ctx, r)
	if err != nil || !proto.Equal(again.Owner, exact.Owner) {
		t.Fatalf("retry: %v %v", again, err)
	}
	exact.LaunchAction = "abort"
	a.coexistence.options.Cleanup = func(context.Context, broker.Owner) error { return errors.New("fixture cleanup failure") }
	if _, err = a.LifecycleAcquire(ctx, exact); err == nil {
		t.Fatal("cleanup failure hidden")
	}
	// Admission stays closed across failed cleanup; observation cannot reopen it.
	if _, err = a.AttachRetained(controlOwner(exact)); err == nil {
		t.Fatal("observed aborting owner")
	}
	a.coexistence.options.Cleanup = nil
	if _, err = a.LifecycleAcquire(ctx, exact); err == nil {
		t.Fatal("uncertain existing cleanup silently resolved")
	}
	// Generic lifecycle cleanup failure deliberately remains blocked. A separate
	// successful operation proves durable duplicate cleanup.
	a, ctx, r = launchFixture(t)
	exact = acquireLaunch(t, a, ctx, r)
	exact.LaunchAction = "abort"
	if _, err = a.LifecycleAcquire(ctx, exact); err != nil {
		t.Fatal(err)
	}
	// Reload the durable result, not only the in-memory duplicate result.
	a.coexistence.launches = nil
	if _, err = a.LifecycleAcquire(ctx, exact); err != nil {
		t.Fatal(err)
	}
	if len(a.coexistence.owners) != 0 {
		t.Fatal("owner leaked")
	}
}
func TestLaunchReceiptDeliveryAndObservationRetain(t *testing.T) {
	for _, observation := range []string{"deliver", "attach-detach", "list", "check", "reacquire"} {
		t.Run(observation, func(t *testing.T) {
			a, ctx, r := launchFixture(t)
			exact := acquireLaunch(t, a, ctx, r)
			var err error
			switch observation {
			case "deliver":
				exact.LaunchAction = "deliver"
				_, err = a.LifecycleAcquire(ctx, exact)
			case "attach-detach":
				var attachment *broker.Attachment
				attachment, err = a.AttachRetained(controlOwner(exact))
				if err == nil {
					attachment.Detach()
				}
			case "list":
				_, err = a.LifecycleList(ctx, &pb.LifecycleControlRequest{Target: r.Target})
			case "check":
				_, err = a.CheckRetainedSession(ctx, &pb.CheckRetainedSessionRequest{HostInstance: exact.Owner.Host, ConversationId: exact.Owner.Conversation, OwnershipEpoch: exact.Owner.Epoch})
			case "reacquire":
				_, err = a.LifecycleAcquire(ctx, &pb.LifecycleControlRequest{Target: r.Target, Owner: r.Owner})
			}
			if err != nil {
				t.Fatal(err)
			}
			exact.LaunchAction = "abort"
			if _, err = a.LifecycleAcquire(ctx, exact); err == nil {
				t.Fatal("delivered owner reclaimed")
			}
			if a.coexistence.owners[exact.Owner.Conversation] == nil {
				t.Fatal("valid owner lost")
			}
		})
	}
}
func TestLaunchReceiptExactEpochAndExisting(t *testing.T) {
	a, ctx, r := launchFixture(t)
	exact := acquireLaunch(t, a, ctx, r)
	wrong := proto.Clone(exact).(*pb.LifecycleControlRequest)
	wrong.LaunchAction = "abort"
	wrong.Owner.Epoch = "other"
	if _, err := a.LifecycleAcquire(ctx, wrong); err == nil {
		t.Fatal("wrong epoch accepted")
	}
	wrong = proto.Clone(exact).(*pb.LifecycleControlRequest)
	wrong.LaunchOperation = "other"
	wrong.LaunchAction = "abort"
	if _, err := a.LifecycleAcquire(ctx, wrong); err == nil {
		t.Fatal("wrong operation accepted")
	}
	exact.LaunchAction = "abort"
	if _, err := a.LifecycleAcquire(ctx, exact); err != nil {
		t.Fatal(err)
	}
	// The durable row remains an existing conversation even after release.
	r.LaunchOperation = "second-operation"
	if _, err := a.LifecycleAcquire(ctx, r); err == nil {
		t.Fatal("existing conversation received cleanup authority")
	}
	// A later ordinary acquisition is not fenced by the old aborted receipt.
	next, err := a.LifecycleAcquire(ctx, &pb.LifecycleControlRequest{Target: r.Target, Owner: &pb.LifecycleOwner{Conversation: r.Owner.Conversation}})
	if err != nil {
		t.Fatal(err)
	}
	if next.Owner.Epoch == exact.Owner.Epoch {
		t.Fatal("reacquisition reused epoch")
	}
	attachment, err := a.AttachRetained(controlOwner(&pb.LifecycleControlRequest{Owner: next.Owner}))
	if err != nil {
		t.Fatal("old receipt fenced new epoch", err)
	}
	attachment.Detach()
	if _, err := a.LifecycleAcquire(ctx, exact); err != nil {
		t.Fatal("duplicate old abort", err)
	}
	if a.coexistence.owners[next.Owner.Conversation] == nil {
		t.Fatal("duplicate old abort reclaimed new epoch")
	}
}
func TestLaunchReceiptAttachAbortRace(t *testing.T) {
	for i := 0; i < 25; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			a, ctx, r := launchFixture(t)
			exact := acquireLaunch(t, a, ctx, r)
			exact.LaunchAction = "abort"
			start := make(chan struct{})
			attached := make(chan error, 1)
			aborted := make(chan error, 1)
			go func() {
				<-start
				att, err := a.AttachRetained(controlOwner(exact))
				if err == nil {
					att.Detach()
				}
				attached <- err
			}()
			go func() { <-start; _, err := a.LifecycleAcquire(ctx, exact); aborted <- err }()
			close(start)
			ae, be := <-attached, <-aborted
			if (ae == nil) == (be == nil) {
				t.Fatalf("exactly one must win: attach=%v abort=%v", ae, be)
			}
			if ae == nil && a.coexistence.owners[exact.Owner.Conversation] == nil {
				t.Fatal("observer winner reclaimed")
			}
		})
	}
}
