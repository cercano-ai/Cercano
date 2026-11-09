package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Regression: changing the selected host must not mint a competing operation.
func TestResumeDiscoversOriginalBeforeMint(t *testing.T) {
	ctx, cancel := context.WithTimeout(recoveryFixturePeer(context.Background()), 10*time.Second)
	defer cancel()
	a, b, c := lifecycleFixture(t)
	endpoints := inMemoryRecoveryControls(t, a, b, c)
	c.coexistence.options.ResolveSource = func(_ context.Context, host string) (*pb.LifecycleIdentity, string, error) {
		for _, s := range []*Server{a, b} {
			if s.coexistence.controlIdentity().Host == host {
				return s.coexistence.controlIdentity(), endpoints[s], nil
			}
		}
		return nil, "", fmt.Errorf("unknown host")
	}
	c.coexistence.options.RecoverySelection = func(_ context.Context, host string) (*pb.LifecycleRecoverySelection, error) {
		return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: endpoints[b], TokenFile: "fixture-token"}, nil
	}
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	held, err := a.AdmitRetainedWork(old, broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.LifecycleResume(ctx, resumeRequest(b, endpoints[b]))
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatal(err)
	}
	if err = held.Complete(); err != nil {
		t.Fatal(err)
	}
	got, err := c.LifecycleResume(ctx, resumeRequest(c, endpoints[c]))
	if err != nil || !got.GetConfirmationRequired() {
		t.Fatalf("expected original-operation offer, got %v %v", got, err)
	}
	if _, err = os.Stat(resumeJournal(c.coexistence, "chat")); !os.IsNotExist(err) {
		t.Fatalf("competing journal created: %v", err)
	}
}

