//go:build darwin && cgo

package worker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/brewrestart"
	wire "cercano/source/server/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Invoked only by the disposable brewrestart harness. The production spawn
// function and real worker executable are used, not a sleep/cat replacement.
func TestHostDeathSpawnProbe(t *testing.T) {
	root := os.Getenv("CERCANO_HOST_DEATH_PROBE")
	if root == "" {
		t.Skip("disposable subprocess helper")
	}
	p, err := NewSpawnPolicy(SpawnOptions{GenerationDir: filepath.Join(root, "generation"), ManifestPath: filepath.Join(root, "generation", "manifest.json"), HostInstance: "disposable-source", ResourceRoot: os.Getenv("CERCANO_HOST_DEATH_RESOURCES")})
	if err != nil {
		t.Fatal(err)
	}
	h, err := spawnWorkerWithPolicy(context.Background(), "idle", 1, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { h.Kill(); <-h.reapDone }()
	// Prove a real gRPC-only disconnect does not retire this worker and a new
	// connection can handshake on the same process before host death.
	for i := 0; i < 2; i++ {
		conn, err := grpc.NewClient("unix:"+h.socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		stream, err := wire.NewWorkerClient(conn).Accounting(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := stream.Send(&wire.WorkerAccountingReceipt{}); err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Recv(); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		cancel()
	}
	id, err := brewrestart.Inspect(h.cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		Identity brewrestart.Identity
		Socket   string
	}{id, h.socketPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "child.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	// A test-owned pipe keeps the spawning host idle. The parent kills this
	// process only after obtaining the immutable child identity and socket.
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}
