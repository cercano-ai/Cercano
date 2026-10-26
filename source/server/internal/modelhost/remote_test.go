package modelhost

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/pkg/proto"
)

// remoteModels is the inventory every Remote test provider discovers.
func remoteModels() []localruntime.ModelRecord {
	return []localruntime.ModelRecord{
		{ID: "present", Runtime: "fake", DisplayName: "Present", Path: "/models/present.gguf", DownloadState: localruntime.Downloaded},
		{ID: "absent", Runtime: "fake", DisplayName: "Absent", Path: "/models/absent.gguf", DownloadState: localruntime.DownloadNotStarted},
	}
}

// testHost is a real Server over an InMemoryManager on a loopback listener.
type testHost struct {
	provider *localruntimetest.FakeProvider
	mgr      *localruntime.InMemoryManager
	srv      *Server
	g        *grpc.Server
	rec      Record
}

func startTestHost(t *testing.T, token string) *testHost {
	t.Helper()
	provider := localruntimetest.NewFakeProvider("fake", remoteModels()...)
	mgr := localruntime.NewManager()
	mgr.RegisterProvider(provider)
	srv := NewServer(mgr, ServerOptions{Token: token, Version: "v-test"})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	srv.Register(g)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	return &testHost{
		provider: provider, mgr: mgr, srv: srv, g: g,
		rec: Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: token, APIVersion: APIVersion, Version: "v-test", StartedAt: time.Now().UTC()},
	}
}

// publish writes h's record into dir, as a launched host would.
func (h *testHost) publish(t *testing.T, dir string) {
	t.Helper()
	if err := WriteRecord(dir, h.rec); err != nil {
		t.Fatal(err)
	}
}

func remoteOpts(dir string, launch func(context.Context) error) RemoteOptions {
	return RemoteOptions{
		Ensure:  EnsureOptions{RunDir: dir, ClientVersion: "c-test", Launch: launch, Timeout: 5 * time.Second},
		Profile: "default",
	}
}

func newTestRemote(t *testing.T, opts RemoteOptions) *Remote {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := NewRemote(ctx, opts)
	if err != nil {
		t.Fatalf("NewRemote: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// eventLog is an Observer that records instance events.
type eventLog struct {
	mu        sync.Mutex
	instances []localruntime.InstanceEvent
	downloads []localruntime.DownloadEvent
}

func (e *eventLog) OnDownloadStateChange(ev localruntime.DownloadEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.downloads = append(e.downloads, ev)
}

func (e *eventLog) OnInstanceStateChange(ev localruntime.InstanceEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.instances = append(e.instances, ev)
}

func (e *eventLog) sawInstance(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, ev := range e.instances {
		if ev.Instance.ID == id {
			return true
		}
	}
	return false
}

func (e *eventLog) instanceCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.instances)
}

var timeType = reflect.TypeOf(time.Time{})

// normalize zeroes every time.Time and every int field named PID reachable
// from v, in place. Timestamps differ between runs and lose their monotonic
// reading and location over JSON; PIDs are process-specific.
func normalize(v any) {
	normalizeValue(reflect.ValueOf(v))
}

func normalizeValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			normalizeValue(v.Elem())
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			normalizeValue(v.Index(i))
		}
	case reflect.Struct:
		if v.Type() == timeType {
			if v.CanSet() {
				v.Set(reflect.Zero(timeType))
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !f.CanSet() {
				continue
			}
			if v.Type().Field(i).Name == "PID" && f.Kind() == reflect.Int {
				f.SetInt(0)
				continue
			}
			normalizeValue(f)
		}
	}
}

