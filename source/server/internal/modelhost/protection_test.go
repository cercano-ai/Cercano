package modelhost

import (
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net"
	"testing"
	"time"

	"cercano/source/server/internal/localruntime"
)

type protectionManager struct {
	localruntime.Manager
	stops     int
	instances []localruntime.InstanceRecord
}

func (m *protectionManager) RegisterObserver(localruntime.Observer) {}
func (m *protectionManager) Instances(context.Context) ([]localruntime.InstanceRecord, error) {
	return m.instances, nil
}
func (m *protectionManager) Stop(context.Context, localruntime.StopRequest) error {
	m.stops++
	return nil
}

func TestStopWithoutHoldDoesNotStopManager(t *testing.T) {
	m := &protectionManager{}
	s := NewServer(m, ServerOptions{})
	if err := s.stop(context.Background(), "not-owner", localruntime.StopRequest{InstanceID: "external"}); err != nil {
		t.Fatal(err)
	}
	if m.stops != 0 {
		t.Fatalf("no-hold Stop called manager.Stop %d times", m.stops)
	}
}

func (m *protectionManager) Inventory(context.Context) ([]localruntime.ModelRecord, error) {
	return nil, nil
}

func TestProtectedServerDenials(t *testing.T) {
	m := &protectionManager{}
	s := NewServer(m, ServerOptions{CoexistenceProtection: true, Now: func() time.Time { return time.Unix(100, 0) }})
	ctx := context.Background()
	for _, method := range []string{"Restart", "DownloadModel", "EnsureModelsPresent", "CancelDownload", "DeleteModel", "EnrollDownload"} {
		if _, err := s.dispatch(ctx, "a", method, []byte(`{}`)); err != ErrCoexistenceProtected {
			t.Fatalf("%s: %v", method, err)
		}
	}
	s.addStream("a")
	if _, err := s.start(ctx, "a", localruntime.StartRequest{Runtime: "fake", ModelID: "m"}); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
	if _, err := s.Shutdown(ctx, &proto.ShutdownRequest{}); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
	if err := s.StopAll(ctx); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
	s.idleSince = time.Time{}
	s.checkIdle(ctx)
	if s.draining.Load() || s.idleFired || m.stops != 0 {
		t.Fatal("denial mutated runtime")
	}
}

func TestProtectedReuseAndOwnRelease(t *testing.T) {
	h := newHarness(t)
	// Seed the fake manager directly, then wrap with immutable protection.
	inst, err := h.mgr.Start(context.Background(), localruntime.StartRequest{Runtime: "fake", ModelID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(h.mgr, ServerOptions{CoexistenceProtection: true})
	s.addStream("a")
	s.addStream("b")
	for _, id := range []string{"a", "b"} {
		got, err := s.start(context.Background(), id, localruntime.StartRequest{Runtime: "fake", ModelID: "m"})
		if err != nil || got.ID != inst.ID {
			t.Fatalf("reuse: %v %v", got, err)
		}
	}
	if err := s.stop(context.Background(), "a", localruntime.StopRequest{InstanceID: inst.ID}); err != nil {
		t.Fatal(err)
	}
	s.releaseClient("stranger")
	if h.provider.StopCalls() != 0 || s.holds.count(inst.ID) != 1 {
		t.Fatal("sibling runtime released")
	}
}

func TestProtectedEnsureNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	old := &oldHost{dir: dir}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	proto.RegisterModelHostServer(g, old)
	go g.Serve(lis)
	defer g.Stop()
	if err := WriteRecord(dir, Record{PID: 4242, Addr: lis.Addr().String(), Token: "tok", APIVersion: APIVersion - 1}); err != nil {
		t.Fatal(err)
	}
	launch := func(context.Context) error { t.Fatal("launched replacement"); return nil }
	if _, err := Ensure(context.Background(), EnsureOptions{RunDir: dir, Launch: launch, CoexistenceProtection: true}); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
	if _, err := Relaunch(context.Background(), RelaunchOptions{RunDir: dir, Launch: launch, CoexistenceProtection: true}); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
	if old.shutdown.Load() != 0 {
		t.Fatal("shut down old host")
	}
	if _, err := Ensure(context.Background(), EnsureOptions{RunDir: t.TempDir(), Launch: launch, CoexistenceProtection: true}); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
}

func TestProtectionRequestEnforcedAtServer(t *testing.T) {
	m := &protectionManager{}
	s := NewServer(m, ServerOptions{})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(protectionHeader, "true"))
	s.addStream("a")
	if _, err := s.start(ctx, "a", localruntime.StartRequest{}); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
	if _, err := s.dispatch(ctx, "a", "DownloadModel", nil); err != ErrCoexistenceProtected {
		t.Fatal(err)
	}
}

func (m *protectionManager) ResolveOpenModel(context.Context, string, string) (localruntime.ModelRecord, error) {
	return localruntime.ModelRecord{}, nil
}

func TestProtectedRemoteCloseAndReconnect(t *testing.T) {
	dir := t.TempDir()
	host := startTestHost(t, "protected-token")
	host.publish(t, dir)
	opts := remoteOpts(dir, nil)
	opts.CoexistenceProtection = true
	calls := 0
	ensure := func(ctx context.Context, o EnsureOptions) (Record, error) {
		calls++
		if !o.CoexistenceProtection {
			t.Error("protection lost on Ensure")
		}
		return Ensure(ctx, o)
	}
	a, err := newRemote(context.Background(), opts, ensure)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b := newTestRemote(t, remoteOpts(dir, nil))
	inst, err := b.Start(context.Background(), localruntime.StartRequest{Runtime: "fake", ModelID: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Start(context.Background(), localruntime.StartRequest{Runtime: "fake", ModelID: "m1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.reconnect(context.Background(), a.HoldEpoch()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("Ensure calls = %d", calls)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "own holds released", func() bool { return len(host.srv.holds.heldBy(a.ClientID())) == 0 })
	if host.srv.draining.Load() || host.provider.StopCalls() != 0 || host.provider.StartCalls() != 1 || host.srv.holds.count(inst.ID) != 1 {
		t.Fatal("Close disturbed sibling runtime")
	}
}
