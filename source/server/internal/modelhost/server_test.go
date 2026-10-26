package modelhost

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/pkg/proto"
)

const testToken = "secret"

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type harness struct {
	t        *testing.T
	srv      *Server
	provider *localruntimetest.FakeProvider
	mgr      *localruntime.InMemoryManager
	client   proto.ModelHostClient
	clock    *fakeClock
	idle     chan struct{}
}

func newHarness(t *testing.T, models ...string) *harness {
	t.Helper()
	provider := localruntimetest.NewFakeProvider("fake", localruntime.ModelRecord{ID: "absent", Runtime: "fake"})
	mgr := localruntime.NewManager()
	mgr.RegisterProvider(provider)
	clock := &fakeClock{t: time.Unix(1000, 0)}
	h := &harness{t: t, provider: provider, mgr: mgr, clock: clock, idle: make(chan struct{}, 4)}
	h.srv = NewServer(mgr, ServerOptions{
		Token:       testToken,
		IdleTimeout: time.Minute,
		Now:         clock.Now,
		Version:     "test-1",
		OnIdle:      func() { h.idle <- struct{}{} },
	})
	lis := bufconn.Listen(1 << 20)
	g := grpc.NewServer()
	h.srv.Register(g)
	go g.Serve(lis)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(); g.Stop() })
	h.client = proto.NewModelHostClient(conn)
	return h
}

func authCtx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, tokenHeader, testToken)
}

func (h *harness) call(clientID, method string, req, resp any) error {
	h.t.Helper()
	payload := ""
	if req != nil {
		b, err := json.Marshal(req)
		if err != nil {
			h.t.Fatal(err)
		}
		payload = string(b)
	}
	ctx, cancel := context.WithTimeout(authCtx(context.Background()), 5*time.Second)
	defer cancel()
	out, err := h.client.Call(ctx, &proto.CallRequest{ClientId: clientID, Method: method, PayloadJson: payload})
	if err != nil {
		return err
	}
	if resp != nil {
		if err := json.Unmarshal([]byte(out.PayloadJson), resp); err != nil {
			h.t.Fatalf("decode %s response %q: %v", method, out.PayloadJson, err)
		}
	}
	return nil
}

func (h *harness) start(clientID, model string) localruntime.InstanceRecord {
	h.t.Helper()
	if h.srv.attachedClients(clientID) == 0 {
		h.attach(clientID)
	}
	var rec localruntime.InstanceRecord
	if err := h.call(clientID, "Start", localruntime.StartRequest{Runtime: "fake", ModelID: model}, &rec); err != nil {
		h.t.Fatalf("Start: %v", err)
	}
	return rec
}

func (h *harness) stop(clientID, instanceID string) {
	h.t.Helper()
	if err := h.call(clientID, "Stop", localruntime.StopRequest{InstanceID: instanceID}, nil); err != nil {
		h.t.Fatalf("Stop: %v", err)
	}
}