func TestOriginalRecoveryConfirmedJourney(t *testing.T) {
	for _, mode := range []struct{ legacy, ordinaryOnly, uncertain bool }{{}, {legacy: true}, {legacy: true, ordinaryOnly: true}, {uncertain: true}} {
		legacy := mode.legacy
		t.Run(fmt.Sprintf("legacy=%t/ordinary-only=%t/uncertain=%t", legacy, mode.ordinaryOnly, mode.uncertain), func(t *testing.T) {
			ctx := recoveryFixturePeer(context.Background())
			a, b, c := lifecycleFixture(t)
			endpoints := inMemoryRecoveryControls(t, a, b, c, legacy, mode.ordinaryOnly)
			c.coexistence.options.ResolveSource = func(_ context.Context, host string) (*pb.LifecycleIdentity, string, error) {
				for _, s := range []*Server{a, b} {
					if s.coexistence.controlIdentity().Host == host {
						return s.coexistence.controlIdentity(), endpoints[s], nil
					}
				}
				return nil, "", fmt.Errorf("unknown peer")
			}
			c.coexistence.options.RecoverySelection = func(_ context.Context, host string) (*pb.LifecycleRecoverySelection, error) {
				return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: endpoints[b], TokenFile: "fixture-token"}, nil
			}
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			writeCtx, writeTicket, err := a.BeginRetainedWork(ctx, old, broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.coexistence.store.EnsureConversation(writeCtx, "chat", "", ""); err != nil {
				t.Fatal(err)
			}
			for i, content := range []string{"first user message", "first assistant reply", "second user message"} {
				role := "user"
				if i == 1 {
					role = "assistant"
				}
				if err := a.coexistence.store.Append(writeCtx, conversation.Turn{ID: fmt.Sprint(i), ConversationID: "chat", Role: role, Content: content}); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeTicket.Complete(); err != nil {
				t.Fatal(err)
			}
			before, err := a.coexistence.store.GetTurns(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			if legacy {
				// No source closure/reservation exists: the separately approved legacy
				// controls must be allowed to initiate exactly this original operation.
				op := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoints[b], Owner: wireOwner(old), Management: true, HandoverId: "original-legacy-op"}
				accepted, err := b.LifecyclePrepare(ctx, op)
				if err != nil {
					t.Fatal(err)
				}
				op.Receipt = accepted.Receipt
				op.Target = op.Source
				if err := saveResume(resumeJournal(b.coexistence, "chat"), op); err != nil {
					t.Fatal(err)
				}
			} else {
				held, err := a.AdmitRetainedWork(old, broker.PersistenceWork)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := b.LifecycleResume(ctx, resumeRequest(b, endpoints[b])); status.Code(err) != codes.DeadlineExceeded {
					t.Fatal(err)
				}
				if mode.uncertain {
					if err := held.PersistenceFailed(); err != nil {
						t.Fatal(err)
					}
					if err := held.Complete(); err == nil {
						t.Fatal("uncertain write completed successfully")
					}
				} else if err := held.Complete(); err != nil {
					t.Fatal(err)
				}
			}
			offered, err := c.LifecycleResume(ctx, resumeRequest(c, endpoints[c]))
			if err != nil {
				t.Fatal(err)
			}
			o := offered.GetOriginalRecovery()
			if !offered.GetConfirmationRequired() || o.GetLegacy() != legacy || !proto.Equal(o.GetSelection().GetIdentity(), b.coexistence.controlIdentity()) {
				t.Fatalf("bad offer: %v", offered)
			}
			confirm := resumeRequest(c, endpoints[c])
			confirm.RecoveryChallenge = o.Challenge
			confirm.ConfirmOriginalRecovery = true
			confirm.ConfirmLegacyRecovery = legacy
			got, err := c.LifecycleResume(ctx, confirm)
			if mode.uncertain {
				if status.Code(err) != codes.FailedPrecondition {
					t.Fatalf("uncertain writes recovered: %v %v", got, err)
				}
				v, readErr := conversation.ReadOwnership(ctx, a.coexistence.store, "chat")
				if readErr != nil || v.Owner != old.Host || v.Epoch != old.Epoch {
					t.Fatalf("uncertain writes changed owner: %v %v", v, readErr)
				}
				if _, statErr := os.Stat(resumeJournal(c.coexistence, "chat")); !os.IsNotExist(statErr) {
					t.Fatalf("competing journal: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !got.GetReady() || got.GetOwner().GetHost() != oldDestination(b) || got.GetOwner().GetEpoch() == old.Epoch || got.GetOperationId() != o.Operation.HandoverId {
				t.Fatalf("bad original result: %v", got)
			}
			if _, err := c.LifecycleResume(ctx, confirm); status.Code(err) != codes.PermissionDenied {
				t.Fatalf("consent replay accepted: %v", err)
			}
			after, err := b.coexistence.store.GetTurns(ctx, "chat")
			if err != nil || fmt.Sprint(before) != fmt.Sprint(after) {
				t.Fatalf("ordered persisted history changed: %v %v", after, err)
			}
			if _, err := os.Stat(resumeJournal(c.coexistence, "chat")); !os.IsNotExist(err) {
				t.Fatalf("competing operation: %v", err)
			}
			epoch, err := conversation.HandoverResult(ctx, b.coexistence.store, "chat", b.coexistence.controlIdentity().Host, o.Operation.HandoverId)
			if err != nil || epoch != got.Owner.Epoch {
				t.Fatalf("missing exact durable handover: %q %v", epoch, err)
			}
		})
	}
}
func oldDestination(s *Server) string { return s.coexistence.controlIdentity().Host }

// Malicious journals can select candidates, never authorize a transfer.
func TestOriginalRecoveryRejectsUnsafeHints(t *testing.T) {
	for _, change := range []string{"epoch", "source", "operation", "destination", "receipt", "ambiguous", "unavailable", "selection", "missing-acceptance", "changed-after-offer"} {
		t.Run(change, func(t *testing.T) {
			ctx := recoveryFixturePeer(context.Background())
			a, b, c := lifecycleFixture(t)
			endpoints := inMemoryRecoveryControls(t, a, b, c, true)
			c.coexistence.options.ResolveSource = func(_ context.Context, host string) (*pb.LifecycleIdentity, string, error) {
				for _, s := range []*Server{a, b} {
					if s.coexistence.controlIdentity().Host == host {
						return s.coexistence.controlIdentity(), endpoints[s], nil
					}
				}
				return nil, "", fmt.Errorf("unknown provenance")
			}
			c.coexistence.options.RecoverySelection = func(_ context.Context, host string) (*pb.LifecycleRecoverySelection, error) {
				return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: endpoints[b], TokenFile: "fixture-token"}, nil
			}
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			op := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), Owner: wireOwner(old), DestinationEndpoint: endpoints[b], Management: true, HandoverId: "original"}
			accepted, err := b.LifecyclePrepare(ctx, op)
			if err != nil {
				t.Fatal(err)
			}
			op.Target = op.Source
			op.Receipt = accepted.Receipt
			path := resumeJournal(b.coexistence, "chat")
			if err := saveResume(path, op); err != nil {
				t.Fatal(err)
			}
			var confirm *pb.LifecycleControlRequest
			if change == "changed-after-offer" {
				offer, err := c.LifecycleResume(ctx, resumeRequest(c, endpoints[c]))
				if err != nil {
					t.Fatal(err)
				}
				confirm = resumeRequest(c, endpoints[c])
				confirm.ConfirmOriginalRecovery = true
				confirm.ConfirmLegacyRecovery = true
				confirm.RecoveryChallenge = offer.OriginalRecovery.Challenge
			}
			op = proto.Clone(op).(*pb.LifecycleControlRequest)
			switch change {
			case "epoch":
				op.Owner.Epoch = "wrong"
			case "source":
				op.Source = proto.Clone(op.Source).(*pb.LifecycleIdentity)
				op.Source.Host = "unknown"
				op.Target = op.Source
				op.Owner.Host = "unknown"
			case "operation":
				op.HandoverId = "wrong"
			case "destination":
				op.Destination = proto.Clone(op.Destination).(*pb.LifecycleIdentity)
				op.Destination.Build = "wrong"
			case "receipt", "changed-after-offer":
				op.Receipt = "wrong"
			case "ambiguous":
				if err := saveResume(path+".extra.json", op); err != nil {
					t.Fatal(err)
				}
			case "unavailable":
				c.coexistence.options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
					return nil, "", fmt.Errorf("offline peer unavailable")
				}
			case "selection":
				c.coexistence.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
					return &pb.LifecycleRecoverySelection{Identity: a.coexistence.controlIdentity(), Endpoint: endpoints[a], TokenFile: "fixture"}, nil
				}
			case "missing-acceptance":
				b.coexistence.mu.Lock()
				b.coexistence.incoming = nil
				b.coexistence.mu.Unlock()
			}
			if err := saveResume(path, op); err != nil {
				t.Fatal(err)
			}
			req := resumeRequest(c, endpoints[c])
			if confirm != nil {
				req = confirm
			}
			if got, err := c.LifecycleResume(ctx, req); err == nil {
				t.Fatalf("unsafe hint accepted: %v", got)
			}
			v, err := conversation.ReadOwnership(ctx, a.coexistence.store, "chat")
			if err != nil || v.Owner != old.Host || v.Epoch != old.Epoch || v.State != "owned" {
				t.Fatalf("unsafe hint mutated owner: %v %v", v, err)
			}
			if _, err := os.Stat(resumeJournal(c.coexistence, "chat")); !os.IsNotExist(err) {
				t.Fatalf("competing operation created: %v", err)
			}
		})
	}
}

