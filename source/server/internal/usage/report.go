package usage

import (
	"encoding/json"
	"errors"
	"time"

	"cercano/source/server/internal/llm"
)

const ReportSchemaV2 = "cercano.usage-report.v2"
const MeterSchemaV1 = "cercano.meters.v1"

type reportMeter struct {
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	Quantity *int64 `json:"quantity"`
}

type reportEvent struct {
	AttemptID                   string        `json:"attempt_id"`
	Revision                    uint64        `json:"revision"`
	OccurredAt                  time.Time     `json:"occurred_at"`
	ProfileVersion              int64         `json:"profile_version"`
	TaskType                    string        `json:"task_type,omitempty"`
	RouteID                     string        `json:"route_id"`
	SourceConnectionID          string        `json:"source_connection_id,omitempty"`
	RequestedRouteID            string        `json:"requested_route_id,omitempty"`
	RequestedSourceConnectionID string        `json:"requested_source_connection_id,omitempty"`
	Provider                    string        `json:"provider"`
	RequestedModelID            string        `json:"requested_model_id"`
	ActualProvider              string        `json:"actual_provider,omitempty"`
	ActualModelID               string        `json:"actual_model_id,omitempty"`
	Finality                    string        `json:"finality"`
	Meters                      []reportMeter `json:"meters"`
}

func countPtr(c llm.TokenCount) *int64 {
	if !c.Known {
		return nil
	}
	v := c.Value
	return &v
}

// uncachedInput converts the client's inclusive input total to the v1 meter
// schema's exclusive base input. An absent cache breakdown is not generally
// zero: providers whose accounting semantics cannot prove the base stay unknown.
func uncachedInput(a AttemptObservation) *int64 {
	u := a.Tokens
	if !u.Input.Known {
		return nil
	}
	base := u.Input.Value
	switch a.RequestedProvider {
	case "bedrock":
		if !u.CacheRead.Known && !u.CacheWrite.Known {
			return &base
		} // adapter documents no cache checkpoints
		if !u.CacheRead.Known || !u.CacheWrite.Known {
			return nil
		}
		if base < u.CacheRead.Value || base-u.CacheRead.Value < u.CacheWrite.Value {
			return nil
		}
		base -= u.CacheRead.Value + u.CacheWrite.Value
	case "anthropic":
		if !u.CacheRead.Known || !u.CacheWrite.Known {
			return nil
		}
		if base < u.CacheRead.Value || base-u.CacheRead.Value < u.CacheWrite.Value {
			return nil
		}
		base -= u.CacheRead.Value + u.CacheWrite.Value
	case "openai":
		if !u.CacheRead.Known {
			return nil
		} // SDK loses missing-vs-zero presence
		if base < u.CacheRead.Value || (u.CacheWrite.Known && base-u.CacheRead.Value < u.CacheWrite.Value) {
			return nil
		}
		base -= u.CacheRead.Value
		if u.CacheWrite.Known {
			base -= u.CacheWrite.Value
		}
	case "ollama":
		// Local Ollama reports uncached prompt tokens.
	default:
		return nil
	}
	if base < 0 {
		return nil
	}
	return &base
}

// ReportEventV2 produces one content-free immutable event from a physical
// attempt snapshot. The caller must persist it atomically with that snapshot.
// It does not infer billing source from provider, model, profile or route.
func ReportEventV2(a AttemptObservation) (json.RawMessage, bool, error) {
	if a.Report.ProfileVersion == 0 {
		return nil, false, nil
	}
	if a.Report.OrganizationID == "" || a.Report.HostID == "" || a.RouteID == "" || a.RequestedProvider == "" || a.RequestedModel == "" || a.StartedAt.IsZero() {
		return nil, false, errors.New("incomplete managed report identity")
	}
	finality := "provisional"
	if a.Tokens.Final {
		finality = "final"
	}
	e := reportEvent{AttemptID: a.ID, Revision: a.Revision, OccurredAt: a.StartedAt.UTC().Truncate(time.Microsecond), ProfileVersion: a.Report.ProfileVersion, TaskType: a.Report.TaskType,
		RouteID: a.RouteID, SourceConnectionID: a.SourceConnectionID, RequestedRouteID: a.Report.RequestedRouteID, RequestedSourceConnectionID: a.Report.RequestedSourceConnectionID,
		Provider: a.RequestedProvider, RequestedModelID: a.RequestedModel, Finality: finality,
		Meters: []reportMeter{{Name: "input_tokens", Unit: "token", Quantity: uncachedInput(a)}, {Name: "output_tokens", Unit: "token", Quantity: countPtr(a.Tokens.Output)}, {Name: "requests", Unit: "request", Quantity: func() *int64 { v := int64(1); return &v }()}},
	}
	// Adapter backend labels are not verified upstream providers. Preserve
	// the signed API provider for pricing and omit speculative actual-provider
	// metadata until the provider supplies a separate trusted serving route.
	if a.Model != a.RequestedModel {
		e.ActualModelID = a.Model
	}
	if a.Tokens.CacheRead.Known {
		e.Meters = append(e.Meters, reportMeter{Name: "cache_read_input_tokens", Unit: "token", Quantity: countPtr(a.Tokens.CacheRead)})
	}
	if a.Tokens.CacheWrite.Known {
		e.Meters = append(e.Meters, reportMeter{Name: "cache_write_input_tokens", Unit: "token", Quantity: countPtr(a.Tokens.CacheWrite)})
	}
	if a.Tokens.Reasoning.Known {
		e.Meters = append(e.Meters, reportMeter{Name: "reasoning_tokens", Unit: "token", Quantity: countPtr(a.Tokens.Reasoning)})
	}
	raw, err := json.Marshal(e)
	return raw, true, err
}
