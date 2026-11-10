package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

// Bound only the source wait, not the destination's context: the destination
// must observe the actual source timeout after fencing, not a racing RPC cancel.
type interruptedCutoverSource struct{ *Server }

func (s *interruptedCutoverSource) LifecycleForceRelinquish(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return s.Server.LifecycleForceRelinquish(ctx, r)
}

// Synthetic identity checks and bufconn are fixture trust, not production auth.
// Endpoint strings satisfy protocol route validation; no socket is ever opened.
func inMemoryRecoveryControls(t *testing.T, a, b, c *Server, legacy ...bool) map[*Server]string {
	return inMemoryRecoveryControlsWithHook(t, a, b, c, nil, legacy...)
}

func inMemoryRecoveryControlsWithHook(t *testing.T, a, b, c *Server, before func(string, any), legacy ...bool) map[*Server]string {
	t.Helper()
	servers := []*Server{a, b, c}
	endpoints := map[*Server]string{a: "127.0.0.1:19001", b: "127.0.0.1:19002", c: "127.0.0.1:19003"}
	listeners := make(map[string]*bufconn.Listener)
	identities := make(map[string]*pb.LifecycleIdentity)
	for _, s := range servers {
		endpoint := endpoints[s]
		listener := bufconn.Listen(1024 * 1024)
		listeners[endpoint], identities[endpoint] = listener, s.coexistence.controlIdentity()
		// bufconn has no IP peer; explicitly supply synthetic fixture peer trust.
		transport := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			if len(legacy) > 1 && legacy[1] && strings.HasSuffix(info.FullMethod, "/LifecycleForceRelinquish") {
				return nil, status.Error(codes.Unimplemented, "fixture: old peer has only ordinary relinquish")
			}
			if before != nil {
				before(info.FullMethod, req)
			}
			return handler(recoveryFixturePeer(ctx), req)
		}))
		if s == a {
			pb.RegisterAgentServer(transport, &interruptedCutoverSource{Server: s})
		} else {
			pb.RegisterAgentServer(transport, s)
		}
		go transport.Serve(listener)
		t.Cleanup(func() { transport.Stop(); listener.Close() })
	}
	dial := func(ctx context.Context, endpoint string, identity *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
		listener := listeners[endpoint]
		if listener == nil || !proto.Equal(identity, identities[endpoint]) {
			return nil, errors.New("fixture route/identity mismatch")
		}
		return grpc.NewClient("passthrough:///in-memory-recovery", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	}
	for _, s := range servers {
		s.coexistence.options.ControlAuthorize = func(context.Context) error { return nil }
		s.coexistence.options.ControlDial = dial
	}
	setResumeSource(b, a, endpoints[a])
	setResumeSource(c, a, endpoints[a])
	return endpoints
}

func recoveryFixturePeer(ctx context.Context) context.Context {
	return peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 19000}})
}

