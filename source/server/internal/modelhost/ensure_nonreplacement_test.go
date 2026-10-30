package modelhost

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// First probe is unavailable, then an older host is visible under launch.lock.
type lateOlderHost struct {
	stubbornHost
	hellos atomic.Int32
}

func (h *lateOlderHost) Hello(ctx context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	if h.hellos.Add(1) == 1 {
		return nil, status.Error(codes.Unavailable, "not ready")
	}
	return h.stubbornHost.Hello(ctx, req)
}
func TestEnsureRefuseOlderHostAtBothProbes(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial", true: "under-lock"}[late], func(t *testing.T) {
			dir := t.TempDir()
			host := &lateOlderHost{}
			if !late {
				host.hellos.Store(1)
			}
			rec := Record{PID: 4243, Addr: serve(t, host), Token: "tok", APIVersion: APIVersion - 1}
			if err := WriteRecord(dir, rec); err != nil {
				t.Fatal(err)
			}
			launches := 0
			opts := testOpts(dir, func(context.Context) error { launches++; return nil })
			opts.RefuseOlderHost = true
			if _, err := Ensure(context.Background(), opts); !errors.Is(err, ErrOlderModelHost) {
				t.Fatalf("err=%v", err)
			}
			if launches != 0 || host.shutdown.Load() != 0 {
				t.Fatalf("launches=%d Shutdown=%d", launches, host.shutdown.Load())
			}
			if got, err := ReadRecord(dir); err != nil || got != rec {
				t.Fatalf("record changed: %+v %v", got, err)
			}
		})
	}
}
func TestEnsureRefuseOlderHostStillLaunchesAbsentHost(t *testing.T) {
	dir := t.TempDir()
	launches := 0
	var want Record
	opts := testOpts(dir, func(context.Context) error { launches++; want = startHost(t); return WriteRecord(dir, want) })
	opts.RefuseOlderHost = true
	got, err := Ensure(context.Background(), opts)
	if err != nil || got != want || launches != 1 {
		t.Fatalf("got=%+v launches=%d err=%v", got, launches, err)
	}
}
