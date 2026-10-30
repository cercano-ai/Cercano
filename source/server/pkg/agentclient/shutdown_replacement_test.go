package agentclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type shutdownFakeClient struct {
	proto.AgentClient
	response *proto.ShutdownAgentResponse
	err      error
	calls    *int
}

func (f shutdownFakeClient) ShutdownAgent(context.Context, *proto.ShutdownAgentRequest, ...grpc.CallOption) (*proto.ShutdownAgentResponse, error) {
	if f.calls != nil {
		*f.calls++
	}
	return f.response, f.err
}

func TestShutdownReplacementGate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *proto.ShutdownAgentResponse
		err      error
		allowed  bool
	}{
		{name: "pending", response: &proto.ShutdownAgentResponse{Accepted: true}},
		{name: "legacy acceptance", response: &proto.ShutdownAgentResponse{Accepted: true, Message: "scheduled"}},
		{name: "rejected", response: &proto.ShutdownAgentResponse{Message: "secret"}},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "disconnect", err: errors.New("secret disconnected")},
		{name: "failed complete", response: &proto.ShutdownAgentResponse{Accepted: true, Complete: true, Phase: "failed", Message: "secret", IncompleteReason: "secret"}},
		{name: "unknown complete", response: &proto.ShutdownAgentResponse{Accepted: true, Complete: true, Phase: "future-secret"}},
		{name: "legacy complete", response: &proto.ShutdownAgentResponse{Accepted: true, Complete: true}},
		{name: "nil response"},
		{name: "complete with transport error", response: &proto.ShutdownAgentResponse{Accepted: true, Complete: true, Phase: "complete"}, err: errors.New("secret transport error")},
		{name: "unaccepted complete", response: &proto.ShutdownAgentResponse{Complete: true, Phase: "complete"}},
		{name: "incomplete phase complete", response: &proto.ShutdownAgentResponse{Accepted: true, Phase: "complete"}},
		{name: "complete", response: &proto.ShutdownAgentResponse{Accepted: true, Complete: true, Phase: "complete"}, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, wrapper := range []bool{false, true} {
				calls := 0
				c := &Client{agent: shutdownFakeClient{response: tc.response, err: tc.err, calls: &calls}}
				if wrapper {
					err := c.ShutdownAgent(context.Background(), "test")
					if (err == nil) != tc.allowed || (err != nil && strings.Contains(err.Error(), "secret")) {
						t.Fatalf("shutdown error: %v", err)
					}
				} else {
					_, _ = c.RequestShutdown(context.Background(), "test")
				}
				called := false
				_, _, err := c.launchReplacement(context.Background(), func() (string, bool, error) { called = true; return "", true, nil })
				if called != tc.allowed || (err == nil) != tc.allowed {
					t.Fatalf("wrapper=%v replacement called=%v err=%v", wrapper, called, err)
				}
				if calls != 1 {
					t.Fatalf("unexpected retries: %d", calls)
				}
			}
		})
	}
}
