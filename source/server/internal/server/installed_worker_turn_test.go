package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// The copied current-code test executable supplies only the CLI's worker entry
// point. It uses the real worker server, handshake, snapshot and HTTP provider;
// no prepared authority, fake resolver, or direct RunTurn call is involved.
func init() {
	marker := os.Getenv("CERCANO_INSTALLED_TURN_FIXTURE")
	if marker == "" || len(os.Args) != 4 || os.Args[1] != "worker" || os.Args[2] != "--socket" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	identity, _ := json.Marshal(struct {
		Executable string
		PID        int
	}{exe, os.Getpid()})
	if err := os.WriteFile(marker, identity, 0600); err != nil {
		panic(err)
	}
	// Child transport is independently hermetic (parent transport is not inherited).
	http.DefaultTransport = installedTurnLoopbackTransport{&http.Transport{}}
	secret := secrets.NewMemory()
	if err := secret.Set("normal", "fixture-key"); err != nil {
		panic(err)
	}
	listener, err := net.Listen("unix", os.Args[3])
	if err != nil {
		panic(err)
	}
	srv := grpc.NewServer(grpc.MaxRecvMsgSize(worker.MaxMsgBytes), grpc.MaxSendMsgSize(worker.MaxMsgBytes))
	proto.RegisterWorkerServer(srv, worker.NewWithSecretStore(secret))
	if err := srv.Serve(listener); err != nil {
		panic(err)
	}
	os.Exit(0)
}

type installedTurnLoopbackTransport struct{ next http.RoundTripper }

func (r installedTurnLoopbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ip := net.ParseIP(req.URL.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("fixture blocked non-loopback HTTP: %s", req.URL.Host)
	}
	return r.next.RoundTrip(req)
}

func TestInstalledWorkerSuccessfulTurnAfterAliasSwitch(t *testing.T) {
	credentialtest.Enable(t)
	allocated, err := os.MkdirTemp("/tmp", "it-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(allocated) })
	root, err := filepath.EvalSymlinks(allocated)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", root) // bounded Darwin Unix socket paths and owned pidfiles
	marker := filepath.Join(root, "identity.json")
	t.Setenv("CERCANO_INSTALLED_TURN_FIXTURE", marker)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	makeKeg := func(version string) string {
		path := filepath.Join(root, "Cellar", "cercano-agent", version, "bin", "cercano")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, contents, 0700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	a, b := makeKeg("1.0"), makeKeg("2.0")
	alias := filepath.Join(root, "cercano")
	if err := os.Symlink(a, alias); err != nil {
		t.Fatal(err)
	}
	policy, selected, err := worker.SelectInstalledKegPolicy(alias)
	if err != nil || !selected {
		t.Fatalf("installed selection: %v, %v", selected, err)
	}
	if err := policy.ValidatePrepared(); err == nil {
		t.Fatal("installed policy granted prepared authority")
	}

	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/chat/completions" || !strings.Contains(string(body), "identify pinned worker") || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("unexpected provider request: path=%s body=%s", r.URL.Path, body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Pinned worker completed turn\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer api.Close()
	cfg := config.Defaults()
	cfg.ExecutionMode = "worker"
	cfg.WorkerIdleTimeoutSeconds = -1
	cfg.LocusMode = "cloud_only"
	cfg.ActiveCloudProfile = "normal"
	cfg.BackupCloudProfile = ""
	cfg.CloudProfiles = []config.CloudProfile{{Name: "normal", Flavor: "chat_completions", Model: "fixture", BaseURL: api.URL}}
	srv, store := newServerWithStore(t, filepath.Join(root, "conversations.db"))
	srv.SetConfigPersistence("", cfg)
	secret := secrets.NewMemory()
	if err := secret.Set("normal", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	if err := srv.SetSecrets(secret); err != nil {
		t.Fatal(err)
	}
	// Arm the ordinary front-door tool-loop gate, but fail if dispatch falls back
	// to this host provider instead of executing in the installed worker.
	srv.SetCloudLLMProvider(&scriptedProvider{})
	if err := srv.ConfigureInstalledWorker(policy); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.workerRunner.(interface{ CleanupOwnedWorkers() }).CleanupOwnedWorkers() })
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("worker started before alias switch: %v", err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, alias); err != nil {
		t.Fatal(err)
	}
	if target, err := filepath.EvalSymlinks(alias); err != nil || target != b {
		t.Fatalf("public alias = %s, %v", target, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	stream := &fakeStream{ctx: ctx}
	err = srv.StreamProcessRequest(&proto.ProcessRequestRequest{Input: "identify pinned worker", ConversationId: "installed-a", WorkDir: root}, stream)
	if err != nil {
		t.Fatalf("ordinary dispatch: %v", err)
	}
	final := ""
	for _, response := range stream.sent {
		if f := response.GetFinalResponse(); f != nil {
			final = f.Output
		}
	}
	if final != "Pinned worker completed turn" || calls.Load() != 1 {
		t.Fatalf("final=%q provider calls=%d", final, calls.Load())
	}
	identityBytes, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		Executable string
		PID        int
	}
	if err := json.Unmarshal(identityBytes, &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Executable != a || identity.PID <= 0 || identity.PID == os.Getpid() {
		t.Fatalf("not a fresh A worker: %+v", identity)
	}
	turns, err := store.GetTurns(ctx, "installed-a")
	if err != nil || len(turns) != 2 || turns[0].Role != "user" || turns[1].Role != "assistant" || turns[1].Content != final {
		t.Fatalf("persisted turns=%+v error=%v", turns, err)
	}
	t.Logf("ordinary dispatch completed: public alias=%s fresh worker=%s pid=%d calls=%d persisted turns=%d", b, identity.Executable, identity.PID, calls.Load(), len(turns))
}
