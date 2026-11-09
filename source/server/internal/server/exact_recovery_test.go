package server

// Characterizes the original transfer controls and supplies isolated fixtures
// for retry-only regressions. Prepare is used ONLY for fixture setup. The
// Destination/Target gap is now a rejection regression.
import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

type exactRecoveryFixture struct {
	a, b, c   *Server
	clients   map[*Server]pb.AgentClient
	endpoints map[*Server]string
	op        *pb.LifecycleControlRequest
	ctx       context.Context
}

func newExactRecoveryFixture(t *testing.T) *exactRecoveryFixture {
	t.Helper()
	a, b, c := lifecycleFixture(t)
	f := &exactRecoveryFixture{a: a, b: b, c: c, clients: map[*Server]pb.AgentClient{}, endpoints: map[*Server]string{}, ctx: context.Background()}
	listeners := map[*Server]*bufconn.Listener{}
	auths := map[*Server]*devhostauth.Auth{}
	for i, s := range []*Server{a, b, c} {
		path := filepath.Join(t.TempDir(), "token")
		if err := devhostauth.Create(path); err != nil {
			t.Fatal(err)
		}
		auth, err := devhostauth.Load(s.coexistence.options.Host, path)
		if err != nil {
			t.Fatal(err)
		}
		auths[s] = auth
		s.coexistence.options.ControlAuthorize = auth.Authorize
		f.endpoints[s] = fmt.Sprintf("127.0.0.1:%d", 19101+i)
		listener := bufconn.Listen(1024 * 1024)
		listeners[s] = listener
		opts := auth.ServerOptions()
		// bufconn has no IP peer: synthetic loopback transport metadata only;
		// real private fixture tokens are still checked on every RPC.
		opts = append(opts, grpc.ChainUnaryInterceptor(func(ctx context.Context, r any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			return next(recoveryFixturePeer(ctx), r)
		}))
		transport := grpc.NewServer(opts...)
		pb.RegisterAgentServer(transport, s)
		go transport.Serve(listener)
		t.Cleanup(func() { transport.Stop(); listener.Close() })
	}
	dial := func(_ context.Context, endpoint string, identity *pb.LifecycleIdentity) (*grpc.ClientConn, error) {
		for s, ep := range f.endpoints {
			if endpoint == ep && proto.Equal(identity, s.coexistence.controlIdentity()) {
				opts := auths[s].DialOptions()
				opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listeners[s].DialContext(ctx) }))
				return grpc.NewClient("passthrough:///exact-recovery-fixture", opts...)
			}
		}
		return nil, fmt.Errorf("fixture identity/route mismatch")
	}
	for _, s := range []*Server{a, b, c} {
		s.coexistence.options.ControlDial = dial
		conn, err := dial(f.ctx, f.endpoints[s], s.coexistence.controlIdentity())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		f.clients[s] = pb.NewAgentClient(conn)
	}
	owner, err := a.Retain(f.ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	f.op = &pb.LifecycleControlRequest{Target: a.coexistence.controlIdentity(), Source: a.coexistence.controlIdentity(), Destination: b.coexistence.controlIdentity(), DestinationEndpoint: f.endpoints[b], Owner: wireOwner(owner), HandoverId: "existing-operation", Management: true}
	setup := proto.Clone(f.op).(*pb.LifecycleControlRequest)
	setup.Target = setup.Destination
	accepted, err := f.clients[b].LifecyclePrepare(f.ctx, setup)
	if err != nil {
		t.Fatal(err)
	}
	f.op.Receipt = accepted.Receipt
	return f
}
func (f *exactRecoveryFixture) claim() *pb.LifecycleControlRequest {
	r := proto.Clone(f.op).(*pb.LifecycleControlRequest)
	r.Target = r.Destination
	return r
}
func (f *exactRecoveryFixture) snapshot(t *testing.T) string {
	t.Helper()
	v, err := conversation.ReadOwnership(f.ctx, f.a.coexistence.store, "chat")
	if err != nil {
		t.Fatal(err)
	}
	result := ""
	for _, s := range []*Server{f.a, f.b, f.c} {
		l := s.coexistence
		journals, err := filepath.Glob(filepath.Join(l.options.StorePath+".resume-operations", "*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(journals) != 0 {
			t.Fatalf("exact recovery created an unexpected resume journal: %v", journals)
		}
		l.mu.Lock()
		var records []string
		for o, r := range l.closures {
			state := "no-control"
			if c := r.control; c != nil {
				state = fmt.Sprintf("%s/%t/%t/%t/%s", c.request, c.active, c.committing, c.completed, c.receipt)
			}
			records = append(records, fmt.Sprintf("closure:%+v/%s", o, state))
		}
		for o, forced := range l.forced {
			records = append(records, fmt.Sprintf("forced:%+v/%t", o, forced))
		}
		for h, a := range l.incoming {
			records = append(records, fmt.Sprintf("incoming:%+v/%s/%s", h, a.sourceBuild, a.receipt))
		}
		for _, cap := range l.owners {
			o := ownerOf(cap)
			_, retained, open := l.admission.CheckRetained(o)
			records = append(records, fmt.Sprintf("owner:%+v/%t/%t/%t", o, l.ready[o], retained, open))
		}
		sort.Strings(records)
		result += fmt.Sprintf("/%v", records)
		l.mu.Unlock()
	}
	return fmt.Sprintf("%+v%s", v, result)
}
func (f *exactRecoveryFixture) reserve(t *testing.T) {
	t.Helper()
	if _, err := f.clients[f.a].LifecycleRelinquish(f.ctx, f.op); err != nil {
		t.Fatal(err)
	}
}

func TestExactRecoverySourceRequiresExistingReservationGap(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			f := newExactRecoveryFixture(t)
			if len(f.a.coexistence.closures) != 0 {
				t.Fatal("fixture already reserved")
			}
			// A read-only acceptance preflight cannot distinguish starting the source
			// reservation from retrying one: both source APIs start it here.
			if _, err := f.clients[f.b].LifecycleValidateReceipt(f.ctx, f.claim()); err != nil {
				t.Fatal(err)
			}
			var err error
			if force {
				_, err = f.clients[f.a].LifecycleForceRelinquish(f.ctx, f.op)
			} else {
				_, err = f.clients[f.a].LifecycleRelinquish(f.ctx, f.op)
			}
			if err != nil {
				t.Fatal(err)
			}
			v, err := conversation.ReadOwnership(f.ctx, f.a.coexistence.store, "chat")
			if err != nil || v.State != "reserved" || len(f.a.coexistence.closures) != 1 {
				t.Fatalf("gap not reproduced: %+v %v", v, err)
			}
		})
	}
}

