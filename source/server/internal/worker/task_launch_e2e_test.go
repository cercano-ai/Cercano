package worker

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/credentialtest"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/dispatch"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/taskrun"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type launchLinkSink struct{ events []runner.Event }

func (s *launchLinkSink) Emit(e runner.Event) { s.events = append(s.events, e) }

func TestPublicWorkerDispatchReturnsBeforeChildFinishes(t *testing.T) {
	credentialtest.Enable(t)
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			event := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "launch", "type": "function", "function": map[string]any{"name": "dispatch", "arguments": `{"task":"background work","tools":["Edit"]}`}}}}, "finish_reason": "tool_calls"}}}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"launched\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer api.Close()
	cfg := config.Defaults()
	cfg.LocusMode = "cloud_only"
	cfg.ActiveCloudProfile = "normal"
	cfg.BackupCloudProfile = ""
	cfg.CloudProfiles = []config.CloudProfile{{Name: "normal", Flavor: "chat_completions", Model: "fixture", BaseURL: api.URL}}
	secret := secrets.NewMemory()
	_ = secret.Set("normal", "fixture-key")
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	proto.RegisterWorkerServer(server, NewWithSecretStore(secret))
	go server.Serve(listener)
	defer server.Stop()
	dial := func(ctx context.Context) (*grpc.ClientConn, error) {
		return grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	host := newWorkerRunnerWithDial(nil, cfgsvc.New("", cfg, secret), permissions.New(nil, nil, nil), dial)
	host.SetProfileReader(func(string) agent.Profile { return agent.Profile{Name: "developer"} })
	store, err := conversation.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	release := make(chan struct{})
	manager, err := taskrun.New(store.(conversation.TaskStore), func(ctx context.Context, spec dispatch.Spec) (dispatch.Result, error) {
		if spec.ConversationID != "owner" || len(spec.Tools) != 1 || spec.Tools[0] != "Edit" {
			t.Errorf("launch spec: %+v", spec)
		}
		select {
		case <-release:
			return dispatch.Result{Text: "child complete", GrantedTools: spec.Tools, Model: "child-model", Provider: "fixture"}, nil
		case <-ctx.Done():
			return dispatch.Result{}, ctx.Err()
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := manager.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	host.SetTasks(manager)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	confirmations := 0
	requester := func(_ context.Context, _ string, name string, _ json.RawMessage, tier llm.Permission, _ bool) (bool, error) {
		confirmations++
		if name != "dispatch" || tier != llm.PermX {
			t.Errorf("wrong approval %s %s", name, tier)
		}
		return true, nil
	}
	sink := &launchLinkSink{}
	if _, err := host.RunTurn(ctx, runner.Request{ConversationID: "owner", Input: "launch work", WorkDir: t.TempDir()}, sink, requester, nil); err != nil {
		t.Fatal(err)
	}
	if confirmations != 1 {
		t.Fatalf("confirmations=%d", confirmations)
	}
	// Evict the originating worker endpoint before allowing the child to finish.
	// Task ownership must not depend on that worker's connection or service.
	server.Stop()
	records, err := manager.List(context.Background(), "owner", 20)
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%+v err=%v", records, err)
	}
	links := 0
	for _, ev := range sink.events {
		if ev.Kind == runner.EventSubAgent && ev.SubAgentKind == "started" {
			links++
			if ev.ToolUseID != "launch" || ev.SubAgentID != records[0].ChildID || ev.SubAgentParentID != "owner" {
				t.Fatalf("wrong launch link: %+v", ev)
			}
		}
	}
	if links != 1 {
		t.Fatalf("launch links=%d", links)
	}
	if records[0].Terminal() {
		t.Fatalf("child terminated with parent turn: %+v", records[0])
	}
	close(release)
	record, err := manager.Wait(ctx, "owner", records[0].ID)
	if err != nil || record.Status != "completed" {
		t.Fatalf("completion %+v %v", record, err)
	}
	var result dispatch.Result
	if err := json.Unmarshal(record.ResultJSON, &result); err != nil {
		t.Fatal(err)
	}
	if result.Text != "child complete" || result.Model != "child-model" || len(result.GrantedTools) != 1 {
		t.Fatalf("missing deferred evidence: %+v", result)
	}
}
