package modelhost

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/pkg/proto"
)

// startHost runs a real Server on a loopback listener and returns its record.
func startHost(t *testing.T) Record {
	t.Helper()
	mgr := localruntime.NewManager()
	mgr.RegisterProvider(localruntimetest.NewFakeProvider("fake"))
	srv := NewServer(mgr, ServerOptions{Token: "tok", Version: "v-test"})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	srv.Register(g)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	return Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: "tok", APIVersion: APIVersion, Version: "v-test", StartedAt: time.Now().UTC()}
}

func testOpts(dir string, launch func(context.Context) error) EnsureOptions {
	return EnsureOptions{RunDir: dir, ClientVersion: "c1", Launch: launch, Timeout: 5 * time.Second}
}

func TestEnsureReusesLiveHost(t *testing.T) {
	dir := t.TempDir()
	rec := startHost(t)
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	got, err := Ensure(context.Background(), testOpts(dir, func(context.Context) error { launches.Add(1); return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if launches.Load() != 0 || got.Addr != rec.Addr {
		t.Fatalf("launches=%d got=%+v", launches.Load(), got)
	}
}

func TestEnsureLaunchesWhenNoRecord(t *testing.T) {
	dir := t.TempDir()
	var launches atomic.Int32
	var want Record
	got, err := Ensure(context.Background(), testOpts(dir, func(context.Context) error {
		launches.Add(1)
		want = startHost(t)
		return WriteRecord(dir, want)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if launches.Load() != 1 || got.Addr != want.Addr {
		t.Fatalf("launches=%d got=%+v want=%+v", launches.Load(), got, want)
	}
}

// oldHost reports an older api_version and removes its record on Shutdown.
type oldHost struct {
	proto.UnimplementedModelHostServer
	dir      string
	shutdown atomic.Int32
}

func (o *oldHost) Hello(_ context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	return &proto.HelloResponse{ApiVersion: APIVersion - 1, Version: "old", Pid: 4242, Proof: helloProof("tok", req.GetNonce())}, nil
}

func (o *oldHost) Shutdown(context.Context, *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	o.shutdown.Add(1)
	_ = RemoveRecord(o.dir, 4242)
	return &proto.ShutdownResponse{}, nil
}

func TestEnsureReplacesIncompatibleHost(t *testing.T) {
	dir := t.TempDir()
	old := &oldHost{dir: dir}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	proto.RegisterModelHostServer(g, old)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	if err := WriteRecord(dir, Record{PID: 4242, Addr: lis.Addr().String(), Token: "tok", APIVersion: APIVersion - 1}); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	var want Record
	got, err := Ensure(context.Background(), testOpts(dir, func(context.Context) error {
		launches.Add(1)
		if old.shutdown.Load() != 1 {
			t.Errorf("launch before shutdown")
		}
		want = startHost(t)
		return WriteRecord(dir, want)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if old.shutdown.Load() != 1 || launches.Load() != 1 || got.Addr != want.Addr {
		t.Fatalf("shutdown=%d launches=%d got=%+v", old.shutdown.Load(), launches.Load(), got)
	}
}

func TestConcurrentEnsureLaunchesOnce(t *testing.T) {
	dir := t.TempDir()
	var launches atomic.Int32
	launch := func(context.Context) error {
		launches.Add(1)
		rec := startHost(t)
		rec.PID = 777
		time.Sleep(200 * time.Millisecond)
		return WriteRecord(dir, rec)
	}
	var wg sync.WaitGroup
	pids := make([]int, 8)
	errs := make([]error, 8)
	for i := range pids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := Ensure(context.Background(), testOpts(dir, launch))
			pids[i], errs[i] = r.PID, err
		}()
	}
	wg.Wait()
	for i := range pids {
		if errs[i] != nil || pids[i] != 777 {
			t.Fatalf("goroutine %d: pid=%d err=%v", i, pids[i], errs[i])
		}
	}
	if launches.Load() != 1 {
		t.Fatalf("launches = %d, want 1", launches.Load())
	}
}

func TestEnsureIgnoresStaleRecord(t *testing.T) {
	dir := t.TempDir()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := lis.Addr().String()
	lis.Close()
	if err := WriteRecord(dir, Record{PID: 99, Addr: dead, Token: "tok", APIVersion: APIVersion}); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	var want Record
	got, err := Ensure(context.Background(), testOpts(dir, func(context.Context) error {
		launches.Add(1)
		want = startHost(t)
		return WriteRecord(dir, want)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if launches.Load() != 1 || got.Addr != want.Addr {
		t.Fatalf("launches=%d got=%+v", launches.Load(), got)
	}
}

func TestEnsureWrongTokenIsNotLive(t *testing.T) {
	dir := t.TempDir()
	rec := startHost(t)
	rec.Token = "wrong"
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	_, err := Ensure(context.Background(), EnsureOptions{RunDir: dir, Timeout: 500 * time.Millisecond,
		Launch: func(context.Context) error { launches.Add(1); return nil }})
	if err == nil || launches.Load() != 1 {
		t.Fatalf("err=%v launches=%d", err, launches.Load())
	}
}

func TestEnsureTimesOutWhenHostNeverAppears(t *testing.T) {
	_, err := Ensure(context.Background(), EnsureOptions{RunDir: t.TempDir(), Timeout: 300 * time.Millisecond,
		Launch: func(context.Context) error { return nil }})
	if err == nil {
		t.Fatal("want timeout error")
	}
}

func TestLaunchBinarySpawnsModelHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script launcher")
	}
	runDir := filepath.Join(t.TempDir(), "run")
	marker := filepath.Join(t.TempDir(), "marker")
	script := filepath.Join(t.TempDir(), "fake-cercano")
	body := "#!/bin/sh\necho \"argv:$*\" > " + marker + "\necho hello-log\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := LaunchBinary(script, runDir)(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		data, err := os.ReadFile(marker)
		if err == nil && strings.TrimSpace(string(data)) == "argv:model-host" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker not written correctly: %q %v", data, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for {
		log, _ := os.ReadFile(filepath.Join(runDir, "modelhost.log"))
		if strings.Contains(string(log), "hello-log") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("log missing output: %q", log)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// stubbornHost acks Shutdown but keeps answering Hello with an older version.
type stubbornHost struct {
	proto.UnimplementedModelHostServer
	shutdown atomic.Int32
}

func (o *stubbornHost) Hello(_ context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	return &proto.HelloResponse{ApiVersion: APIVersion - 1, Pid: 4243, Proof: helloProof("tok", req.GetNonce())}, nil
}

func (o *stubbornHost) Shutdown(context.Context, *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	o.shutdown.Add(1)
	return &proto.ShutdownResponse{}, nil
}

func serve(t *testing.T, srv proto.ModelHostServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	proto.RegisterModelHostServer(g, srv)
	go g.Serve(lis)
	t.Cleanup(g.Stop)
	return lis.Addr().String()
}

func TestEnsureLaunchesAfterReplaceWaitElapses(t *testing.T) {
	old := replaceWait
	replaceWait = 400 * time.Millisecond
	t.Cleanup(func() { replaceWait = old })
	dir := t.TempDir()
	host := &stubbornHost{}
	addr := serve(t, host)
	if err := WriteRecord(dir, Record{PID: 4243, Addr: addr, Token: "tok", APIVersion: APIVersion - 1}); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	var want Record
	start := time.Now()
	got, err := Ensure(context.Background(), testOpts(dir, func(context.Context) error {
		launches.Add(1)
		want = startHost(t)
		return WriteRecord(dir, want)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if host.shutdown.Load() != 1 || launches.Load() != 1 || got.Addr != want.Addr {
		t.Fatalf("shutdown=%d launches=%d got=%+v", host.shutdown.Load(), launches.Load(), got)
	}
	if time.Since(start) < 400*time.Millisecond {
		t.Fatalf("launched before the replace wait elapsed (%v)", time.Since(start))
	}
}

func TestEnsureDoesNotShutDownIncompatibleHostWithoutLauncher(t *testing.T) {
	dir := t.TempDir()
	host := &stubbornHost{}
	addr := serve(t, host)
	if err := WriteRecord(dir, Record{PID: 4243, Addr: addr, Token: "tok", APIVersion: APIVersion - 1}); err != nil {
		t.Fatal(err)
	}
	_, err := Ensure(context.Background(), EnsureOptions{RunDir: dir, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "incompatible model host running and no launcher configured") {
		t.Fatalf("err = %v", err)
	}
	if host.shutdown.Load() != 0 {
		t.Fatal("incompatible host was shut down without a launcher")
	}
}

// impostor answers Hello with a proof made from the wrong token and records
// whether any call ever carried a token.
type impostor struct {
	proto.UnimplementedModelHostServer
	tokenSeen atomic.Int32
	hellos    atomic.Int32
	calls     atomic.Int32
}

func (i *impostor) note(ctx context.Context) {
	if md, ok := metadata.FromIncomingContext(ctx); ok && len(md.Get(tokenHeader)) > 0 {
		i.tokenSeen.Add(1)
	}
}

func (i *impostor) Hello(ctx context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	i.hellos.Add(1)
	i.note(ctx)
	return &proto.HelloResponse{ApiVersion: APIVersion, Pid: 1, Proof: helloProof("not-the-token", req.GetNonce())}, nil
}

func (i *impostor) Shutdown(ctx context.Context, _ *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	i.note(ctx)
	return &proto.ShutdownResponse{}, nil
}

func (i *impostor) Call(ctx context.Context, _ *proto.CallRequest) (*proto.CallResponse, error) {
	i.calls.Add(1)
	i.note(ctx)
	return &proto.CallResponse{}, nil
}

func TestDialRejectsHostWithWrongProofAndNeverSendsToken(t *testing.T) {
	imp := &impostor{}
	addr := serve(t, imp)
	rec := Record{PID: 1, Addr: addr, Token: "tok", APIVersion: APIVersion}
	if conn, err := Dial(context.Background(), rec); err == nil {
		conn.Close()
		t.Fatal("Dial accepted a host with a bad proof")
	}
	dir := t.TempDir()
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	_, err := Ensure(context.Background(), EnsureOptions{RunDir: dir, Timeout: 400 * time.Millisecond,
		Launch: func(context.Context) error { launches.Add(1); return nil }})
	if err == nil || launches.Load() != 1 {
		t.Fatalf("err=%v launches=%d", err, launches.Load())
	}
	if imp.tokenSeen.Load() != 0 {
		t.Fatalf("impostor received the token on %d calls", imp.tokenSeen.Load())
	}
}

func TestDialRejectsNonLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"10.1.2.3:5000", "example.com:5000", "localhost:5000", "[2001:db8::1]:5000", "nonsense"} {
		if conn, err := Dial(context.Background(), Record{Addr: addr, Token: "tok"}); err == nil {
			conn.Close()
			t.Errorf("Dial accepted %q", addr)
		}
	}
}

func TestDialAcceptsVerifiedHostAndAttachesToken(t *testing.T) {
	rec := startHost(t)
	conn, err := Dial(context.Background(), rec)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := proto.NewModelHostClient(conn).Call(context.Background(), &proto.CallRequest{Method: "Providers"}); err != nil {
		t.Fatalf("token-bearing call failed: %v", err)
	}
}

// newerHost answers Hello with a newer api_version than this client's.
type newerHost struct {
	proto.UnimplementedModelHostServer
	shutdown atomic.Int32
}

func (n *newerHost) Hello(_ context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	return &proto.HelloResponse{ApiVersion: APIVersion + 1, Version: "newer", Pid: 4244, Proof: helloProof("tok", req.GetNonce())}, nil
}

func (n *newerHost) Shutdown(context.Context, *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	n.shutdown.Add(1)
	return &proto.ShutdownResponse{}, nil
}

// TestEnsureLeavesNewerHostRunning: an older client must not shut down a
// newer host (or launch a competing one); it reports ErrNewerModelHost.
func TestEnsureLeavesNewerHostRunning(t *testing.T) {
	dir := t.TempDir()
	host := &newerHost{}
	addr := serve(t, host)
	if err := WriteRecord(dir, Record{PID: 4244, Addr: addr, Token: "tok", APIVersion: APIVersion + 1}); err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	start := time.Now()
	_, err := Ensure(context.Background(), testOpts(dir, func(context.Context) error { launches.Add(1); return nil }))
	if !errors.Is(err, ErrNewerModelHost) {
		t.Fatalf("err = %v, want ErrNewerModelHost", err)
	}
	if host.shutdown.Load() != 0 || launches.Load() != 0 {
		t.Fatalf("newer host: shutdown=%d launches=%d, want 0 and 0", host.shutdown.Load(), launches.Load())
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("Ensure took %s to report a newer host", time.Since(start))
	}
}

// TestLaunchBinaryRunsInRunDir: the host outlives the agent that launched it,
// so it must not keep that agent's working directory.
func TestLaunchBinaryRunsInRunDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script launcher")
	}
	runDir := filepath.Join(t.TempDir(), "run")
	marker := filepath.Join(t.TempDir(), "marker")
	script := filepath.Join(t.TempDir(), "fake-cercano")
	body := "#!/bin/sh\npwd -P > " + marker + ".tmp && mv " + marker + ".tmp " + marker + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := LaunchBinary(script, runDir)(context.Background()); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(runDir)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		data, err := os.ReadFile(marker)
		if err == nil {
			if got := strings.TrimSpace(string(data)); got != want {
				t.Fatalf("host working directory = %q, want %q", got, want)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker not written: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
