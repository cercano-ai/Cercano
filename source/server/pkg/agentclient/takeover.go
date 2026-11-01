package agentclient

import (
	"context"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const TerminalTakeoverMetadata = "cercano-takeover-terminal"
const TerminalConflictReason = "CONVERSATION_CLIENT_ATTACHED"

// TakeoverTarget is the exact attachment presented by an authenticated host.
// The server compares it atomically on confirmation; this is not a force flag.
func TakeoverTarget(err error) string {
	if status.Code(err) != codes.AlreadyExists {
		return ""
	}
	for _, detail := range status.Convert(err).Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok && info.Reason == TerminalConflictReason {
			return info.Metadata["terminal"]
		}
	}
	return ""
}

// TakeoverConversation must only be called after explicit user confirmation.
// The expected attachment is sent only on the first enrollment, never retries.
func (c *Client) TakeoverConversation(ctx context.Context, parent, expected string) (<-chan ConversationUpdate, error) {
	if expected == "" {
		return nil, status.Error(codes.InvalidArgument, "missing takeover target")
	}
	return c.attachConversation(ctx, parent, metadata.Pairs(TerminalTakeoverMetadata, expected))
}