// scenarioResult captures everything the parity scenario observes.
type scenarioResult struct {
	Providers      []localruntime.ProviderInfo
	Inventory      []localruntime.ModelRecord
	InventoryErr   string
	Started        *localruntime.InstanceRecord
	StartErr       string
	Instances      []localruntime.InstanceRecord
	Status         *localruntime.StatusSnapshot
	StatusErr      string
	Present        localruntime.ModelRecord
	PresentErr     string
	Absent         localruntime.ModelRecord
	AbsentErr      string
	AbsentSentinel bool
	Missing        localruntime.ModelRecord
	MissingErr     string
	MissingSent    bool
	Logs           []localruntime.LogEntry
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func runScenario(t *testing.T, m localruntime.Manager) scenarioResult {
	t.Helper()
	ctx := context.Background()
	m.SetEndpoints([]localruntime.EndpointRecord{{ID: "ep1", Kind: "openai", BaseURL: "http://127.0.0.1:9", Models: []string{"m"}}})
	var r scenarioResult
	var err error
	r.Providers = m.Providers()
	r.Inventory, err = m.Inventory(ctx)
	r.InventoryErr = errString(err)
	r.Started, err = m.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"})
	r.StartErr = errString(err)
	r.Instances, err = m.Instances(ctx)
	if err != nil {
		t.Fatalf("Instances: %v", err)
	}
	r.Status, err = m.Status(ctx)
	r.StatusErr = errString(err)
	r.Present, err = m.ResolveOpenModel(ctx, "fake", "present")
	r.PresentErr = errString(err)
	r.Absent, err = m.ResolveOpenModel(ctx, "fake", "absent")
	r.AbsentErr = errString(err)
	r.AbsentSentinel = errors.Is(err, localruntime.ErrModelNotPresent)
	r.Missing, err = m.ResolveOpenModel(ctx, "fake", "no-such-model")
	r.MissingErr = errString(err)
	r.MissingSent = errors.Is(err, localruntime.ErrModelNotFound)
	r.Logs, err = m.Logs(ctx, localruntime.LogRequest{Tail: 50})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	normalize(&r)
	return r
}

// TestRemoteMatchesInMemoryManager runs one scenario directly against an
// InMemoryManager and through Server+Remote (separate managers, same setup)
// and requires identical results once timestamps and PIDs are normalized.
func TestRemoteMatchesInMemoryManager(t *testing.T) {
	local := localruntime.NewManager()
	local.RegisterProvider(localruntimetest.NewFakeProvider("fake", remoteModels()...))
	want := runScenario(t, local)

	dir := t.TempDir()
	host := startTestHost(t, "tok-parity")
	host.publish(t, dir)
	remote := newTestRemote(t, remoteOpts(dir, nil))
	got := runScenario(t, remote)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Remote diverges from InMemoryManager:\n got: %+v\nwant: %+v", got, want)
	}
	if !want.AbsentSentinel || !want.MissingSent || want.Absent.ID != "absent" || want.Started == nil {
		t.Fatalf("scenario did not exercise what it should: %+v", want)
	}
}

func TestRemoteResolveErrorsKeepSentinels(t *testing.T) {
	dir := t.TempDir()
	startTestHost(t, "tok-sentinel").publish(t, dir)
	r := newTestRemote(t, remoteOpts(dir, nil))
	ctx := context.Background()

	rec, err := r.ResolveOpenModel(ctx, "fake", "absent")
	if !errors.Is(err, localruntime.ErrModelNotPresent) {
		t.Fatalf("absent: err = %v, want ErrModelNotPresent", err)
	}
	if rec.ID != "absent" || rec.Path != "/models/absent.gguf" {
		t.Fatalf("absent: record not carried with the sentinel: %+v", rec)
	}
	if _, err := r.ResolveOpenModel(ctx, "fake", "nope"); !errors.Is(err, localruntime.ErrModelNotFound) {
		t.Fatalf("missing: err = %v, want ErrModelNotFound", err)
	}
	if rec, err := r.ResolveOpenModel(ctx, "fake", "present"); err != nil || rec.ID != "present" {
		t.Fatalf("present: rec=%+v err=%v", rec, err)
	}
	// A plain (non-sentinel) call error keeps the host's message.
	err = r.EnsureModelsPresent(ctx, "fake", []string{"nope"})
	local := localruntime.NewManager()
	local.RegisterProvider(localruntimetest.NewFakeProvider("fake", remoteModels()...))
	localErr := local.EnsureModelsPresent(ctx, "fake", []string{"nope"})
	if err == nil || localErr == nil || !strings.Contains(err.Error(), localErr.Error()) {
		t.Fatalf("EnsureModelsPresent error differs: remote=%v local=%v", err, localErr)
	}
}

func TestRemoteEndpointsArePerClient(t *testing.T) {
	dir := t.TempDir()
	startTestHost(t, "tok-endpoints").publish(t, dir)
	a := newTestRemote(t, remoteOpts(dir, nil))
	b := newTestRemote(t, remoteOpts(dir, nil))
	a.SetEndpoints([]localruntime.EndpointRecord{{ID: "a"}})
	b.SetEndpoints([]localruntime.EndpointRecord{{ID: "b1"}, {ID: "b2"}})
	ctx := context.Background()
	ea, err := a.Endpoints(ctx)
	if err != nil || len(ea) != 1 || ea[0].ID != "a" {
		t.Fatalf("a endpoints = %+v, %v", ea, err)
	}
	eb, err := b.Endpoints(ctx)
	if err != nil || len(eb) != 2 || eb[0].ID != "b1" || eb[1].ID != "b2" {
		t.Fatalf("b endpoints = %+v, %v", eb, err)
	}
	st, err := a.Status(ctx)
	if err != nil || len(st.Endpoints) != 1 || st.Endpoints[0].ID != "a" {
		t.Fatalf("a status endpoints = %+v, %v", st, err)
	}
	// The caller's slice is copied, not aliased.
	in := []localruntime.EndpointRecord{{ID: "x"}}
	a.SetEndpoints(in)
	in[0].ID = "mutated"
	if ea, _ := a.Endpoints(ctx); ea[0].ID != "x" {
		t.Fatalf("SetEndpoints aliased the caller's slice: %+v", ea)
	}
}