// attach opens an Attach stream and waits until the server has registered it.
func (h *harness) attach(clientID string) (events <-chan *proto.ManagerEvent, cancel func()) {
	h.t.Helper()
	ctx, cancelFn := context.WithCancel(authCtx(context.Background()))
	stream, err := h.client.Attach(ctx, &proto.AttachRequest{ClientId: clientID})
	if err != nil {
		h.t.Fatal(err)
	}
	// The server sends nothing on attach; wait for registration via a Call
	// round trip ordering is not guaranteed, so poll the attached count.
	deadline := time.Now().Add(2 * time.Second)
	for h.srv.attachedClients(clientID) == 0 {
		if time.Now().After(deadline) {
			h.t.Fatal("attach not registered")
		}
		time.Sleep(time.Millisecond)
	}
	ch := make(chan *proto.ManagerEvent, 64)
	go func() {
		defer close(ch)
		for {
			ev, err := stream.Recv()
			if err != nil {
				return
			}
			ch <- ev
		}
	}()
	h.t.Cleanup(cancelFn)
	return ch, cancelFn
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestTwoClientsStartSameModelShareOneInstance(t *testing.T) {
	h := newHarness(t)
	a := h.start("A", "m1")
	b := h.start("B", "m1")
	if h.provider.StartCalls() != 1 {
		t.Fatalf("StartCalls = %d, want 1", h.provider.StartCalls())
	}
	if a.ID == "" || a.ID != b.ID {
		t.Fatalf("instance IDs differ: %q vs %q", a.ID, b.ID)
	}
}

func TestStopKeepsInstanceWhileAnotherClientHoldsIt(t *testing.T) {
	h := newHarness(t)
	a := h.start("A", "m1")
	h.start("B", "m1")
	h.stop("A", a.ID)
	if h.provider.StopCalls() != 0 {
		t.Fatalf("StopCalls = %d after A stop, want 0", h.provider.StopCalls())
	}
	h.stop("B", a.ID)
	if h.provider.StopCalls() != 1 {
		t.Fatalf("StopCalls = %d after B stop, want 1", h.provider.StopCalls())
	}
}

func TestDroppedAttachReleasesHolds(t *testing.T) {
	h := newHarness(t)
	_, cancel := h.attach("A")
	h.start("A", "m1")
	cancel()
	waitFor(t, "stop after attach dropped", func() bool { return h.provider.StopCalls() == 1 })
}

func TestDroppedAttachKeepsInstanceHeldByOthers(t *testing.T) {
	h := newHarness(t)
	_, cancelA := h.attach("A")
	h.start("A", "m1")
	h.start("B", "m1")
	cancelA()
	waitFor(t, "A detached", func() bool { return h.srv.attachedClients("A") == 0 })
	time.Sleep(50 * time.Millisecond)
	if h.provider.StopCalls() != 0 {
		t.Fatalf("StopCalls = %d, want 0 while B holds", h.provider.StopCalls())
	}
}

func TestRestartKeepsHolds(t *testing.T) {
	h := newHarness(t)
	a := h.start("A", "m1")
	h.start("B", "m1")
	var restarted localruntime.InstanceRecord
	if err := h.call("A", "Restart", localruntime.RestartRequest{InstanceID: a.ID}, &restarted); err != nil {
		t.Fatal(err)
	}
	if restarted.ID != a.ID {
		t.Fatalf("restart changed instance id: %q -> %q", a.ID, restarted.ID)
	}
	// The manager's restart stops and relaunches the provider once; holds must
	// not cause any further stop.
	afterRestart := h.provider.StopCalls()
	h.stop("B", a.ID)
	if got := h.provider.StopCalls(); got != afterRestart {
		t.Fatalf("StopCalls = %d after B stop, want %d (A still holds)", got, afterRestart)
	}
	h.stop("A", a.ID)
	if got := h.provider.StopCalls(); got != afterRestart+1 {
		t.Fatalf("StopCalls = %d after A stop, want %d", got, afterRestart+1)
	}
}

func TestIdleTimeoutStopsAllAndCallsOnIdle(t *testing.T) {
	h := newHarness(t)
	_, cancel := h.attach("A")
	h.start("A", "m1")
	// An instance created internally (not by an unattached RPC) is swept on idle.
	if _, err := h.mgr.Start(context.Background(), localruntime.StartRequest{Runtime: "fake", ModelID: "m2"}); err != nil {
		t.Fatal(err)
	}
	cancel()
	waitFor(t, "A detached", func() bool { return h.srv.attachedClients("A") == 0 })
	waitFor(t, "A's hold released", func() bool { return h.provider.StopCalls() == 1 })

	h.srv.checkIdle(context.Background())
	select {
	case <-h.idle:
		t.Fatal("OnIdle fired before timeout")
	default:
	}
	h.clock.Advance(61 * time.Second)
	h.srv.checkIdle(context.Background())
	h.srv.checkIdle(context.Background())
	select {
	case <-h.idle:
	default:
		t.Fatal("OnIdle not called")
	}
	select {
	case <-h.idle:
		t.Fatal("OnIdle called twice")
	default:
	}
	if h.provider.StopCalls() != 2 {
		t.Fatalf("StopCalls = %d, want 2 (every instance stopped)", h.provider.StopCalls())
	}
}

func TestAttachResetsIdleTimer(t *testing.T) {
	h := newHarness(t)
	h.clock.Advance(2 * time.Minute)
	h.attach("A")
	h.srv.checkIdle(context.Background())
	select {
	case <-h.idle:
		t.Fatal("OnIdle fired while a stream is attached")
	default:
	}
}

func TestRejectsMissingOrWrongToken(t *testing.T) {
	h := newHarness(t)
	for name, ctx := range map[string]context.Context{
		"missing": context.Background(),
		"wrong":   metadata.AppendToOutgoingContext(context.Background(), tokenHeader, "nope"),
	} {
		_, err := h.client.Call(ctx, &proto.CallRequest{Method: "Providers"})
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s Call: code = %v, want Unauthenticated", name, status.Code(err))
		}
		_, err = h.client.Shutdown(ctx, &proto.ShutdownRequest{})
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s Shutdown: code = %v, want Unauthenticated", name, status.Code(err))
		}
		stream, err := h.client.Attach(ctx, &proto.AttachRequest{ClientId: "x"})
		if err == nil {
			_, err = stream.Recv()
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s Attach: code = %v, want Unauthenticated", name, status.Code(err))
		}
	}
}

