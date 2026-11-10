package server

import (
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"context"
	"fmt"
	"google.golang.org/grpc/peer"
	"net"
	"testing"
)

type containedFixture struct {
	stop   int
	held   bool
	refuse bool
}

func (*containedFixture) Prepare(context.Context, *pb.LifecycleIdentity) (string, error) {
	return "qualified-fixture", nil
}
func (*containedFixture) Stop(context.Context, *pb.LifecycleIdentity, string, func() error) error {
	return fmt.Errorf("uncontained stop forbidden")
}
func (p *containedFixture) WithStoppedSource(ctx context.Context, _ *pb.LifecycleIdentity, _ string, validate func() error, use func(context.Context) error) error {
	if e := validate(); e != nil {
		return e
	}
	if p.refuse {
		return fmt.Errorf("uncertain stop")
	}
	p.stop++
	p.held = true
	defer func() { p.held = false }()
	return use(ctx)
}
func TestLiveSourceRecovery(t *testing.T) {
	for _, mode := range []string{"recover", "reserved", "stale", "uncertain"} {
		t.Run(mode, func(t *testing.T) {
			ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
			a, b, _ := lifecycleFixture(t)
			owner, e := a.Retain(ctx, "live")
			if e != nil {
				t.Fatal(e)
			}
			cap := a.coexistence.owners["live"]
			if e = a.coexistence.store.EnsureConversation(conversation.WithOwnership(ctx, cap), "live", "", ""); e != nil {
				t.Fatal(e)
			}
			if mode == "reserved" {
				e = cap.ReserveHandover(ctx, "historical-destination", "historical-transfer")
			} else {
				e = cap.Close()
			}
			if e != nil {
				t.Fatal(e)
			}
			p := &containedFixture{refuse: mode == "uncertain"}
			l := b.coexistence
			l.options.LegacyStop = p
			l.options.ControlAuthorize = func(context.Context) error { return nil }
			l.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
				return a.coexistence.controlIdentity(), "127.0.0.1:1", nil
			}
			l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
				return &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: "127.0.0.1:2", TokenFile: "fixture-token"}, nil
			}
			l.options.Reload = func(context.Context, conversation.Store, *conversation.Ownership) error {
				if !p.held {
					t.Error("exclusion released before reload")
				}
				return nil
			}
			r := &pb.LifecycleControlRequest{Target: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "live"}, Requester: l.controlIdentity()}
			before, _ := conversation.ReadOwnership(ctx, l.store, "live")
			offer, e := b.LifecycleResume(ctx, r)
			if e != nil || !offer.GetConfirmationRequired() {
				t.Fatalf("offer %v %v", offer, e)
			}
			after, _ := conversation.ReadOwnership(ctx, l.store, "live")
			if before != after || p.stop != 0 {
				t.Fatal("offer mutated source")
			}
			r.RecoveryChallenge = offer.OriginalRecovery.Challenge
			r.ConfirmOriginalRecovery = true
			if mode == "stale" {
				r.Requester = a.coexistence.controlIdentity()
			}
			result, e := b.LifecycleResume(ctx, r)
			if mode != "recover" && mode != "reserved" {
				if e == nil || p.stop != 0 {
					t.Fatalf("unsafe confirmation %v %v", result, e)
				}
				after, _ = conversation.ReadOwnership(ctx, l.store, "live")
				if after != before {
					t.Fatal("refusal mutated owner")
				}
				return
			}
			if e != nil || !result.GetCurrentOwner() || !result.GetReady() || result.Owner.Host != l.options.Host || result.Owner.Epoch == owner.Epoch {
				t.Fatalf("recover %v %v", result, e)
			}
			r.RecoveryChallenge = ""
			r.ConfirmOriginalRecovery = false
			retry, e := b.LifecycleResume(ctx, r)
			if e != nil || retry.GetOwner().GetEpoch() != result.Owner.Epoch || p.stop != 1 {
				t.Fatalf("lost response %v %v", retry, e)
			}
		})
	}
}
