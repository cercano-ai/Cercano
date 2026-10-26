package server

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestDisconnectedCompletionUsesWorkerMainAndPublishesReply(t *testing.T) {
	credentialtest.Enable(t)
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if !strings.Contains(string(body), "host:task-completions") || !strings.Contains(string(body), "child-one") {
			t.Errorf("worker did not receive completion metadata: %s", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Worker handled completed work\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer api.Close()
	cfg := config.Defaults()
	cfg.LocusMode = "cloud_only"
	cfg.ActiveCloudProfile = "normal"
	cfg.BackupCloudProfile = ""
	cfg.CloudProfiles = []config.CloudProfile{{Name: "normal", Flavor: "chat_completions", Model: "fixture", BaseURL: api.URL}}
	secret := secrets.NewMemory()
	if err := secret.Set("normal", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	workerServer := grpc.NewServer()
	proto.RegisterWorkerServer(workerServer, worker.NewWithSecretStore(secret))
	go workerServer.Serve(listener)
	defer workerServer.Stop()
	dial := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	s, _ := newServerWithStore(t)
	store := seedCompletion(t, s, "one")
	s.workerRunner = worker.NewWorkerRunnerWithDial(s.persistSvc, cfgsvc.New("", cfg, secret), permissions.New(nil, nil, nil), dial)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	// No CLI or initiating StreamProcessRequest is connected during execution.
	s.tryTaskCompletionWake(ctx, "owner")
	pending, err := store.PendingTasks(ctx, "owner")
	if err != nil || len(pending) != 0 || calls.Load() != 1 {
		t.Fatalf("calls=%d pending=%v error=%v", calls.Load(), pending, err)
	}
	client := taskObserverClient(t, s)
	updates, err := client.AttachConversation(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case update, ok := <-updates:
			if !ok || update.Err != nil {
				t.Fatalf("attachment ended: %v", update.Err)
			}
			if final := update.Response.GetFinalResponse(); final != nil {
				if final.Output != "Worker handled completed work" || !update.Response.BackgroundTurn {
					t.Fatalf("final=%v", update.Response)
				}
				cancel()
				for range updates {
				}
				return
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