func TestHelloNeedsNoTokenAndProvesIt(t *testing.T) {
	h := newHarness(t)
	nonce := []byte("0123456789abcdef0123456789abcdef")
	resp, err := h.client.Hello(context.Background(), &proto.HelloRequest{Nonce: nonce})
	if err != nil {
		t.Fatal(err)
	}
	if !hmac.Equal(resp.Proof, helloProof(testToken, nonce)) {
		t.Fatal("proof does not match HMAC(token, nonce)")
	}
	if hmac.Equal(resp.Proof, helloProof("other", nonce)) {
		t.Fatal("proof must depend on the token")
	}
}

func TestInstanceEventsReachEveryAttachedClient(t *testing.T) {
	h := newHarness(t)
	evA, _ := h.attach("A")
	evB, _ := h.attach("B")
	rec := h.start("C", "m1")
	for name, ch := range map[string]<-chan *proto.ManagerEvent{"A": evA, "B": evB} {
		select {
		case ev := <-ch:
			raw := ev.GetInstanceEventJson()
			if raw == "" {
				t.Fatalf("%s: not an instance event: %v", name, ev)
			}
			var ie localruntime.InstanceEvent
			if err := json.Unmarshal([]byte(raw), &ie); err != nil {
				t.Fatal(err)
			}
			if ie.Instance.ID != rec.ID {
				t.Errorf("%s: event instance %q, want %q", name, ie.Instance.ID, rec.ID)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: no event", name)
		}
	}
}

func TestHelloAndShutdown(t *testing.T) {
	h := newHarness(t)
	shut := make(chan struct{}, 1)
	h.srv.opts.OnShutdown = func() { shut <- struct{}{} }
	ctx := authCtx(context.Background())
	resp, err := h.client.Hello(ctx, &proto.HelloRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ApiVersion != APIVersion || resp.Version != "test-1" || resp.Pid == 0 {
		t.Fatalf("Hello = %+v", resp)
	}
	h.start("A", "m1")
	if _, err := h.client.Shutdown(ctx, &proto.ShutdownRequest{}); err != nil {
		t.Fatal(err)
	}
	if h.provider.StopCalls() != 1 {
		t.Fatalf("StopCalls = %d, want 1", h.provider.StopCalls())
	}
	select {
	case <-shut:
	default:
		t.Fatal("OnShutdown not called")
	}
}

func TestCallUnknownMethodAndEnroll(t *testing.T) {
	h := newHarness(t)
	err := h.call("A", "Bogus", nil, nil)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("unknown method code = %v", status.Code(err))
	}
	// InMemoryManager implements DownloadEnroller.
	if err := h.call("A", "EnrollDownload", localruntime.ModelRecord{ID: "x", Runtime: "fake"}, nil); err != nil {
		t.Fatalf("EnrollDownload: %v", err)
	}
	var infos []localruntime.ProviderInfo
	if err := h.call("A", "Providers", nil, &infos); err != nil || len(infos) != 1 || infos[0].Name != "fake" {
		t.Fatalf("Providers = %v, %v", infos, err)
	}
}

func TestEnrollDownloadWithoutEnrollerIsFailedPrecondition(t *testing.T) {
	s := NewServer(struct{ localruntime.Manager }{localruntime.NewManager()}, ServerOptions{Token: testToken})
	_, err := s.Call(authCtx2(), &proto.CallRequest{Method: "EnrollDownload", PayloadJson: `{}`})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v", status.Code(err))
	}
}