func readRecoveryOperation(t *testing.T, s *Server) *pb.LifecycleControlRequest {
	t.Helper()
	data, err := os.ReadFile(resumeJournal(s.coexistence, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	op := new(pb.LifecycleControlRequest)
	if err := json.Unmarshal(data, op); err != nil {
		t.Fatal(err)
	}
	return op
}

// Characterization, not a new recovery policy: a fresh destination cannot
// supersede the source's exact in-flight operation, even after work drains.
func TestResumeNewDestinationInterruptedTransfer(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		name := "completed-writeback"
		if uncertain {
			name = "uncertain-writeback"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(recoveryFixturePeer(context.Background()), 10*time.Second)
			defer cancel()
			a, b, c := lifecycleFixture(t)
			endpoints := inMemoryRecoveryControls(t, a, b, c)
			old, err := a.Retain(ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			sibling, err := a.Retain(ctx, "sibling")
			if err != nil {
				t.Fatal(err)
			}
			held, err := a.AdmitRetainedWork(old, broker.PersistenceWork)
			if err != nil {
				t.Fatal(err)
			}
			assertOldOwned := func() {
				t.Helper()
				v, e := conversation.ReadOwnership(ctx, a.coexistence.store, "chat")
				if e != nil {
					t.Fatal(e)
				}
				if v.State != "owned" || v.Owner != old.Host || v.Epoch != old.Epoch || v.Destination != old.Host || v.Handover != "" {
					t.Fatalf("unexpected durable ownership: %+v", v)
				}
			}
			_, err = b.LifecycleResume(ctx, resumeRequest(b, endpoints[b]))
			if status.Code(err) != codes.DeadlineExceeded || !strings.Contains(err.Error(), "source stays fenced") {
				t.Fatalf("source wait not exercised: %v", err)
			}
			first := readRecoveryOperation(t, b)
			if first.HandoverId == "" || first.Receipt == "" || !proto.Equal(first.Destination, b.coexistence.controlIdentity()) {
				t.Fatalf("incomplete journal: %v", first)
			}
			t.Logf("probe: original operation timed out after fencing: %v", err)
			assertOldOwned()
			if attachment, e := a.AttachRetained(old); e == nil {
				attachment.Detach()
				t.Fatal("fenced owner reattached")
			}
			if work, e := a.AdmitRetainedWork(old, broker.PersistenceWork); e == nil {
				work.Complete()
				t.Fatal("fenced owner admitted new persistence")
			}
			attachment, e := a.AttachRetained(sibling)
			if e != nil {
				t.Fatalf("sibling fenced: %v", e)
			}
			attachment.Detach()
			if _, e := c.Retain(ctx, "chat"); e == nil {
				t.Fatal("new destination stole source ownership")
			}
			claim := proto.Clone(first).(*pb.LifecycleControlRequest)
			claim.Target = claim.Destination
			if _, e := b.LifecycleClaim(ctx, claim); e == nil {
				t.Fatal("claimed before writeback/reservation")
			}
			if uncertain {
				if e := held.PersistenceFailed(); e != nil {
					t.Fatal(e)
				}
				if e := held.Complete(); e == nil {
					t.Fatal("uncertain persistence completed without reconciliation")
				}
				if _, e := b.LifecycleResume(ctx, resumeRequest(b, endpoints[b])); status.Code(e) != codes.FailedPrecondition || !strings.Contains(e.Error(), "unresolved persistence") {
					t.Fatalf("uncertain retry: %v", e)
				}
				assertOldOwned()
				if e := held.ResolvePersistence(); e != nil {
					t.Fatal(e)
				}
			}
			if e := held.Complete(); e != nil {
				t.Fatal(e)
			}
			// Without containment C may try cooperation, but cannot bypass the
			// source closure for B or obtain ownership.
			_, err = c.LifecycleResume(ctx, resumeRequest(c, endpoints[c]))
			if err == nil {
				t.Fatalf("competing destination not blocked: %v", err)
			}
			assertOldOwned()
			// Knowing B's operation or receipt is not authority to claim it as C.
			wrong := proto.Clone(claim).(*pb.LifecycleControlRequest)
			wrong.Target = c.coexistence.controlIdentity()
			if _, e := c.LifecycleClaim(ctx, wrong); e == nil {
				t.Fatal("C claimed B receipt")
			}
			wrong = proto.Clone(claim).(*pb.LifecycleControlRequest)
			wrong.Receipt = "unrelated-receipt"
			if _, e := b.LifecycleValidateReceipt(ctx, wrong); e == nil {
				t.Fatal("B validated C receipt")
			}
			wrong = proto.Clone(first).(*pb.LifecycleControlRequest)
			wrong.HandoverId = "unrelated-operation"
			if _, e := a.LifecycleForceRelinquish(ctx, wrong); !errors.Is(e, broker.ErrAdmissionBusy) {
				t.Fatalf("changed operation bypassed closure: %v", e)
			}
			// Exact B retry is the already-implemented safe recovery, not a reset.
			got, e := b.LifecycleResume(ctx, resumeRequest(b, endpoints[b]))
			if e != nil {
				t.Fatal(e)
			}
			if !got.Ready || got.OperationId != first.HandoverId || got.Owner.Host != "B" || got.Owner.Epoch == old.Epoch {
				t.Fatalf("bad exact recovery: %v", got)
			}
			again, e := b.LifecycleResume(ctx, resumeRequest(b, endpoints[b]))
			if e != nil || !proto.Equal(again.Owner, got.Owner) {
				t.Fatalf("retry minted second epoch: %v %v", again, e)
			}
			epoch, e := conversation.HandoverResult(ctx, b.coexistence.store, "chat", "B", first.HandoverId)
			if e != nil || epoch != got.Owner.Epoch {
				t.Fatalf("missing durable exact-operation result: %q %v", epoch, e)
			}
			v, e := conversation.ReadOwnership(ctx, b.coexistence.store, "chat")
			if e != nil || v.State != "owned" || v.Owner != "B" || v.Epoch != epoch {
				t.Fatalf("bad recovered ownership: %+v %v", v, e)
			}
			if attachment, e := a.AttachRetained(old); e == nil {
				attachment.Detach()
				t.Fatal("stale source revived")
			}
			if !proto.Equal(readRecoveryOperation(t, b), first) {
				t.Fatal("recovery rewrote operation authority")
			}
			t.Log("probe: original destination recovered exact operation, one fresh durable epoch; stale source remains fenced")

			// Characterize an existing-API second hop, not adoption of A's operation.
			// The fixture still cannot resolve the new source B.
			if _, e := c.LifecycleResume(ctx, resumeRequest(c, endpoints[c])); e == nil {
				t.Fatal("C resumed without trusted source resolution")
			}
			next := &pb.LifecycleControlRequest{Target: b.coexistence.controlIdentity(), Source: b.coexistence.controlIdentity(), Destination: c.coexistence.controlIdentity(), DestinationEndpoint: endpoints[c], Owner: got.Owner, HandoverId: "explicit-second-hop", Management: true}
			reserved, e := b.LifecycleRelinquish(ctx, next)
			if e != nil {
				t.Fatalf("explicit second hop reservation: %v", e)
			}
			next.Target, next.Receipt = next.Destination, reserved.Receipt
			last, e := c.LifecycleClaim(ctx, next)
			if e != nil || last.GetOwner().GetHost() != "C" || last.GetOwner().GetEpoch() == got.Owner.Epoch {
				t.Fatalf("explicit second hop claim: %v %v", last, e)
			}
			v, e = conversation.ReadOwnership(ctx, c.coexistence.store, "chat")
			if e != nil || v.Owner != "C" || v.Epoch != last.Owner.Epoch || v.State != "owned" {
				t.Fatalf("second hop ownership: %+v %v", v, e)
			}
			stale := readRecoveryOperation(t, c)
			stale.Target = stale.Destination
			if _, e := c.LifecycleClaim(ctx, stale); e == nil {
				t.Fatal("stale competing acceptance claimed new epoch")
			}
			listing, e := c.LifecycleList(ctx, &pb.LifecycleControlRequest{Target: c.coexistence.controlIdentity()})
			if e != nil || listing.GetPendingIncoming() != 1 {
				t.Fatalf("expected retained, non-authoritative competing acceptance: %v %v", listing, e)
			}
		})
	}
}
