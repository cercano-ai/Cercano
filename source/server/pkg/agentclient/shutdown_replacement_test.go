package agentclient

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type shutdownFakeClient struct {
	proto.AgentClient
	response *proto.ShutdownAgentResponse
	err      error
}

func (f shutdownFakeClient) ShutdownAgent(context.Context, *proto.ShutdownAgentRequest, ...grpc.CallOption) (*proto.ShutdownAgentResponse, error) {
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
		{name: "rejected", response: &proto.ShutdownAgentResponse{}},
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "disconnect", err: errors.New("disconnected")},
		{name: "complete", response: &proto.ShutdownAgentResponse{Accepted: true, Complete: true}, allowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{agent: shutdownFakeClient{response: tc.response, err: tc.err}}
			err := c.ShutdownAgent(context.Background(), "test")
			if (err == nil) != tc.allowed {
				t.Fatalf("shutdown error: %v", err)
			}
			called := false
			_, _, err = c.launchReplacement(context.Background(), func() (string, bool, error) { called = true; return "", true, nil })
			if called != tc.allowed || (err == nil) != tc.allowed {
				t.Fatalf("replacement called=%v err=%v", called, err)
			}
		})
	}
}