func authCtx2() context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(tokenHeader, testToken))
}

func TestSlowStreamDoesNotBlockManager(t *testing.T) {
	mgr := localruntime.NewManager()
	s := NewServer(mgr, ServerOptions{Token: testToken})
	// Register a stream that nobody drains.
	st := s.addStream("slow")
	defer s.removeStream("slow", st)
	done := make(chan struct{})
	go func() {
		for i := 0; i < streamBuffer*3; i++ {
			s.OnInstanceStateChange(localruntime.InstanceEvent{})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("observer blocked on a full stream")
	}
}

func (h *harness) restart(clientID string, req localruntime.RestartRequest) localruntime.InstanceRecord {
	h.t.Helper()
	var rec localruntime.InstanceRecord
	if err := h.call(clientID, "Restart", req, &rec); err != nil {
		h.t.Fatalf("Restart: %v", err)
	}
	return rec
}

func TestRestartWithNewInstanceIDKeepsHolds(t *testing.T) {
	for name, byName := range map[string]bool{"by id": false, "by runtime name": true} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.provider.UniqueIDs = true
			a := h.start("A", "m1")
			h.start("B", "m1")
			req := localruntime.RestartRequest{InstanceID: a.ID}
			if byName {
				req = localruntime.RestartRequest{Runtime: "fake"}
			}
			re := h.restart("A", req)
			if re.ID == a.ID {
				t.Fatalf("test needs a new instance id, got %q twice", re.ID)
			}
			if h.srv.holds.count(a.ID) != 0 || h.srv.holds.count(re.ID) != 2 {
				t.Fatalf("holds: old=%d new=%d, want 0 and 2", h.srv.holds.count(a.ID), h.srv.holds.count(re.ID))
			}
			afterRestart := h.provider.StopCalls()
			h.stop("B", re.ID)
			if h.provider.StopCalls() != afterRestart {
				t.Fatal("instance stopped while A still holds it")
			}
			h.stop("A", re.ID)
			if h.provider.StopCalls() != afterRestart+1 {
				t.Fatal("instance not stopped after last hold released")
			}
		})
	}
}

func TestReconnectRaceKeepsNewHold(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	req := localruntime.StartRequest{Runtime: "fake", ModelID: "m1"}
	old := h.srv.addStream("A")
	if _, err := h.srv.start(ctx, "A", req); err != nil {
		t.Fatal(err)
	}
	// The old stream ends, but before its release runs the client reconnects
	// and starts again.
	if !h.srv.removeStream("A", old) {
		t.Fatal("expected client to be fully detached")
	}
	fresh := h.srv.addStream("A")
	defer h.srv.removeStream("A", fresh)
	if _, err := h.srv.start(ctx, "A", req); err != nil {
		t.Fatal(err)
	}
	h.srv.releaseClient("A")
	if h.provider.StopCalls() != 0 {
		t.Fatalf("StopCalls = %d, reconnected client lost its hold", h.provider.StopCalls())
	}
	if h.srv.holds.count("fake-m1") != 1 {
		t.Fatalf("holds = %d, want 1", h.srv.holds.count("fake-m1"))
	}
}

