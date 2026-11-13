package usage

import (
	"encoding/json"
	"testing"
	"time"

	"cercano/source/server/internal/llm"
)

func TestReportEventV2NormalizesInclusiveInputWithoutDoubleCharge(t *testing.T) {
	a := AttemptObservation{ID: "attempt", Revision: 2, StartedAt: time.Unix(100, 0), Provider: "anthropic", Model: "actual", RequestedProvider: "anthropic", RequestedModel: "requested", RouteID: "shared", SourceConnectionID: "subscription", Report: ReportIdentity{OrganizationID: "org", HostID: "host", ProfileVersion: 5, TaskType: "chat", RequestedRouteID: "initial"},
		Tokens: llm.TokenUsage{Final: true, Input: llm.ReportedTokens(100), CacheRead: llm.ReportedTokens(30), CacheWrite: llm.ReportedTokens(10), Output: llm.ReportedTokens(20), Reasoning: llm.ReportedTokens(5)}}
	raw, ok, err := ReportEventV2(a)
	if err != nil || !ok {
		t.Fatalf("event: %v %v", ok, err)
	}
	var event struct {
		SourceConnectionID string `json:"source_connection_id"`
		ActualModelID      string `json:"actual_model_id"`
		Finality           string `json:"finality"`
		Meters             []struct {
			Name     string `json:"name"`
			Quantity *int64 `json:"quantity"`
		} `json:"meters"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	if event.SourceConnectionID != "subscription" || event.ActualModelID != "actual" || event.Finality != "final" {
		t.Fatalf("metadata: %+v", event)
	}
	values := map[string]int64{}
	for _, m := range event.Meters {
		if m.Quantity != nil {
			values[m.Name] = *m.Quantity
		}
	}
	if values["input_tokens"] != 60 || values["cache_read_input_tokens"] != 30 || values["cache_write_input_tokens"] != 10 || values["output_tokens"] != 20 || values["reasoning_tokens"] != 5 || values["requests"] != 1 {
		t.Fatalf("meters: %v", values)
	}
	a.RequestedProvider = "openai"
	a.Tokens.CacheRead = llm.TokenCount{}
	raw, ok, err = ReportEventV2(a)
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	for _, m := range event.Meters {
		if m.Name == "input_tokens" && m.Quantity != nil {
			t.Fatal("unproven OpenAI uncached base was priced")
		}
	}
}

func TestReportEventV2DoesNotUploadPersonalOrBrokenIdentity(t *testing.T) {
	a := AttemptObservation{ID: "local", Revision: 1, StartedAt: time.Now(), RouteID: "local"}
	if raw, ok, err := ReportEventV2(a); raw != nil || ok || err != nil {
		t.Fatalf("personal report %+v %v %v", raw, ok, err)
	}
	a.Report.ProfileVersion = 2
	if _, _, err := ReportEventV2(a); err == nil {
		t.Fatal("incomplete managed identity accepted")
	}
}

func TestReportEventV2UsesSignedAPIProviderNotCustomLabel(t *testing.T) {
	a := AttemptObservation{ID: "custom", Revision: 1, StartedAt: time.Now().UTC(), Provider: "openrouter-custom", Model: "wire-model", RequestedProvider: "openai", RequestedModel: "signed-model", RouteID: "approved", SourceConnectionID: "source", Report: ReportIdentity{OrganizationID: "org", HostID: "host", ProfileVersion: 1}}
	raw, managed, err := ReportEventV2(a)
	if err != nil || !managed {
		t.Fatalf("conversion: %v %v", managed, err)
	}
	var e struct {
		Provider           string `json:"provider"`
		ActualProvider     string `json:"actual_provider"`
		RequestedModelID   string `json:"requested_model_id"`
		SourceConnectionID string `json:"source_connection_id"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	if e.Provider != "openai" || e.ActualProvider != "" || e.RequestedModelID != "signed-model" || e.SourceConnectionID != "source" {
		t.Fatalf("custom label contaminated pricing identity: %+v", e)
	}
}
