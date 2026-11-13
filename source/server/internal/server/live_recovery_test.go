package server

import (
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"context"
	"database/sql"
	"fmt"
	"google.golang.org/grpc/peer"
	"net"
	"testing"
	"time"
)

type containedFixture struct {
	stop       int
	held       bool
	refuse     bool
	beforeStop func()
}

func (*containedFixture) Prepare(context.Context, *pb.LifecycleIdentity) (string, error) {
	return "qualified-fixture", nil
}
func (p *containedFixture) WithStoppedSource(ctx context.Context, _ *pb.LifecycleIdentity, _ string, validate func() error, use func(context.Context) error) error {
	if p.beforeStop != nil {
		p.beforeStop()
	}
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
	for _, mode := range []string{"recover", "reserved", "stale", "uncertain", "expiry", "decline", "refreshed", "cross-conversation"} {
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
			l.options.RecoverySource = p
			l.options.ControlAuthorize = func(context.Context) error { return nil }
			l.options.RecoveryResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
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
			switch mode {
			case "expiry":
				l.recoveryOffersMu.Lock()
				old := *l.recoveryOffers[r.RecoveryChallenge]
				old.expires = time.Now().Add(-time.Second)
				l.recoveryOffers[r.RecoveryChallenge] = &old
				l.recoveryOffersMu.Unlock()
			case "cross-conversation":
				if _, err := a.Retain(ctx, "other"); err != nil {
					t.Fatal(err)
				}
				r.Owner = &pb.LifecycleOwner{Conversation: "other"}
			case "decline":
				r.ConfirmOriginalRecovery = false
			case "refreshed":
				fresh, err := b.LifecycleResume(ctx, &pb.LifecycleControlRequest{Target: l.controlIdentity(), Owner: r.Owner, Requester: r.Requester})
				if err != nil || fresh.GetOriginalRecovery().GetChallenge() == r.RecoveryChallenge {
					t.Fatalf("fresh offer: %v %v", fresh, err)
				}
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
				// A declined, expired, stale or failed attempt cannot be replayed.
				r.Requester = l.controlIdentity()
				r.Owner = &pb.LifecycleOwner{Conversation: "live"}
				r.ConfirmOriginalRecovery = true
				replay, err := b.LifecycleResume(ctx, r)
				if err == nil || replay != nil || p.stop != 0 {
					t.Fatalf("replayed consent: %v %v", replay, err)
				}
				return
			}
			if e != nil || !result.GetCurrentOwner() || !result.GetReady() || result.Owner.Host != l.options.Host || result.Owner.Epoch == owner.Epoch {
				t.Fatalf("recover %v %v", result, e)
			}
			r.RecoveryChallenge = ""
			r.ConfirmOriginalRecovery = false
			retry, e := b.LifecycleResume(ctx, r)
			if e != nil || result.OperationId == "" || retry.OperationId != result.OperationId || retry.GetOwner().GetEpoch() != result.Owner.Epoch || p.stop != 1 {
				t.Fatalf("lost response %v %v", retry, e)
			}
			again, e := b.LifecycleResume(ctx, r)
			if e != nil || !again.GetReady() || again.OperationId != result.OperationId || again.GetOwner().GetEpoch() != result.Owner.Epoch || p.stop != 1 {
				t.Fatalf("repeated ready resume: %v %v", again, e)
			}
			// A later ordinary acquisition must not resurrect this receipt.
			if e = l.owners["live"].Release(ctx); e != nil {
				t.Fatal(e)
			}
			delete(l.owners, "live")
			later, e := conversation.AcquireOwnership(ctx, l.store, "live", l.options.Host)
			if e != nil {
				t.Fatal(e)
			}
			defer later.Close()
			current, e := conversation.ReadOwnership(ctx, l.store, "live")
			if e != nil || current.Epoch == result.Owner.Epoch {
				t.Fatalf("new epoch: %v %v", current, e)
			}
			for _, epoch := range []string{current.Epoch, result.Owner.Epoch} {
				operation, err := conversation.CurrentRecoveryResult(ctx, l.store, "live", l.options.Host, epoch)
				if err == nil || operation != "" || p.stop != 1 {
					t.Fatalf("historic receipt resurrected: %q %v", operation, err)
				}
			}
		})
	}
}