// A competing attempt may finish and release after confirmation's initial owner
// check but before its claim. A historical handover is not current ownership.
func TestOriginalRecoveryRejectsHistoricalClaim(t *testing.T) {
	for _, mode := range []struct {
		legacy bool
		after  string
	}{{false, "released"}, {true, "released"}, {false, "new-epoch"}, {true, "new-epoch"}, {false, "moved"}, {true, "moved"}} {
		t.Run(fmt.Sprintf("legacy=%t/%s", mode.legacy, mode.after), func(t *testing.T) {
			legacy := mode.legacy
			ctx := recoveryFixturePeer(context.Background())
			a, b, c := lifecycleFixture(t)
			var interleaved atomic.Bool
			endpoints := inMemoryRecoveryControlsWithHook(t, a, b, c, func(method string, req any) {
				if !strings.HasSuffix(method, "/LifecycleClaim") && !strings.HasSuffix(method, "/LifecycleRetryClaim") {
					return
				}
				if !interleaved.CompareAndSwap(false, true) {
					return
				}
				claimed, err := b.LifecycleClaim(ctx, req.(*pb.LifecycleControlRequest))
				if err != nil {
					t.Error(err)
					return
				}
				if err := b.ReleaseRetained(ctx, controlOwner(&pb.LifecycleControlRequest{Owner: claimed.Owner}), nil); err != nil {
					t.Error(err)
				}
				if mode.after != "released" {
					next := b
					if mode.after == "moved" {
						next = c
					}
					if _, err := next.Retain(ctx, "chat"); err != nil {
						t.Error(err)
					}
				}
			}, legacy)
			configureOriginalRecoveryFixture(c, a, b, endpoints)
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			op := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoints[b], Owner: wireOwner(old), Management: true, HandoverId: "historical-original"}
			accepted, err := b.LifecyclePrepare(ctx, op)
			if err != nil {
				t.Fatal(err)
			}
			op.Receipt = accepted.Receipt
			op.Target = op.Source
			if err := saveResume(resumeJournal(b.coexistence, "chat"), op); err != nil {
				t.Fatal(err)
			}
			if !legacy {
				if _, err := a.LifecycleForceRelinquish(ctx, op); err != nil {
					t.Fatal(err)
				}
			}
			offer, err := c.LifecycleResume(ctx, resumeRequest(c, endpoints[c]))
			if err != nil {
				t.Fatal(err)
			}
			req := resumeRequest(c, endpoints[c])
			req.RecoveryChallenge = offer.OriginalRecovery.Challenge
			req.ConfirmOriginalRecovery = true
			req.ConfirmLegacyRecovery = legacy
			got, err := c.LifecycleResume(ctx, req)
			if !interleaved.Load() {
				t.Fatal("claim interleaving not reached")
			}
			if err == nil || got.GetReady() {
				t.Fatalf("historical claim reported ready: %v err=%v", got, err)
			}
		})
	}
}

