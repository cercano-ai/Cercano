package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestSuggestPlan_WorkerLiveProfile(t *testing.T) {
	for _, scenario := range []string{"already", "external", "same_batch", "consecutive"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			active := agent.Profile{}
			if scenario == "already" {
				active = agent.PlanProfile()
			}
			requests, entries := 0, 0
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				call := requests
				if scenario == "external" && call == 1 {
					active = agent.PlanProfile()
				}
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				if call == 1 || (scenario == "consecutive" && call == 2) {
					tools := []any{}
					count := 1
					if scenario == "same_batch" {
						count = 2
					}
					for i := 0; i < count; i++ {
						tools = append(tools, map[string]any{"index": i, "id": fmt.Sprintf("entry-%d-%d", call, i), "type": "function", "function": map[string]any{"name": "suggest_plan", "arguments": `{"reason":"complex change"}`}})
					}
					event := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": tools}, "finish_reason": "tool_calls"}}}
					data, _ := json.Marshal(event)
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer api.Close()
			cfg := config.Defaults()
			cfg.LocusMode = "cloud_only"
			cfg.ActiveCloudProfile = "normal"
			cfg.BackupCloudProfile = ""
			cfg.CloudProfiles = []config.CloudProfile{
				{Name: "normal", TierOverrides: map[config.CostTier]string{config.CostEconomy: "normal-model", config.CostStandard: "normal-model", config.CostPremium: "normal-model"}, Flavor: "chat_completions", Model: "normal-model", BaseURL: api.URL},
				{Name: "deepinfra", Flavor: "chat_completions", Model: "saved-default", BaseURL: api.URL},
			}
			secret := secrets.NewMemory()
			_ = secret.Set("normal", "normal-key")
			_ = secret.Set("deepinfra", "deepinfra-key")
			listener := bufconn.Listen(1 << 20)
			server := grpc.NewServer()
			proto.RegisterWorkerServer(server, New())
			go server.Serve(listener)
			defer server.Stop()
			dial := func(ctx context.Context) (*grpc.ClientConn, error) {
				return grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
			}
			host := newWorkerRunnerWithDial(nil, cfgsvc.New("", cfg, secret), permissions.New(nil, nil, nil), dial)

			host.SetProfileReader(func(convID string) agent.Profile {
				if convID != "one" {
					t.Errorf("wrong read scope: %q", convID)
				}
				mu.Lock()
				defer mu.Unlock()
				return active
			})
			host.SetProfileHandler(func(_ context.Context, convID, name string) error {
				if convID != "one" || name != "plan" {
					return fmt.Errorf("wrong entry scope/name: %s %s", convID, name)
				}
				mu.Lock()
				defer mu.Unlock()
				entries++
				active = agent.PlanProfile()
				return nil
			})
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			prompts := 0
			_, err := host.RunTurn(ctx, runner.Request{ConversationID: "one", Input: "plan a complex change", WorkDir: t.TempDir()}, nil, func(context.Context, string, string, json.RawMessage, llm.Permission, bool) (bool, error) {
				prompts++
				return true, nil
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if scenario == "already" || scenario == "external" {
				want = 0
			}
			mu.Lock()
			gotEntries := entries
			mu.Unlock()
			if prompts != want || gotEntries != want {
				t.Fatalf("prompts=%d entries=%d want %d", prompts, gotEntries, want)
			}
		})
	}
}
