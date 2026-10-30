package main

import (
	"context"
	"errors"
	"net"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// Synthetic old API advertisement; real attachment/lifecycle, fake runtime only.
type ordinaryUpgradeHost struct {
	*modelhost.Server
	api       atomic.Int32
	shutdowns atomic.Int32
}

func (h *ordinaryUpgradeHost) Hello(ctx context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	r, err := h.Server.Hello(ctx, req)
	if err == nil {
		r.ApiVersion = h.api.Load()
	}
	return r, err
}
func (h *ordinaryUpgradeHost) Shutdown(ctx context.Context, req *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	h.shutdowns.Add(1)
	return h.Server.Shutdown(ctx, req)
}

func TestOrdinaryStartupRefusesOlderHostAndFallsBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir, err := modelhost.RunDir()
	if err != nil {
		t.Fatal(err)
	}
	mgr := localruntime.NewManager()
	provider := localruntimetest.NewFakeProvider("fake")
	mgr.RegisterProvider(provider)
	h := &ordinaryUpgradeHost{Server: modelhost.NewServer(mgr, modelhost.ServerOptions{Token: "tok", OnShutdown: func() { _ = modelhost.RemoveRecord(dir, os.Getpid()) }})}
	h.api.Store(modelhost.APIVersion)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	proto.RegisterModelHostServer(g, h)
	go g.Serve(lis)
	defer g.Stop()
	rec := modelhost.Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: "tok", APIVersion: modelhost.APIVersion}
	if err := modelhost.WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := modelhost.NewRemote(ctx, modelhost.RemoteOptions{ClientID: "A", Ensure: modelhost.EnsureOptions{RunDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"}); err != nil {
		t.Fatal(err)
	}
	launches := 0
	prev := launchModelHost
	launchModelHost = func(string) func(context.Context) error {
		return func(context.Context) error { launches++; return errors.New("offline unexpected replacement") }
	}
	defer func() { launchModelHost = prev }()
	// Same-API ordinary B shares A's host and closes only its attachment.
	b, err := newModelHostRemote(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	h.api.Store(modelhost.APIVersion - 1)
	d, local, calls := runtimeFixture(t)
	d.remote = productionRuntimeDefaults().remote
	got, closeFn, err := d.connect(ctx, config.Defaults(), nil)
	if err != nil || got != local {
		t.Fatalf("fallback manager=%T err=%v", got, err)
	}
	closeFn()
	closeFn()
	if local.closed != 1 || !reflect.DeepEqual(*calls, []string{"local"}) {
		t.Fatalf("fallback cleanup=%d calls=%v", local.closed, *calls)
	}
	if h.shutdowns.Load() != 0 || provider.StopCalls() != 0 || launches != 0 {
		t.Fatalf("ordinary B disrupted A: Shutdown=%d stops=%d launches=%d", h.shutdowns.Load(), provider.StopCalls(), launches)
	}
	if after, err := modelhost.ReadRecord(dir); err != nil || after != rec {
		t.Fatalf("record changed: %+v %v", after, err)
	}
	if _, err := a.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"}); err != nil {
		t.Fatalf("A attachment unusable: %v", err)
	}
}
