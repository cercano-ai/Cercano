package builtins

import (
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/conversation"
	"context"
	"encoding/json"
	"fmt"
)

type conversationHistoryCap struct{ retrieve bool }

func ConversationSearch() capabilities.Capability   { return conversationHistoryCap{} }
func ConversationRetrieve() capabilities.Capability { return conversationHistoryCap{retrieve: true} }
func (c conversationHistoryCap) Name() string {
	if c.retrieve {
		return "conversation_retrieve"
	}
	return "conversation_search"
}
func (conversationHistoryCap) Tier() capabilities.Tier { return capabilities.TierR }
func (conversationHistoryCap) Surfaces() capabilities.Surface {
	return capabilities.SurfaceAgent | capabilities.SurfaceMCP
}
func (c conversationHistoryCap) Description() string {
	return "Read persisted user/assistant text in this conversation only. Search is literal and case-sensitive; retrieval requires a turn_id returned by search. At most 50 entries, each 2048 bytes; truncated entries are marked. No system prompts, tool payloads, attachments or cross-conversation access. Use the last cursor as after to page search results."
}
func (c conversationHistoryCap) Schema() capabilities.Schema {
	if c.retrieve {
		return capabilities.Schema(`{"type":"object","properties":{"turn_id":{"type":"string","maxLength":256}},"required":["turn_id"],"additionalProperties":false}`)
	}
	return capabilities.Schema(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":512},"after":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":50}},"required":["query"],"additionalProperties":false}`)
}
func (c conversationHistoryCap) Execute(ctx context.Context, call *capabilities.Call) (*capabilities.Result, error) {
	q := conversation.HistoryQuery{Limit: 20}
	if c.retrieve {
		var a struct {
			TurnID string `json:"turn_id"`
		}
		if err := decodeReadQuery(call.Args, &a); err != nil {
			return nil, err
		}
		q.TurnID = a.TurnID
		q.Limit = 1
	} else {
		a := struct {
			Query string `json:"query"`
			After int64  `json:"after"`
			Limit int    `json:"limit"`
		}{Limit: 20}
		if err := decodeReadQuery(call.Args, &a); err != nil {
			return nil, err
		}
		q.Query = a.Query
		q.After = a.After
		q.Limit = a.Limit
	}
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if call.ConversationID == "" || call.Svc.History == nil {
		return nil, fmt.Errorf("conversation history unavailable for this execution context")
	}
	entries, err := call.Svc.History.ReadHistory(ctx, call.ConversationID, q)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	return &capabilities.Result{Text: string(b)}, nil
}