func configureOriginalRecoveryFixture(c, a, b *Server, endpoints map[*Server]string) {
	c.coexistence.options.ResolveSource = func(_ context.Context, host string) (*pb.LifecycleIdentity, string, error) {
		for _, s := range []*Server{a, b} {
			if s.coexistence.controlIdentity().Host == host {
				return s.coexistence.controlIdentity(), endpoints[s], nil
			}
		}
		return nil, "", fmt.Errorf("unknown fixture host")
	}
	c.coexistence.options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
		return &pb.LifecycleRecoverySelection{Identity: b.coexistence.controlIdentity(), Endpoint: endpoints[b], TokenFile: "fixture-token"}, nil
	}
}

func TestOriginalRecoveryReaffirmsJournalDurability(t *testing.T) {
	ctx := recoveryFixturePeer(context.Background())
	a, b, c := lifecycleFixture(t)
	var mutations atomic.Int32
	endpoints := inMemoryRecoveryControlsWithHook(t, a, b, c, func(method string, _ any) {
		if strings.HasSuffix(method, "Relinquish") {
			mutations.Add(1)
		}
	}, true)
	configureOriginalRecoveryFixture(c, a, b, endpoints)
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	op := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoints[b], Owner: wireOwner(old), Management: true, HandoverId: "uncertain-original"}
	accepted, err := b.LifecyclePrepare(ctx, op)
	if err != nil {
		t.Fatal(err)
	}
	op.Receipt = accepted.Receipt
	op.Target = op.Source
	path := resumeJournal(b.coexistence, "chat")
	failSync := func(*os.File) error { return errors.New("fixture directory sync failure") }
	err = saveResumeWithSync(path, op, failSync)
	var uncertain *resumePublicationUncertain
	if !errors.As(err, &uncertain) {
		t.Fatalf("expected visible uncertain publication: %v", err)
	}
	// Cross-host coordinator starts with no in-memory knowledge of publication.
	c.coexistence.resumeJournalSync = failSync
	confirm := func() (*pb.LifecycleControlResponse, error) {
		offered, err := c.LifecycleResume(ctx, resumeRequest(c, endpoints[c]))
		if err != nil {
			return nil, err
		}
		req := resumeRequest(c, endpoints[c])
		req.RecoveryChallenge = offered.GetOriginalRecovery().GetChallenge()
		req.ConfirmOriginalRecovery = true
		req.ConfirmLegacyRecovery = true
		return c.LifecycleResume(ctx, req)
	}
	got, err := confirm()
	if err == nil || got.GetReady() || mutations.Load() != 0 {
		t.Fatalf("failed durability allowed source mutation: mutations=%d result=%v err=%v", mutations.Load(), got, err)
	}
	if !proto.Equal(readRecoveryOperation(t, b), op) {
		t.Fatal("uncertain original journal changed")
	}
	v, err := conversation.ReadOwnership(ctx, a.coexistence.store, "chat")
	if err != nil || v.Owner != old.Host || v.Epoch != old.Epoch || v.State != "owned" {
		t.Fatalf("failed sync changed ownership: %v %v", v, err)
	}
	// Simulate coordinator restart: consent is ephemeral; original operation is not.
	c.coexistence.recoveryOffers = nil
	c.coexistence.resumeJournalSync = nil
	got, err = confirm()
	if err != nil || !got.GetReady() || got.GetOperationId() != op.HandoverId {
		t.Fatalf("exact operation not recoverable after successful sync: %v %v", got, err)
	}
	if !proto.Equal(readRecoveryOperation(t, b), op) {
		t.Fatal("recovery replaced original operation")
	}
	if _, err := os.Stat(resumeJournal(c.coexistence, "chat")); !os.IsNotExist(err) {
		t.Fatalf("competing journal: %v", err)
	}
}

