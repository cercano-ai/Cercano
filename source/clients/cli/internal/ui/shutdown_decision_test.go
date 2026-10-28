package ui

import (
	"context"
	"net"
	"strings"
	"testing"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type shutdownUIFixture struct {
	permissionOriginStub
	outcome string
}

func (s *shutdownUIFixture) UpdateConfig(context.Context, *proto.UpdateConfigRequest) (*proto.UpdateConfigResponse, error) {
	if s.outcome == "config" {
		return nil, status.Error(codes.Internal, "secret")
	}
	return &proto.UpdateConfigResponse{Success: true}, nil
}
func (s *shutdownUIFixture) ShutdownAgent(context.Context, *proto.ShutdownAgentRequest) (*proto.ShutdownAgentResponse, error) {
	if s.outcome == "error" {
		return nil, status.Error(codes.Unavailable, "secret")
	}
	return &proto.ShutdownAgentResponse{Accepted: s.outcome != "rejected", Complete: s.outcome == "complete", Message: "secret", IncompleteReason: "secret"}, nil
}
func TestShutdownDecisionRPCAndOrigin(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	for _, outcome := range []string{"config", "error", "rejected", "pending", "complete"} {
		t.Run(outcome, func(t *testing.T) {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			proto.RegisterAgentServer(server, &shutdownUIFixture{outcome: outcome})
			go server.Serve(l)
			defer server.Stop()
			client, err := agentclient.DialExisting(context.Background(), l.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			for _, action := range []string{"restart", "runtime", "model"} {
				m := New(client, false)
				m.convID = "origin"
				m.ensureTaskSubscription()
				defer m.taskSubscriptionCancel()
				cmd := dispatchShutdown(m.shutdownOrigin(), "test", nil)
				if action == "runtime" {
					cmd = dispatchOpenRuntimeSwitch(m.shutdownOrigin(), "llama_server")
				}
				if action == "model" {
					cmd = dispatchOpenModelPick(m.shutdownOrigin(), "llama_server", "/fake/model")
				}
				result := cmd().(shutdownDecisionMsg)
				want := "incomplete"
				if outcome == "complete" {
					want = "shutdown complete"
				}
				if outcome == "config" && action != "restart" {
					want = "configuration update failed"
				}
				if !strings.Contains(result.text, want) || strings.Contains(result.text, "secret") {
					t.Fatalf("%s: %s", action, result.text)
				}
				before := len(m.mainChat().entries)
				updated, _ := m.Update(result)
				current := updated.(Model)
				if len(current.mainChat().entries) != before+1 {
					t.Fatal("current result missing")
				}
				for _, fence := range []string{"client", "conversation", "generation"} {
					stale := result
					switch fence {
					case "client":
						stale.client = nil
					case "conversation":
						stale.conversation = "other"
					case "generation":
						stale.generation++
					}
					updated, _ := current.Update(stale)
					next := updated.(Model)
					if len(next.mainChat().entries) != before+1 {
						t.Fatalf("stale %s applied", fence)
					}
				}
			}
		})
	}
}
