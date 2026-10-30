package acp

import "context"

// Backend never changes daemon permission policy. Decisions apply to one tool
// call in one conversation, and only in response to a daemon permission event.
type Backend interface {
	NewSession(context.Context, string) (string, error)
	Prompt(context.Context, string, string, string) (<-chan Event, error)
	Permission(context.Context, string, string, bool) error
	Close() error
}
type Event struct {
	Kind     string
	Text     string
	ToolID   string
	ToolName string
	Input    string
	Failed   bool
	Err      error
}