func TestRemoteReconnectsAfterHostRestart(t *testing.T) {
	dir := t.TempDir()
	first := startTestHost(t, "tok-first")
	first.publish(t, dir)

	var launches atomic.Int32
	var mu sync.Mutex
	var second *testHost
	launch := func(context.Context) error {
		launches.Add(1)
		mu.Lock()
		defer mu.Unlock()
		if second == nil {
			second = startTestHost(t, "tok-second")
		}
		return WriteRecord(dir, second.rec)
	}
	r := newTestRemote(t, remoteOpts(dir, launch))
	obs := &eventLog{}
	r.RegisterObserver(obs)
	ctx := context.Background()

	inst, err := r.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"})
	if err != nil {
		t.Fatalf("Start on first host: %v", err)
	}
	waitUntil(t, "first instance event", func() bool { return obs.sawInstance(inst.ID) })

	// The first host dies and its record goes with it.
	first.g.Stop()
	if err := RemoveRecord(dir, first.rec.PID); err != nil {
		t.Fatal(err)
	}

	// Concurrent callers share one recovery.
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = r.Instances(ctx)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Instances[%d] after host restart: %v", i, err)
		}
	}
	if n := launches.Load(); n != 1 {
		t.Fatalf("launches = %d, want 1", n)
	}

	before := obs.instanceCount()
	inst2, err := r.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"})
	if err != nil {
		t.Fatalf("Start on second host: %v", err)
	}
	mu.Lock()
	host2 := second
	mu.Unlock()
	if host2.provider.StartCalls() != 1 {
		t.Fatalf("second host start calls = %d, want 1", host2.provider.StartCalls())
	}
	waitUntil(t, "second host instance event", func() bool {
		return obs.instanceCount() > before && obs.sawInstance(inst2.ID)
	})
}

func TestRemoteRecoversWhenStreamBreaksWithoutCalls(t *testing.T) {
	dir := t.TempDir()
	first := startTestHost(t, "tok-a")
	first.publish(t, dir)
	var mu sync.Mutex
	var second *testHost
	launch := func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		if second == nil {
			second = startTestHost(t, "tok-b")
		}
		return WriteRecord(dir, second.rec)
	}
	r := newTestRemote(t, remoteOpts(dir, launch))
	first.g.Stop()
	if err := RemoveRecord(dir, first.rec.PID); err != nil {
		t.Fatal(err)
	}
	// No call is made: the broken Attach stream alone drives the relaunch
	// and re-attach.
	waitUntil(t, "re-attach to the relaunched host", func() bool {
		mu.Lock()
		h := second
		mu.Unlock()
		return h != nil && h.srv.attachedClients(r.ClientID()) == 1
	})
}

