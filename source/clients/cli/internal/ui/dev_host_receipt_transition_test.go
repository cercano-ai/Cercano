package ui

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"

	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// Like the real host, receipts are bound to the original exact owner. Ordinary
// acquisition/resume must never reinterpret that receipt for a new owner.
type receiptTransitionHost struct {
	pb.UnimplementedAgentServer
	deliveries, ordinary atomic.Int32
	mode                 atomic.Int32 // 1: rejected; 2: ambiguous; 3: first delivery response lost
	stale                atomic.Bool
}

func (h *receiptTransitionHost) LifecycleAcquire(_ context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if r.LaunchAction == "deliver" {
		n := h.deliveries.Add(1)
		if r.Owner.Conversation != "original" || r.Owner.Epoch != "original-epoch" || r.LaunchOperation != "operation" {
			return nil, fmt.Errorf("receipt bound to original conversation/epoch")
		}
		if h.mode.Load() == 1 || (h.mode.Load() == 3 && n == 1) {
			return nil, fmt.Errorf("delivery response unavailable")
		}
		return &pb.LifecycleControlResponse{Identity: r.Target, Owner: r.Owner, Receipt: r.LaunchOperation, Pending: h.mode.Load() == 2}, nil
	}
	h.ordinary.Add(1)
	return &pb.LifecycleControlResponse{Identity: r.Target, Owner: &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: r.Target.Host, Epoch: "fresh-epoch"}}, nil
}
func (h *receiptTransitionHost) LifecycleResume(_ context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	h.ordinary.Add(1)
	return &pb.LifecycleControlResponse{Identity: r.Target, Ready: true, Owner: &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: r.Target.Host, Epoch: "resumed-epoch"}}, nil
}
func (h *receiptTransitionHost) GetDevelopmentHostIdentity(context.Context, *pb.GetDevelopmentHostIdentityRequest) (*pb.GetDevelopmentHostIdentityResponse, error) {
	return &pb.GetDevelopmentHostIdentityResponse{HostInstance: "host", Build: "build", Contract: "contract", SchemaVersion: 2}, nil
}
func (h *receiptTransitionHost) CheckRetainedSession(_ context.Context, r *pb.CheckRetainedSessionRequest) (*pb.CheckRetainedSessionResponse, error) {
	if h.stale.Load() || r.HostInstance != "host" || r.ConversationId == "" || r.OwnershipEpoch == "" {
		return nil, fmt.Errorf("owner mismatch")
	}
	return &pb.CheckRetainedSessionResponse{State: pb.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
}
func receiptTransitionSelection(t *testing.T) (DevHostSelection, *receiptTransitionHost) {
	t.Helper()
	token := filepath.Join(t.TempDir(), "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := &receiptTransitionHost{}
	server := grpc.NewServer()
	pb.RegisterAgentServer(server, host)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	return DevHostSelection{LaunchOperation: "operation", OperationFile: "retry.json", Identity: &pb.LifecycleIdentity{Host: "host", Build: "build", Contract: "contract", Schema: 2, Store: "store"}, Conversation: "original", Epoch: "original-epoch", Endpoint: listener.Addr().String(), TokenFile: token}, host
}
func assertNoLaunchReceipt(t *testing.T, s DevHostSelection) {
	t.Helper()
	if s.LaunchOperation != "" || s.OperationFile != "" {
		t.Fatalf("initial receipt survived: %+v", s)
	}
}
func TestLaunchReceiptReusedForOtherConversation(t *testing.T) {
	for _, action := range []string{"clear", "resume"} {
		t.Run(action, func(t *testing.T) {
			s, host := receiptTransitionSelection(t)
			// Keep a pre-ack copy as well: transitions must safely consume old templates.
			template := s
			c, err := s.Dial(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			m := New(c, false).WithDevHost(s, "test")
			m.devHost.defaultSelection = template
			var r devHostResult
			if action == "clear" {
				_, next := m.beginDevHostFreshConversation()
				r = next().(devHostResult)
			} else {
				_, next := m.beginDevHostReopen("history")
				r = next().(devHostResult)
			}
			if r.err != nil {
				t.Fatal(r.err)
			}
			defer r.client.Close()
			assertNoLaunchReceipt(t, *r.selection)
			next, _ := m.applyDevHost(r)
			updated := next.(Model)
			assertNoLaunchReceipt(t, updated.devHost.selection)
			assertNoLaunchReceipt(t, updated.devHost.defaultSelection)
			// A later history reopen uses the now-ordinary default, with no launch replay.
			deliveries := host.deliveries.Load()
			_, reopen := updated.beginDevHostReopen("another-history")
			again := reopen().(devHostResult)
			if again.err != nil {
				t.Fatal(again.err)
			}
			again.client.Close()
			if host.deliveries.Load() != deliveries {
				t.Fatal("default selection replayed receipt")
			}
		})
	}
}
func TestLaunchReceiptDeliveryFailureRetainsRetryAndBlocksTransitions(t *testing.T) {
	for _, mode := range []int32{1, 2, 3} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			for _, action := range []string{"dial", "clear", "resume"} {
				t.Run(action, func(t *testing.T) {
					s, host := receiptTransitionSelection(t)
					host.mode.Store(mode)
					var err error
					switch action {
					case "dial":
						_, err = s.Dial(context.Background())
					case "resume":
						_, err = reopenDevHost(context.Background(), s, "history")
					case "clear":
						m := New(nil, false).WithDevHost(s, "test")
						_, cmd := m.beginDevHostFreshConversation()
						err = cmd().(devHostResult).err
					}
					if err == nil {
						t.Fatal("unresolved initial delivery became usable")
					}
					if s.LaunchOperation != "operation" || s.OperationFile != "retry.json" {
						t.Fatal("lost retry metadata")
					}
					if host.ordinary.Load() != 0 {
						t.Fatal("acquired/resumed before initial delivery acknowledgement")
					}
					if mode == 3 { // Lost successful response: exact retry remains safe.
						if err := s.AcceptLaunchDelivery(context.Background()); err != nil {
							t.Fatal(err)
						}
						assertNoLaunchReceipt(t, s)
					}
				})
			}
		})
	}
}
func TestLaunchReceiptRepeatedAckAndDetachReconnect(t *testing.T) {
	s, host := receiptTransitionSelection(t)
	copy := s
	if err := s.AcceptLaunchDelivery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.AcceptLaunchDelivery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := copy.AcceptLaunchDelivery(context.Background()); err != nil {
		t.Fatal(err)
	}
	if host.deliveries.Load() != 2 {
		t.Fatal("ack not consumed or exact repeat unsafe")
	}
	c, err := s.Dial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := New(c, false).WithDevHost(s, "test")
	detached, _ := m.runDevHost("/host detach")
	m = detached.(Model)
	if host.ordinary.Load() != 0 {
		t.Fatal("detach changed ownership")
	}
	_, cmd := m.runDevHost("/host reconnect")
	r := cmd().(devHostResult)
	if r.err != nil {
		t.Fatal(r.err)
	}
	defer r.client.Close()
	if host.deliveries.Load() != 2 || host.ordinary.Load() != 0 {
		t.Fatal("reconnect replayed launch or acquired ownership")
	}
	// Removing initial metadata must not bypass ordinary retained-owner validation.
	host.stale.Store(true)
	if c, err := s.Dial(context.Background()); err == nil {
		c.Close()
		t.Fatal("stale reconnect accepted")
	}
}
