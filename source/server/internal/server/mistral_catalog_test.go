package server

import (
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cercano/source/server/internal/modelevidence"
	"cercano/source/server/internal/modelmetadata"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

func TestMistralServerCatalogAndKeyLifecycle(t *testing.T) {
	credentialtest.Enable(t)
	s, _ := newTestServer()
	var chatCalls atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			if r.Header.Get("Authorization") != "Bearer work" {
				t.Error("tool roundtrip used wrong account")
			}
			var request struct {
				Model    string `json:"model"`
				Messages []struct {
					Role       string `json:"role"`
					ToolCallID string `json:"tool_call_id"`
					Content    string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Model != "common" {
				t.Errorf("wrong model %q", request.Model)
			}
			w.Header().Set("Content-Type", "application/json")
			if chatCalls.Add(1) == 1 {
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"abc123XYZ","type":"function","function":{"name":"Read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
			} else {
				last := request.Messages[len(request.Messages)-1]
				if last.Role != "tool" || last.ToolCallID != "abc123XYZ" || last.Content != "fixture" {
					t.Errorf("tool correlation=%+v", last)
				}
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
			}
			return
		}

		switch r.Header.Get("Authorization") {
		case "Bearer personal":
			fmt.Fprint(w, `{"data":[{"id":"common","name":"Common","max_context_length":4096,"capabilities":{"completion_chat":true,"function_calling":true,"vision":false}},{"id":"personal-only","capabilities":{"completion_chat":true,"function_calling":true}}]}`)
		case "Bearer work":
			fmt.Fprint(w, `{"data":[{"id":"common","max_context_length":8192,"capabilities":{"completion_chat":true,"function_calling":true,"vision":true}},{"id":"work-only","capabilities":{"completion_chat":true,"function_calling":true}},{"id":"not-agent","capabilities":{"completion_chat":true,"function_calling":false}}]}`)
		default:
			w.WriteHeader(401)
		}
	}))
	defer fixture.Close()
	cfg := config.Config{CloudProfiles: []config.CloudProfile{{Name: "personal", Provider: "mistral", Flavor: "chat_completions", BaseURL: fixture.URL + "/v1"}, {Name: "work", Provider: "mistral", Flavor: "chat_completions", BaseURL: fixture.URL + "/v1"}}}
	s.cfgSvc.Set(cfg)
	for _, name := range []string{"personal", "work"} {
		if err := s.cfgSvc.Secrets().Set(name, name); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	browse, err := s.ListRuntimeModels(ctx, &proto.ListRuntimeModelsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var common *proto.RuntimeModel
	for _, m := range browse.Models {
		if m.CatalogSource == "mistral" && m.CatalogId == "common" {
			common = m
		}
	}
	if common == nil || len(common.AccountObservations) != 2 || common.Metadata != nil || common.Acquisition != "serve" || common.DownloadUrl != "" {
		t.Fatalf("combined row=%v", common)
	}
	for _, name := range []string{"personal", "work"} {
		got, err := s.ListCloudProfileModels(ctx, &proto.ListCloudProfileModelsRequest{ProfileName: name})
		if err != nil || got.Error != "" || len(got.Models) != 2 {
			t.Fatalf("%s=%v %v", name, got, err)
		}
		for _, m := range got.Models {
			if len(m.AccountObservations) != 1 || m.AccountObservations[0].Profile != name || m.Metadata == nil || m.Metadata.PriceIn != nil {
				t.Fatalf("account leak or invented price: %v", m)
			}
		}
	}
	personalID := modelevidence.IdentityFor(cfg.CloudProfiles[0], "common")
	workID := modelevidence.IdentityFor(cfg.CloudProfiles[1], "common")
	personalEvidence := s.modelEvidence().Resolve(ctx, personalID)
	workEvidence := s.modelEvidence().Resolve(ctx, workID)
	if personalEvidence.ContextWindow != 4096 || personalEvidence.Vision != modelmetadata.VisionUnsupported || workEvidence.ContextWindow != 8192 || workEvidence.Vision != modelmetadata.VisionSupported {
		t.Fatalf("evidence crossed accounts: personal=%+v work=%+v", personalEvidence, workEvidence)
	}
	snapshot := s.modelEvidence().Snapshot(ctx, []modelmetadata.Identity{personalID, workID})
	if len(snapshot) != 2 {
		t.Fatalf("snapshot collapsed accounts: %+v", snapshot)
	}
	workProfile := cfg.CloudProfiles[1]
	workProfile.Model = "common"
	workKey, err := s.cfgSvc.Secrets().Get("work")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := cloudfactory.BuildCloudProvider(workProfile, workKey)
	if err != nil {
		t.Fatal(err)
	}
	call := inference.Call{Model: "common", Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: "read fixture"}}}}}
	first, err := provider.Chat(ctx, call)
	if err != nil || len(first.Blocks) != 1 || first.Blocks[0].ToolUseID != "abc123XYZ" {
		t.Fatalf("tool call: %+v %v", first, err)
	}
	call.Messages = append(call.Messages, llm.Message{Role: llm.RoleAssistant, Blocks: first.Blocks}, llm.Message{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockToolResult, ToolUseRef: "abc123XYZ", Content: "fixture"}}})
	second, err := provider.Chat(ctx, call)
	if err != nil || len(second.Blocks) != 1 || second.Blocks[0].Text != "done" || chatCalls.Load() != 2 {
		t.Fatalf("tool result: %+v %v", second, err)
	}
	change, err := s.SetCloudProfileKey(ctx, &proto.SetCloudProfileKeyRequest{Name: "personal", ApiKey: "bad"})
	if err != nil || !change.Ok {
		t.Fatalf("key change: %v %v", change, err)
	}
	if ev := s.modelEvidence().Resolve(ctx, personalID); ev.ContextWindow != 0 || ev.Vision != modelmetadata.VisionUnknown {
		t.Fatalf("evidence survived key rotation: %+v", ev)
	}
	if ev := s.modelEvidence().Resolve(ctx, workID); ev.ContextWindow != 8192 {
		t.Fatalf("rotation affected work evidence: %+v", ev)
	}
	got, _ := s.ListCloudProfileModels(ctx, &proto.ListCloudProfileModelsRequest{ProfileName: "personal"})
	if len(got.Models) != 0 || got.Error == "" {
		t.Fatalf("stale access after rotation: %v", got)
	}
	got, _ = s.ListCloudProfileModels(ctx, &proto.ListCloudProfileModelsRequest{ProfileName: "work"})
	if len(got.Models) != 2 || got.Error != "" {
		t.Fatalf("rotation damaged other account: %v", got)
	}
	removed, err := s.RemoveCloudProfile(ctx, &proto.RemoveCloudProfileRequest{Name: "work"})
	if err != nil || !removed.Ok {
		t.Fatalf("remove: %v %v", removed, err)
	}
	browse, err = s.ListRuntimeModels(ctx, &proto.ListRuntimeModelsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range browse.Models {
		if m.CatalogSource == "mistral" {
			t.Fatalf("retained inaccessible model: %v", m)
		}
	}
}
