package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"

	"cercano/source/server/internal/catalog"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	runtimessvc "cercano/source/server/internal/hostsvc/runtimes"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/localruntimetest"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

// TestDownloadEnrollAssertionUsesInterface: a catalog download must be
// enrolled through localruntime.DownloadEnroller, so it also works when the
// runtime manager is a modelhost.Remote rather than the in-process manager.
func TestDownloadEnrollAssertionUsesInterface(t *testing.T) {
	payload := []byte("GGUF-test-bytes")
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(payload)
	}))
	defer files.Close()

	// The model host: an in-process manager behind a real Server on loopback.
	hostMgr := localruntime.NewManager()
	hostMgr.RegisterProvider(localruntimetest.NewFakeProvider("llama_server"))
	hostSrv := modelhost.NewServer(hostMgr, modelhost.ServerOptions{Token: "tok-enroll", Version: "test"})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	hostSrv.Register(g)
	go g.Serve(lis)
	defer g.Stop()
	runDir := t.TempDir()
	if err := modelhost.WriteRecord(runDir, modelhost.Record{PID: os.Getpid(), Addr: lis.Addr().String(), Token: "tok-enroll", APIVersion: modelhost.APIVersion}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remote, err := modelhost.NewRemote(ctx, modelhost.RemoteOptions{Ensure: modelhost.EnsureOptions{RunDir: runDir, Timeout: 5 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()

	modelDir := t.TempDir()
	cfg := config.Config{}
	cfg.LlamaServer.ModelDirs = []string{modelDir}
	cs := cfgsvc.New("", cfg, nil)
	rt := runtimessvc.New(cs, nil)
	rt.SetRuntimeManager(remote)
	reg := catalog.NewRegistry()
	reg.Register(&fakeCatalogBackend{
		arch:    "qwen2",
		files:   []catalog.Variant{{Name: "x-Q4_K_M.gguf"}},
		urls:    []string{files.URL + "/x-Q4_K_M.gguf"},
		primary: "x-Q4_K_M.gguf",
		total:   int64(len(payload)),
	})
	s := &Server{cfgSvc: cs, runtimesSvc: rt, catalogRegistry: reg}

	resp, err := s.DownloadRuntimeModel(ctx, &proto.DownloadRuntimeModelRequest{
		Runtime:       "llama_server",
		ModelId:       "cat-model",
		CatalogId:     "unsloth/x-GGUF",
		CatalogSource: "fake",
	})
	if err != nil {
		t.Fatalf("DownloadRuntimeModel: %v", err)
	}
	if !resp.GetOk() {
		t.Fatalf("catalog download through Remote was not enrolled: %s", resp.GetError())
	}
	// Let the host's download finish before the temp dirs go away.
	deadline := time.Now().Add(10 * time.Second)
	for {
		inv, _ := hostMgr.Inventory(ctx)
		done := false
		for _, m := range inv {
			if m.ID == "cat-model" && m.DownloadState != localruntime.Downloading {
				done = true
			}
		}
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("host download did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
