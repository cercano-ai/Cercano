package agentclient

import (
	"context"
	"testing"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type permissionResultServer struct {
	proto.UnimplementedAgentServer
	ok  bool
	err error
}

func (s permissionResultServer) AllowToolCall(context.Context, *proto.AllowToolCallRequest) (*proto.AllowToolCallResponse, error) {
	return &proto.AllowToolCallResponse{Ok: s.ok}, s.err
}
func (s permissionResultServer) DenyToolCall(context.Context, *proto.DenyToolCallRequest) (*proto.DenyToolCallResponse, error) {
	return &proto.DenyToolCallResponse{Ok: s.ok}, s.err
}
func TestPermissionDecisionRequiresAcceptedResult(t *testing.T) {
	for _, server := range []permissionResultServer{{ok: true}, {}, {err: status.Error(codes.FailedPrecondition, "refused")}} {
		c := cloudLoginClient(t, server)
		for _, call := range []func() error{
			func() error { return c.AllowToolCall(context.Background(), "c", "t") },
			func() error { return c.AllowToolCallPersist(context.Background(), "c", "t", true) },
			func() error { return c.DenyToolCall(context.Background(), "c", "t") },
			func() error { return c.DenyToolCallWithMessage(context.Background(), "c", "t", "redirect") },
		} {
			if err := call(); (err == nil) != server.ok {
				t.Fatalf("ok=%v err=%v", server.ok, err)
			}
		}
	}
}