func TestExactRecoverySourceRejectionCanFenceGap(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			f := newExactRecoveryFixture(t)
			// Acceptance existed at preflight, but is gone at actual RPC. Do not create
			// replacement records: the nonempty original receipt must be validated.
			if _, err := f.clients[f.b].LifecycleValidateReceipt(f.ctx, f.claim()); err != nil {
				t.Fatal(err)
			}
			f.b.coexistence.mu.Lock()
			for h := range f.b.coexistence.incoming {
				delete(f.b.coexistence.incoming, h)
			}
			f.b.coexistence.mu.Unlock()
			before := f.snapshot(t)
			var err error
			if force {
				_, err = f.clients[f.a].LifecycleForceRelinquish(f.ctx, f.op)
			} else {
				_, err = f.clients[f.a].LifecycleRelinquish(f.ctx, f.op)
			}
			if err == nil {
				t.Fatal("missing acceptance accepted")
			}
			if before == f.snapshot(t) || len(f.a.coexistence.closures) != 1 {
				t.Fatal("expected source closure mutation on rejection")
			}
			v, e := conversation.ReadOwnership(f.ctx, f.a.coexistence.store, "chat")
			if e != nil || v.State != "owned" {
				t.Fatalf("durable ownership changed: %+v %v", v, e)
			}
		})
	}
}