func TestLiveRecoveryConsentMutationBeforeStop(t *testing.T) {
	for _, timing := range []string{"confirmation", "final-pre-stop"} {
		for _, field := range []string{"owner", "state", "epoch", "destination", "handover", "endpoint", "token", "source", "selected-identity"} {
			t.Run(timing+"/"+field, func(t *testing.T) {
				ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
				a, b, _ := lifecycleFixture(t)
				_, err := a.Retain(ctx, "consent")
				if err != nil {
					t.Fatal(err)
				}
				if err = a.coexistence.owners["consent"].ReserveHandover(ctx, "historical", "historical-op"); err != nil {
					t.Fatal(err)
				}
				l := b.coexistence
				p := &containedFixture{}
				l.options.RecoverySource = p
				l.options.ControlAuthorize = func(context.Context) error { return nil }
				source := a.coexistence.controlIdentity()
				selection := &pb.LifecycleRecoverySelection{Identity: l.controlIdentity(), Endpoint: "127.0.0.1:2", TokenFile: "token"}
				l.options.RecoveryResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
					return source, "127.0.0.1:1", nil
				}
				l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) { return selection, nil }
				r := &pb.LifecycleControlRequest{Target: l.controlIdentity(), Requester: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "consent"}}
				offer, err := b.LifecycleResume(ctx, r)
				if err != nil || !offer.GetConfirmationRequired() {
					t.Fatalf("offer %v %v", offer, err)
				}
				mutate := func() {
					switch field {
					case "selected-identity":
						selection.Identity.Build = "changed-build"
					case "endpoint":
						selection.Endpoint = "127.0.0.1:3"
					case "token":
						selection.TokenFile = "different-token"
					case "source":
						source.Build = "changed-build"
					default:
						db, e := sql.Open("sqlite", l.options.StorePath)
						if e != nil {
							t.Fatal(e)
						}
						defer db.Close()
						value := "changed"
						if field == "state" {
							value = "owned"
						}
						if _, e = db.Exec("UPDATE conversation_ownership SET "+field+"=? WHERE conversation_id=?", value, "consent"); e != nil {
							t.Fatal(e)
						}
					}
				}
				if timing == "confirmation" {
					mutate()
				} else {
					p.beforeStop = mutate
				}
				r.RecoveryChallenge = offer.OriginalRecovery.Challenge
				r.ConfirmOriginalRecovery = true
				got, err := b.LifecycleResume(ctx, r)
				if err == nil || p.stop != 0 {
					t.Fatalf("stale consent stopped source: %v %v stops=%d", got, err, p.stop)
				}
				state, e := conversation.ReadOwnership(ctx, l.store, "consent")
				if e != nil || state.Owner == l.options.Host {
					t.Fatalf("stale consent reassigned: %+v %v", state, e)
				}
			})
		}
	}
}

func TestResumeRecoveryLookupFailure(t *testing.T) {
	for _, mode := range []string{"ambiguous", "closed-db"} {
		t.Run(mode, func(t *testing.T) {
			ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
			_, b, _ := lifecycleFixture(t)
			l := b.coexistence
			l.options.ControlAuthorize = func(context.Context) error { return nil }
			p := &containedFixture{}
			l.options.RecoverySource = p
			fallback := 0
			l.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
				fallback++
				return nil, fmt.Errorf("unexpected fallback")
			}
			owner, err := b.Retain(ctx, "lookup")
			if err != nil {
				t.Fatal(err)
			}
			before, err := conversation.ReadOwnership(ctx, l.store, "lookup")
			if err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", l.options.StorePath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if mode == "ambiguous" {
				for _, receipt := range []string{"recovery-one", "recovery-two"} {
					if _, err = db.Exec(`INSERT INTO conversation_handovers(handover,conversation_id,destination,epoch) VALUES(?,?,?,?)`, receipt, "lookup", l.options.Host, owner.Epoch); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				if err = l.store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			r := &pb.LifecycleControlRequest{Target: l.controlIdentity(), Owner: &pb.LifecycleOwner{Conversation: "lookup"}}
			for i := 0; i < 2; i++ {
				got, err := b.LifecycleResume(ctx, r)
				if err == nil || got != nil {
					t.Fatalf("lookup failure accepted: %v %v", got, err)
				}
			}
			var after conversation.OwnershipState
			err = db.QueryRow(`SELECT owner,epoch,state,destination,handover FROM conversation_ownership WHERE conversation_id='lookup'`).Scan(&after.Owner, &after.Epoch, &after.State, &after.Destination, &after.Handover)
			if err != nil || before != after || p.stop != 0 || fallback != 0 {
				t.Fatalf("lookup failure mutated/fell back: %+v %v stops=%d fallback=%d", after, err, p.stop, fallback)
			}
		})
	}
}