func TestUnreachableInstanceIsSharedAndStopped(t *testing.T) {
	h := newHarness(t)
	a := h.start("A", "m1")
	hung := a
	hung.State = localruntime.InstanceUnreachable
	h.mgr.UpdateInstance(hung)
	b := h.start("B", "m1")
	if b.ID != a.ID || h.provider.StartCalls() != 1 {
		t.Fatalf("unreachable instance not shared: %q vs %q, starts=%d", a.ID, b.ID, h.provider.StartCalls())
	}
	if err := h.srv.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.provider.StopCalls() != 1 {
		t.Fatalf("StopCalls = %d, StopAll skipped the unreachable instance", h.provider.StopCalls())
	}
}

func TestSlowStartOfOneModelDoesNotBlockAnother(t *testing.T) {
	h := newHarness(t)
	h.attach("A")
	gate := make(chan struct{})
	entered := make(chan struct{})
	h.provider.OnStart = func(req localruntime.StartRequest) {
		if req.ModelID == "x" {
			close(entered)
			<-gate
		}
	}
	xDone := make(chan struct{})
	go func() {
		defer close(xDone)
		_, _ = h.srv.start(context.Background(), "A", localruntime.StartRequest{Runtime: "fake", ModelID: "x"})
	}()
	<-entered
	defer func() { close(gate); <-xDone }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		y := h.start("B", "y")
		h.stop("B", y.ID)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("operations on model y blocked behind the slow start of x")
	}
}

func TestUpdateInstanceIsNotRemotelyCallable(t *testing.T) {
	h := newHarness(t)
	err := h.call("A", "UpdateInstance", localruntime.InstanceRecord{ID: "fake-m1"}, nil)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("code = %v, want Unimplemented", status.Code(err))
	}
}

func TestResolveOpenModelEnvelope(t *testing.T) {
	h := newHarness(t)
	var res ResolveOpenModelResult
	err := h.call("A", "ResolveOpenModel", ResolveOpenModelRequest{Runtime: "fake", Want: "absent"}, &res)
	if err != nil {
		t.Fatal(err)
	}
	if res.Error != "ErrModelNotPresent" || res.Record.ID != "absent" {
		t.Fatalf("not present: %+v", res)
	}
	res = ResolveOpenModelResult{}
	if err := h.call("A", "ResolveOpenModel", ResolveOpenModelRequest{Runtime: "fake", Want: "nope"}, &res); err != nil {
		t.Fatal(err)
	}
	if res.Error != "ErrModelNotFound" {
		t.Fatalf("not found: %+v", res)
	}
	// A non-sentinel failure is a status error, not an envelope.
	err = h.call("A", "ResolveOpenModel", ResolveOpenModelRequest{Want: "x"}, nil)
	if err == nil {
		t.Fatal("expected error for missing runtime")
	}
	if err := h.call("A", "EnsureModelsPresent", EnsureModelsPresentRequest{Runtime: "fake"}, nil); err != nil {
		t.Fatalf("EnsureModelsPresent: %v", err)
	}
}