// A successful directory sync must not bless a replacement published while the
// barrier runs. The coordinator must neither overwrite it nor touch the source.
func TestOriginalRecoveryRejectsJournalChangeDuringSync(t *testing.T) {
	ctx := recoveryFixturePeer(context.Background())
	a, b, c := lifecycleFixture(t)
	var mutations atomic.Int32
	endpoints := inMemoryRecoveryControlsWithHook(t, a, b, c, func(method string, _ any) {
		if strings.HasSuffix(method, "Relinquish") {
			mutations.Add(1)
		}
	}, true)
	configureOriginalRecoveryFixture(c, a, b, endpoints)
	old, err := a.Retain(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	op := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: endpoints[b], Owner: wireOwner(old), Management: true, HandoverId: "original-before-sync"}
	accepted, err := b.LifecyclePrepare(ctx, op)
	if err != nil {
		t.Fatal(err)
	}
	op.Receipt, op.Target = accepted.Receipt, op.Source
	path := resumeJournal(b.coexistence, "chat")
	if err := saveResume(path, op); err != nil {
		t.Fatal(err)
	}
	offered, err := c.LifecycleResume(ctx, resumeRequest(c, endpoints[c]))
	if err != nil {
		t.Fatal(err)
	}
	changed := proto.Clone(op).(*pb.LifecycleControlRequest)
	changed.HandoverId = "replacement-during-sync"
	c.coexistence.resumeJournalSync = func(d *os.File) error {
		if err := saveResume(path, changed); err != nil {
			return err
		}
		return d.Sync()
	}
	req := resumeRequest(c, endpoints[c])
	req.RecoveryChallenge = offered.OriginalRecovery.Challenge
	req.ConfirmOriginalRecovery, req.ConfirmLegacyRecovery = true, true
	got, err := c.LifecycleResume(ctx, req)
	if err == nil || got.GetReady() || mutations.Load() != 0 {
		t.Fatalf("changed publication allowed source mutation: mutations=%d result=%v err=%v", mutations.Load(), got, err)
	}
	if !proto.Equal(readRecoveryOperation(t, b), changed) {
		t.Fatal("replacement was overwritten")
	}
	v, err := conversation.ReadOwnership(ctx, a.coexistence.store, "chat")
	if err != nil || v.Owner != old.Host || v.Epoch != old.Epoch || v.State != "owned" {
		t.Fatalf("changed publication changed ownership: %v %v", v, err)
	}
}
