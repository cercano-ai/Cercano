package ui

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type sessionTierRPC struct {
	proto.UnimplementedAgentServer
	calls chan *proto.SessionModelRequest
}

func (s *sessionTierRPC) SessionModel(_ context.Context, req *proto.SessionModelRequest) (*proto.SessionModelResponse, error) {
	s.calls <- req
	state := tierPickerModel().sessionModelState
	if req.Action == "tier" {
		route := agentclient.SessionModelRoute{Profile: req.Profile, Model: "gpt-6-luna"}
		state.Effective = &route
		state.Override = &route
	}
	data, _ := json.Marshal(state)
	return &proto.SessionModelResponse{ResultJson: data}, nil
}

func TestSessionTierPickerRPC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := grpc.NewServer()
	stub := &sessionTierRPC{calls: make(chan *proto.SessionModelRequest, 4)}
	proto.RegisterAgentServer(server, stub)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	m := tierPickerModel()
	m.agent = client
	m.sessionTierPicker = nil
	msg := m.openSessionTierPicker()().(sessionModelMsg)
	next, _ := m.Update(msg)
	m = next.(Model)
	if req := <-stub.calls; req.Action != "status" || req.ConversationId != m.convID {
		t.Fatal("bad status request", req)
	}
	m.sessionTierPicker.cursor = 2
	cmd := m.selectSessionTier()
	if cmd == nil {
		t.Fatal("missing select command")
	}
	msg = cmd().(sessionModelMsg)
	if req := <-stub.calls; req.Action != "tier" || req.Model != "economy" || req.Profile != "openai" || req.ConversationId != m.convID {
		t.Fatal("wrong tier RPC", req)
	}
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.sessionModelState.Override.Model != "gpt-6-luna" || m.activeSessionTierPicker() {
		t.Fatal("acknowledgement not applied")
	}
}

// sessionTierRPC uses only in-memory RPC fixtures, never host credentials.
func (*sessionTierRPC) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