func TestExactRecoveryDestination(t *testing.T) {
	for _, which := range []string{"missing-reservation", "missing-receipt", "missing-acceptance", "source-build", "source-host", "source-epoch", "conversation", "operation", "wrong-target", "destination-field-gap", "journal-not-required", "ownership-changed-after-preflight"} {
		t.Run(which, func(t *testing.T) {
			f := newExactRecoveryFixture(t)
			r := f.claim()
			if which != "missing-reservation" {
				f.reserve(t)
			}
			if _, err := f.clients[f.b].LifecycleValidateReceipt(f.ctx, r); err != nil {
				t.Fatal(err)
			}
			switch which {
			case "missing-receipt":
				r.Receipt = ""
			case "missing-acceptance":
				f.b.coexistence.mu.Lock()
				for h := range f.b.coexistence.incoming {
					delete(f.b.coexistence.incoming, h)
				}
				f.b.coexistence.mu.Unlock()
			case "source-build":
				r.Source.Build = "other"
			case "source-host":
				r.Source.Host = "other"
			case "source-epoch":
				r.Owner.Epoch = "other"
			case "conversation":
				r.Owner.Conversation = "other"
			case "operation":
				r.HandoverId = "other"
			case "wrong-target":
				r.Target = f.c.coexistence.controlIdentity()
			case "destination-field-gap":
				r.Destination = f.c.coexistence.controlIdentity()
			case "ownership-changed-after-preflight":
				got, err := f.clients[f.b].LifecycleClaim(f.ctx, r)
				if err != nil {
					t.Fatal(err)
				}
				// Clean release changes current ownership after the read-only preflight.
				if err := f.b.ReleaseRetained(f.ctx, controlOwner(&pb.LifecycleControlRequest{Owner: got.Owner}), nil); err != nil {
					t.Fatal(err)
				}
			}
			before := f.snapshot(t)
			got, err := f.clients[f.b].LifecycleClaim(f.ctx, r)
			success := which == "journal-not-required" || which == "ownership-changed-after-preflight"
			if !success {
				if err == nil {
					t.Fatalf("mismatch accepted: %v", got)
				}
				if before != f.snapshot(t) {
					t.Fatal("rejected claim mutated state")
				}
				return
			}
			if err != nil || got.GetOwner().GetHost() != "B" {
				t.Fatalf("claim: %v %v", got, err)
			}
			if which == "ownership-changed-after-preflight" && before != f.snapshot(t) {
				t.Fatal("historical result reacquired ownership")
			}
			if _, err := os.Stat(resumeJournal(f.b.coexistence, "chat")); !os.IsNotExist(err) {
				t.Fatalf("unexpected journal: %v", err)
			}
			again, e := f.clients[f.b].LifecycleClaim(f.ctx, r)
			if e != nil || !proto.Equal(got.Owner, again.Owner) {
				t.Fatalf("idempotence: %v %v", again, e)
			}
		})
	}
}

