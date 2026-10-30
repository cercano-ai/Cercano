// Package v1 contains the implemented ACP v1 wire subset.
// Source: agentclientprotocol/agent-client-protocol schema/v1/schema.json
// pinned at d09e22d032e5d2f7d1dcf40bb43bd064be5dd3cd. No v2 types are used.
package v1

import "encoding/json"

const ProtocolVersion = 1

type InitializeRequest struct {
	ProtocolVersion *uint64 `json:"protocolVersion"`
}
type InitializeResponse struct {
	ProtocolVersion   uint64         `json:"protocolVersion"`
	AgentCapabilities map[string]any `json:"agentCapabilities"`
	AuthMethods       []any          `json:"authMethods"`
}
type NewSessionRequest struct {
	CWD                   string            `json:"cwd"`
	MCPServers            []json.RawMessage `json:"mcpServers"`
	AdditionalDirectories []string          `json:"additionalDirectories"`
}
type NewSessionResponse struct {
	SessionID string `json:"sessionId"`
}
type ContentBlock struct {
	Type string  `json:"type"`
	Text *string `json:"text,omitempty"`
	Name *string `json:"name,omitempty"`
	URI  *string `json:"uri,omitempty"`
}
type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}
type SessionRequest struct {
	SessionID string `json:"sessionId"`
}
type PromptResponse struct {
	StopReason string `json:"stopReason"`
}
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type ToolContent struct {
	Type    string      `json:"type"`
	Content TextContent `json:"content"`
}

// Update combines only the implemented content-chunk and tool-call variants.
type Update struct {
	SessionUpdate string `json:"sessionUpdate"`
	Content       any    `json:"content,omitempty"`
	ToolCallID    string `json:"toolCallId,omitempty"`
	Title         string `json:"title,omitempty"`
	Status        string `json:"status,omitempty"`
	RawInput      any    `json:"rawInput,omitempty"`
	RawOutput     any    `json:"rawOutput,omitempty"`
}
type SessionNotification struct {
	SessionID string `json:"sessionId"`
	Update    Update `json:"update"`
}
type ToolCallUpdate struct {
	ToolCallID string `json:"toolCallId"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
	RawInput   any    `json:"rawInput,omitempty"`
}
type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}
type RequestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCallUpdate     `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}
type RequestPermissionResponse struct {
	Outcome struct {
		Outcome  string `json:"outcome"`
		OptionID string `json:"optionId"`
	} `json:"outcome"`
}