func TestStopAllDrainsInFlightStartAndRejectsNewOnes(t *testing.T) {
	h := newHarness(t)
	h.attach("A")
	gate := make(chan struct{})
	entered := make(chan struct{})
	h.provider.OnStart = func(req localruntime.StartRequest) {
		if req.ModelID == "x" {
			close(entered)
			<-gate
		}
	}
	type result struct {
		rec *localruntime.InstanceRecord
		err error
	}
	res := make(chan result, 1)
	go func() {
		rec, err := h.srv.start(context.Background(), "A", localruntime.StartRequest{Runtime: "fake", ModelID: "x"})
		res <- result{rec, err}
	}()
	<-entered
	if err := h.srv.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A Start after StopAll began is refused.
	err := h.call("B", "Start", localruntime.StartRequest{Runtime: "fake", ModelID: "y"}, nil)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Start during drain: code = %v, want Unavailable", status.Code(err))
	}
	close(gate)
	r := <-res
	if status.Code(r.err) != codes.Unavailable || r.rec != nil {
		t.Fatalf("in-flight Start = %v, %v; want Unavailable", r.rec, r.err)
	}
	insts, _ := h.mgr.Instances(context.Background())
	for _, inst := range insts {
		if liveInstance(inst) {
			t.Fatalf("instance %s left running after drain", inst.ID)
		}
	}
	if h.srv.holds.count("fake-x") != 0 {
		t.Fatal("hold left on drained instance")
	}
}

func TestStopAllRacingClientStopStopsOnce(t *testing.T) {
	h := newHarness(t)
	a := h.start("A", "m1")
	gate := make(chan struct{})
	entered := make(chan struct{})
	h.provider.OnStop = func(string) {
		select {
		case <-entered:
		default:
			close(entered)
			<-gate
		}
	}
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- h.call("A", "Stop", localruntime.StopRequest{InstanceID: a.ID}, nil)
	}()
	<-entered // the client's Stop is inside provider.Stop, holding the model lock
	allDone := make(chan error, 1)
	go func() { allDone <- h.srv.StopAll(context.Background()) }()
	time.Sleep(50 * time.Millisecond) // let StopAll queue behind the lock
	close(gate)
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := <-allDone; err != nil {
		t.Fatal(err)
	}
	if h.provider.StopCalls() != 1 {
		t.Fatalf("StopCalls = %d, want 1", h.provider.StopCalls())
	}
}

func TestStopAllSweepsLiveInstanceFromFailedStart(t *testing.T) {
	h := newHarness(t)
	h.attach("A")
	h.provider.FailAfterStart = true
	gate := make(chan struct{})
	entered := make(chan struct{})
	h.provider.OnStart = func(localruntime.StartRequest) {
		close(entered)
		<-gate
	}
	res := make(chan error, 1)
	go func() {
		_, err := h.srv.start(context.Background(), "A", localruntime.StartRequest{Runtime: "fake", ModelID: "x"})
		res <- err
	}()
	<-entered
	if err := h.srv.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(gate)
	if err := <-res; status.Code(err) != codes.Unavailable {
		t.Fatalf("Start err = %v, want Unavailable", err)
	}
	insts, _ := h.mgr.Instances(context.Background())
	if len(insts) == 0 {
		t.Fatal("test did not publish a live instance")
	}
	for _, inst := range insts {
		if liveInstance(inst) {
			t.Fatalf("instance %s orphaned after failed start during drain", inst.ID)
		}
	}
}

func TestIdleStopIsFinal(t *testing.T) {
	h := newHarness(t)
	h.clock.Advance(2 * time.Minute)
	h.srv.checkIdle(context.Background())
	select {
	case <-h.idle:
	default:
		t.Fatal("OnIdle not called")
	}
	ctx := authCtx(context.Background())
	if _, err := h.client.Hello(ctx, &proto.HelloRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("Hello after idle: %v, want Unavailable", err)
	}
	stream, err := h.client.Attach(ctx, &proto.AttachRequest{ClientId: "A"})
	if err == nil {
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Attach after idle: %v, want Unavailable", err)
	}
	h.srv.checkIdle(context.Background())
	select {
	case <-h.idle:
		t.Fatal("OnIdle fired twice")
	default:
	}
}

func TestAliasAndCanonicalModelShareOneInstance(t *testing.T) {
	h := newHarness(t)
	h.attach("A")
	h.provider.Models = append(h.provider.Models, localruntime.ModelRecord{ID: "canon", DisplayName: "alias", Runtime: "fake"})
	a := h.start("A", "alias")
	b := h.start("B", "canon")
	if h.provider.StartCalls() != 1 || a.ID != b.ID {
		t.Fatalf("starts=%d ids %q %q; alias and canonical must share one instance", h.provider.StartCalls(), a.ID, b.ID)
	}
	if a.ModelID != "canon" {
		t.Fatalf("ModelID = %q, want canon", a.ModelID)
	}
}