func TestExactRecoveryTimeoutAndExactRetry(t *testing.T) {
	f := newExactRecoveryFixture(t)
	held, err := f.a.AdmitRetainedWork(controlOwner(f.op), broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 80*time.Millisecond)
	_, err = f.clients[f.a].LifecycleForceRelinquish(ctx, f.op)
	cancel()
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("timeout: %v", err)
	}
	// The request may be canceled by transport just before the handler observes
	// it. The closure is installed before the source starts its bounded wait.
	f.a.coexistence.mu.Lock()
	closed := len(f.a.coexistence.closures)
	f.a.coexistence.mu.Unlock()
	if closed != 1 {
		t.Fatal("not fenced")
	}
	if err := held.Complete(); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"operation", "destination", "destination-build", "host", "conversation", "target", "epoch", "build", "receipt"} {
		r := proto.Clone(f.op).(*pb.LifecycleControlRequest)
		switch field {
		case "operation":
			r.HandoverId = "other"
		case "destination":
			r.Destination = f.c.coexistence.controlIdentity()
			r.DestinationEndpoint = f.endpoints[f.c]
		case "destination-build":
			r.Destination.Build = "other"
		case "host":
			r.Owner.Host = "other"
		case "conversation":
			r.Owner.Conversation = "other"
		case "target":
			r.Target = f.c.coexistence.controlIdentity()
		case "epoch":
			r.Owner.Epoch = "other"
		case "build":
			r.Source.Build = "other"
		case "receipt":
			r.Receipt = "other"
		}
		before := f.snapshot(t)
		if _, err := f.clients[f.a].LifecycleForceRelinquish(f.ctx, r); err == nil {
			t.Fatalf("%s mismatch accepted", field)
		}
		if before != f.snapshot(t) {
			t.Fatalf("%s rejection mutated state", field)
		}
	}
	if _, err := f.clients[f.a].LifecycleForceRelinquish(f.ctx, f.op); err != nil {
		t.Fatal(err)
	}
	got, err := f.clients[f.b].LifecycleClaim(f.ctx, f.claim())
	if err != nil {
		t.Fatal(err)
	}
	before := f.snapshot(t)
	if _, err := f.clients[f.a].LifecycleForceRelinquish(f.ctx, f.op); err != nil {
		t.Fatal(err)
	}
	again, err := f.clients[f.b].LifecycleClaim(f.ctx, f.claim())
	if err != nil || !proto.Equal(got.Owner, again.Owner) || before != f.snapshot(t) {
		t.Fatalf("completed retry changed operation: %v %v", again, err)
	}
}

// Source ownership changes after receipt preflight must not be inferred safe
// from that read. The actual source RPC checks the retained owner capability.
func TestExactRecoverySourceOwnershipChangedAfterPreflight(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			f := newExactRecoveryFixture(t)
			if _, err := f.clients[f.b].LifecycleValidateReceipt(f.ctx, f.claim()); err != nil {
				t.Fatal(err)
			}
			if err := f.a.ReleaseRetained(f.ctx, controlOwner(f.op), nil); err != nil {
				t.Fatal(err)
			}
			next, err := f.c.Retain(f.ctx, "chat")
			if err != nil {
				t.Fatal(err)
			}
			if next.Host != "C" {
				t.Fatal("ownership not changed")
			}
			before := f.snapshot(t)
			if force {
				_, err = f.clients[f.a].LifecycleForceRelinquish(f.ctx, f.op)
			} else {
				_, err = f.clients[f.a].LifecycleRelinquish(f.ctx, f.op)
			}
			if err == nil {
				t.Fatal("stale source accepted")
			}
			if before != f.snapshot(t) {
				t.Fatal("stale source mutated state")
			}
		})
	}
}

// Even a retry of an existing forced closure need not present the receipt.
// The source strips it from the key and Prepare reuses the existing acceptance.
// This probe leaves acceptance present: it never creates a replacement record.
func TestExactRecoverySourceMissingReceiptGap(t *testing.T) {
	f := newExactRecoveryFixture(t)
	held, err := f.a.AdmitRetainedWork(controlOwner(f.op), broker.PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 80*time.Millisecond)
	_, err = f.clients[f.a].LifecycleForceRelinquish(ctx, f.op)
	cancel()
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("timeout: %v", err)
	}
	if err := held.Complete(); err != nil {
		t.Fatal(err)
	}
	r := proto.Clone(f.op).(*pb.LifecycleControlRequest)
	r.Receipt = ""
	got, err := f.clients[f.a].LifecycleForceRelinquish(f.ctx, r)
	if err != nil || got.GetReceipt() != f.op.Receipt {
		t.Fatalf("optional receipt gap: %v %v", got, err)
	}
	f.b.coexistence.mu.Lock()
	n := len(f.b.coexistence.incoming)
	f.b.coexistence.mu.Unlock()
	if n != 1 {
		t.Fatal("retry created another acceptance")
	}
}
