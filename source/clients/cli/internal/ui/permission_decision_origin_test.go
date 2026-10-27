package ui

import (
	"context"
	"net"
	"path/filepath"
	"reflect"
	"testing"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type permissionOriginStub struct{ proto.UnimplementedAgentServer }

func (*permissionOriginStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
func (*permissionOriginStub) AllowToolCall(_ context.Context, r *proto.AllowToolCallRequest) (*proto.AllowToolCallResponse, error) {
	return &proto.AllowToolCallResponse{Ok: r.ToolUseId != "refused"}, nil
}
func (*permissionOriginStub) DenyToolCall(_ context.Context, r *proto.DenyToolCallRequest) (*proto.DenyToolCallResponse, error) {
	return &proto.DenyToolCallResponse{Ok: r.ToolUseId != "refused"}, nil
}
func permissionOriginClient(t *testing.T) *agentclient.Client {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	proto.RegisterAgentServer(s, &permissionOriginStub{})
	go s.Serve(l)
	t.Cleanup(func() { s.Stop(); l.Close() })
	c, err := agentclient.DialExisting(context.Background(), l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// Execute A's real RPC command, but leave its result queued until after replacing
// the model. No startup command is executed: only the in-memory RPC fixture runs.
func TestPermissionDecisionQueuedOrigin(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	dir := t.TempDir()
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(dir, "ui.yaml"))
	t.Setenv("CERCANO_THEMES_DIR", filepath.Join(dir, "themes"))
	aClient, bClient := permissionOriginClient(t), permissionOriginClient(t)
	for _, key := range []string{"y", "n", "a"} {
		for _, tool := range []string{"accepted", "refused"} {
			for _, sameClient := range []bool{false, true} {
				t.Run(key+"/"+tool+"/sameClient="+map[bool]string{false: "false", true: "true"}[sameClient], func(t *testing.T) {
					a := New(aClient, false).WithDevHost(DevHostSelection{Conversation: "same"}, "test")
					a.ensureTaskSubscription()
					defer a.taskSubscriptionCancel()
					a.pendingConfirm = toolConfirm(&pendingToolCall{ToolUseID: tool, Name: "mcp__fixture", Permission: "W"})
					a, cmd := a.resolveConfirmKey(key)
					if cmd == nil {
						t.Fatal("missing decision RPC")
					}
					saved, ok := cmd().(permissionDecisionMsg)
					if !ok || (saved.err != nil) != (tool == "refused") {
						t.Fatalf("unexpected RPC result: %+v", saved)
					}
					// Legitimate current replies retain their existing success/error rendering.
					count := len(a.mainChat().entries)
					current, _ := a.Update(saved)
					currentModel := current.(Model)
					if len(currentModel.mainChat().entries) != count+1 {
						t.Fatal("current reply not rendered")
					}
					client := bClient
					if sameClient {
						client = aClient
					}
					next, _ := a.Update(devHostResult{action: "reconnect", client: client})
					b := next.(Model)
					for i := 0; i < 3; i++ {
						before := append([]*Entry(nil), b.mainChat().entries...)
						after, _ := b.Update(saved)
						b = after.(Model)
						if !reflect.DeepEqual(before, b.mainChat().entries) {
							t.Fatal("queued A permission result rendered in replacement B")
						}
						// Repeat reconnects to catch generation resetting in New, even when the
						// callback returns the very same client object and conversation ID.
						if b.taskSubscriptionCancel != nil {
							b.taskSubscriptionCancel()
						}
						next, _ = b.Update(devHostResult{action: "reconnect", client: client})
						b = next.(Model)
						b.ensureTaskSubscription()
					}
					if b.taskSubscriptionCancel != nil {
						b.taskSubscriptionCancel()
					}
				})
			}
		}
	}
}

// Each dimension is necessary: a generation can coincide on distinct clients,
// and a client object can be reused across conversation/attachment lifetimes.
func TestPermissionDecisionOriginDimensions(t *testing.T) {
	for _, dimension := range []string{"client", "conversation", "generation"} {
		t.Run(dimension, func(t *testing.T) {
			m := minimalModel()
			m.agent = &agentclient.Client{}
			m.convID = "same"
			m.ensureTaskSubscription()
			defer m.taskSubscriptionCancel()
			saved := permissionDecisionMsg{client: m.agent, conversation: m.convID, generation: m.taskSubscriptionGeneration, allow: true}
			switch dimension {
			case "client":
				saved.client = &agentclient.Client{}
			case "conversation":
				saved.conversation = "other"
			case "generation":
				saved.generation++
			}
			before := len(m.mainChat().entries)
			next, _ := m.Update(saved)
			after := next.(Model)
			if len(after.mainChat().entries) != before {
				t.Fatal("stale " + dimension + " rendered")
			}
		})
	}
}