// drainingRestart runs restart for req with the launch gated, calls StopAll
// while the launch is in flight, then releases it. It returns restart's error.
func drainingRestart(t *testing.T, h *harness, req localruntime.RestartRequest) error {
	t.Helper()
	if h.srv.attachedClients("A") == 0 {
		h.attach("A")
	}
	h.provider.FailAfterStart = true
	gate := make(chan struct{})
	entered := make(chan struct{})
	h.provider.OnStart = func(localruntime.StartRequest) {
		close(entered)
		<-gate
	}
	res := make(chan error, 1)
	go func() {
		_, err := h.srv.restart(context.Background(), "A", req)
		res <- err
	}()
	<-entered
	if err := h.srv.StopAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(gate)
	return <-res
}

func assertNoLiveInstances(t *testing.T, h *harness) {
	t.Helper()
	insts, _ := h.mgr.Instances(context.Background())
	if len(insts) == 0 {
		t.Fatal("test did not publish a live instance")
	}
	for _, inst := range insts {
		if liveInstance(inst) {
			t.Fatalf("instance %s (model %q) orphaned after restart during drain", inst.ID, inst.ModelID)
		}
		if h.srv.holds.count(inst.ID) != 0 {
			t.Fatalf("hold left on drained instance %s", inst.ID)
		}
	}
}

func TestDrainingRestartByAliasSweepsCanonicalInstance(t *testing.T) {
	h := newHarness(t)
	h.provider.Models = append(h.provider.Models, localruntime.ModelRecord{ID: "canon", DisplayName: "alias", Runtime: "fake"})
	err := drainingRestart(t, h, localruntime.RestartRequest{Runtime: "fake", ModelID: "alias"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Restart err = %v, want Unavailable", err)
	}
	assertNoLiveInstances(t, h)
}

func TestDrainingRestartWithoutModelSweepsDefaultInstance(t *testing.T) {
	h := newHarness(t)
	h.provider.DefaultModel = "dflt"
	err := drainingRestart(t, h, localruntime.RestartRequest{Runtime: "fake"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Restart err = %v, want Unavailable", err)
	}
	assertNoLiveInstances(t, h)
}

func TestIdleCheckAfterShutdownDoesNothing(t *testing.T) {
	h := newHarness(t)
	h.start("A", "m1")
	if _, err := h.client.Shutdown(authCtx(context.Background()), &proto.ShutdownRequest{}); err != nil {
		t.Fatal(err)
	}
	stops := h.provider.StopCalls()
	h.clock.Advance(2 * time.Minute)
	h.srv.checkIdle(context.Background())
	select {
	case <-h.idle:
		t.Fatal("OnIdle called after Shutdown")
	default:
	}
	if got := h.provider.StopCalls(); got != stops {
		t.Fatalf("StopCalls = %d after idle check, want %d", got, stops)
	}
}

// TestAttachConfirmsRegistrationWithHeader: Attach sends its header only once
// the stream is registered, so a client that waits for the header knows no
// later event can be missed.
func TestAttachConfirmsRegistrationWithHeader(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(authCtx(context.Background()))
	defer cancel()
	stream, err := h.client.Attach(ctx, &proto.AttachRequest{ClientId: "hdr"})
	if err != nil {
		t.Fatal(err)
	}
	md, err := stream.Header()
	if err != nil {
		t.Fatal(err)
	}
	if len(md.Get(attachedHeader)) == 0 {
		t.Fatalf("Attach header lacks %s: %v", attachedHeader, md)
	}
	if n := h.srv.attachedClients("hdr"); n != 1 {
		t.Fatalf("attached streams at header time = %d, want 1", n)
	}
}