func TestRemoteCloseReleasesHolds(t *testing.T) {
	dir := t.TempDir()
	host := startTestHost(t, "tok-close")
	host.publish(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := NewRemote(ctx, remoteOpts(dir, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	waitUntil(t, "hold release after Close", func() bool { return host.provider.StopCalls() == 1 })
	if _, err := r.Instances(ctx); err == nil {
		t.Fatal("call after Close succeeded")
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestRemoteNoObserverCallsAfterClose(t *testing.T) {
	dir := t.TempDir()
	host := startTestHost(t, "tok-obs")
	host.publish(t, dir)
	r := newTestRemote(t, remoteOpts(dir, nil))
	other := newTestRemote(t, remoteOpts(dir, nil))
	obs := &eventLog{}
	r.RegisterObserver(obs)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	n := obs.instanceCount()
	if _, err := other.Start(context.Background(), localruntime.StartRequest{Runtime: "fake", ModelID: "present"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if obs.instanceCount() != n {
		t.Fatalf("observer called after Close: %d -> %d", n, obs.instanceCount())
	}
}

func TestRemoteRejectsEmptyToken(t *testing.T) {
	dir := t.TempDir()
	imp := &impostor{}
	addr := serve(t, imp)
	if err := WriteRecord(dir, Record{PID: 1, Addr: addr, APIVersion: APIVersion}); err != nil {
		t.Fatal(err)
	}
	// Ensure itself would reject this host (its proof is wrong), so stub the
	// discovery step to hand Remote the empty-token record directly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := newRemote(ctx, remoteOpts(dir, nil), func(context.Context, EnsureOptions) (Record, error) {
		return Record{PID: 1, Addr: addr, APIVersion: APIVersion}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("NewRemote with empty token: err = %v", err)
	}
	if imp.tokenSeen.Load() != 0 || imp.hellos.Load() != 0 {
		t.Fatalf("Remote contacted a host whose record has no token (hellos=%d)", imp.hellos.Load())
	}
}

func TestRemoteGeneratesClientIDAndAttaches(t *testing.T) {
	dir := t.TempDir()
	host := startTestHost(t, "tok-id")
	host.publish(t, dir)
	a := newTestRemote(t, remoteOpts(dir, nil))
	b := newTestRemote(t, remoteOpts(dir, nil))
	if a.ClientID() == "" || a.ClientID() == b.ClientID() {
		t.Fatalf("client IDs %q, %q", a.ClientID(), b.ClientID())
	}
	named := remoteOpts(dir, nil)
	named.ClientID = "agent-7"
	c := newTestRemote(t, named)
	if c.ClientID() != "agent-7" {
		t.Fatalf("ClientID = %q", c.ClientID())
	}
	// NewRemote returns only once the host has registered the stream.
	for _, r := range []*Remote{a, b, c} {
		if n := host.srv.attachedClients(r.ClientID()); n != 1 {
			t.Fatalf("client %s attached streams = %d", r.ClientID(), n)
		}
	}
}

func TestRemoteRegisterProviderAndUpdateInstanceAreLocalNoOps(t *testing.T) {
	dir := t.TempDir()
	host := startTestHost(t, "tok-noop")
	host.publish(t, dir)
	r := newTestRemote(t, remoteOpts(dir, nil))
	r.RegisterProvider(localruntimetest.NewFakeProvider("other"))
	if ps := host.mgr.Providers(); len(ps) != 1 || ps[0].Name != "fake" {
		t.Fatalf("host providers changed: %+v", ps)
	}
	logs, err := host.mgr.Logs(context.Background(), localruntime.LogRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var warned bool
	for _, l := range logs {
		if l.Level == "warn" && strings.Contains(l.Message, "RegisterProvider") && strings.Contains(l.Message, "other") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("no RegisterProvider warning in host logs: %+v", logs)
	}
	r.UpdateInstance(localruntime.InstanceRecord{ID: "ghost", Runtime: "fake", State: localruntime.InstanceRunning})
	if insts, _ := host.mgr.Instances(context.Background()); len(insts) != 0 {
		t.Fatalf("UpdateInstance reached the host: %+v", insts)
	}
}

func TestRemoteEnrollDownload(t *testing.T) {
	dir := t.TempDir()
	host := startTestHost(t, "tok-enroll")
	host.publish(t, dir)
	r := newTestRemote(t, remoteOpts(dir, nil))
	var _ localruntime.DownloadEnroller = r
	r.EnrollDownload(localruntime.ModelRecord{ID: "cat", Runtime: "fake", Path: "/models/cat.gguf", DownloadURL: "http://127.0.0.1:9/cat.gguf"})
	// The host's manager now carries the enrolled record, so its
	// DownloadModel finds it instead of failing "not found".
	inv, err := host.mgr.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range inv {
		if m.ID == "cat" && m.DownloadURL == "http://127.0.0.1:9/cat.gguf" {
			return
		}
	}
	t.Fatalf("enrolled record missing from host inventory: %+v", inv)
}

// TestVerifiedConnNeverRedials: once the host dies, a verified connection
// must not transparently reconnect to whatever binds the same port next, or
// that process would receive the token.
func TestVerifiedConnNeverRedials(t *testing.T) {
	host := startTestHost(t, "tok-redial")
	conn, err := Dial(context.Background(), host.rec)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := proto.NewModelHostClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Call(ctx, &proto.CallRequest{ClientId: "c", Method: "Providers"}); err != nil {
		t.Fatalf("Call on verified conn: %v", err)
	}
	host.g.Stop()

	lis, err := net.Listen("tcp", host.rec.Addr)
	if err != nil {
		t.Skipf("cannot rebind %s: %v", host.rec.Addr, err)
	}
	imp := &impostor{}
	g := grpc.NewServer()
	proto.RegisterModelHostServer(g, imp)
	go g.Serve(lis)
	defer g.Stop()

	for i := 0; i < 5; i++ {
		cctx, ccancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_, err := client.Call(cctx, &proto.CallRequest{ClientId: "c", Method: "Providers"})
		ccancel()
		if err == nil {
			t.Fatal("call on a dead host's conn succeeded")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if imp.tokenSeen.Load() != 0 || imp.calls.Load() != 0 {
		t.Fatalf("impostor on the reused port received %d calls (%d with the token)", imp.calls.Load(), imp.tokenSeen.Load())
	}
}

// TestRemoteReconnectToSameHostKeepsHolds: a reconnect to the host it is
// already attached to must attach the new stream before ending the old one,
// so the host never sees this client gone and keeps its holds.
func TestRemoteReconnectToSameHostKeepsHolds(t *testing.T) {
	dir := t.TempDir()
	host := startTestHost(t, "tok-same")
	host.publish(t, dir)
	r := newTestRemote(t, remoteOpts(dir, nil))
	ctx := context.Background()
	inst, err := r.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"})
	if err != nil {
		t.Fatal(err)
	}
	epoch := r.HoldEpoch()
	for i := 0; i < 5; i++ {
		r.mu.Lock()
		gen := r.gen
		r.mu.Unlock()
		if err := r.reconnect(ctx, gen); err != nil {
			t.Fatalf("reconnect: %v", err)
		}
	}
	// Give the host time to process the old streams ending.
	time.Sleep(200 * time.Millisecond)
	if n := host.provider.StopCalls(); n != 0 {
		t.Fatalf("reconnecting to the same host stopped the held model (stop calls = %d)", n)
	}
	if host.srv.holds.count(inst.ID) != 1 {
		t.Fatalf("holds on %s = %d after reconnect, want 1", inst.ID, host.srv.holds.count(inst.ID))
	}
	if n := host.srv.attachedClients(r.ClientID()); n != 1 {
		t.Fatalf("attached streams = %d after reconnect, want 1", n)
	}
	if r.HoldEpoch() == epoch {
		t.Fatal("HoldEpoch unchanged after reconnect; cached holds would never be re-taken")
	}
}

// TestNewRemoteFailsFastOnNewerHost: an agent older than the running host
// gets ErrNewerModelHost at once (so it can host models in-process) and
// neither shuts the host down nor launches another.
func TestNewRemoteFailsFastOnNewerHost(t *testing.T) {
	dir := t.TempDir()
	host := &newerHost{}
	addr := serve(t, host)
	if err := WriteRecord(dir, Record{PID: 4244, Addr: addr, Token: "tok", APIVersion: APIVersion + 1}); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	_, err := NewRemote(ctx, remoteOpts(dir, func(context.Context) error { launches.Add(1); return nil }))
	if !errors.Is(err, ErrNewerModelHost) {
		t.Fatalf("NewRemote err = %v, want ErrNewerModelHost", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("NewRemote took %s to give up on a newer host", time.Since(start))
	}
	if host.shutdown.Load() != 0 || launches.Load() != 0 {
		t.Fatalf("shutdown=%d launches=%d, want 0 and 0", host.shutdown.Load(), launches.Load())
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestRemoteRecoveryLogsNewerHostOnce: when the host a long-lived Remote was
// using is replaced by a newer one, background recovery keeps failing with
// the same error; it is logged once, not on every retry.
func TestRemoteRecoveryLogsNewerHostOnce(t *testing.T) {
	var logs syncBuffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	dir := t.TempDir()
	first := startTestHost(t, "tok-first")
	first.publish(t, dir)
	var launches atomic.Int32
	r := newTestRemote(t, remoteOpts(dir, func(context.Context) error { launches.Add(1); return nil }))

	newer := &newerHost{}
	addr := serve(t, newer)
	// Publish the replacement before disconnecting the client. Otherwise recovery
	// can observe the dead old host's record and legitimately launch a host before
	// this test publishes the newer one.
	if err := WriteRecord(dir, Record{PID: 4244, Addr: addr, Token: "tok", APIVersion: APIVersion + 1}); err != nil {
		t.Fatal(err)
	}
	first.g.Stop()
	// Recovery retries with backoff 100ms, 200ms, 400ms, 800ms, ...
	time.Sleep(2 * time.Second)
	r.Close()

	if n := strings.Count(logs.String(), "newer than this client"); n != 1 {
		t.Fatalf("newer-host error logged %d times, want once:\n%s", n, logs.String())
	}
	if newer.shutdown.Load() != 0 || launches.Load() != 0 {
		t.Fatalf("shutdown=%d launches=%d, want 0 and 0", newer.shutdown.Load(), launches.Load())
	}
}
