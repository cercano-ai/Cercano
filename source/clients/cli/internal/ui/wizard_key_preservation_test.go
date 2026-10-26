package ui

import (
	"context"
	"net"
	"strings"
	"testing"

	"cercano/source/clients/cli/internal/wizard"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type wizardKeyCollisionServer struct {
	proto.UnimplementedAgentServer
	writes int
}

func (s *wizardKeyCollisionServer) GetCloudProfiles(context.Context, *proto.GetCloudProfilesRequest) (*proto.GetCloudProfilesResponse, error) {
	return &proto.GetCloudProfilesResponse{Active: "anthropic", Profiles: []*proto.CloudProfileInfo{{Name: "anthropic"}}}, nil
}
func (s *wizardKeyCollisionServer) UpsertCloudProfile(_ context.Context, req *proto.UpsertCloudProfileRequest) (*proto.UpsertCloudProfileResponse, error) {
	if req.GetCreateOnly() {
		return &proto.UpsertCloudProfileResponse{Ok: false, Error: "account already exists; reuse it or edit it in /config"}, nil
	}
	s.writes++
	return &proto.UpsertCloudProfileResponse{Ok: true}, nil
}
func TestWizardNewKeyCannotOverwriteExistingAccount(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := &wizardKeyCollisionServer{}
	proto.RegisterAgentServer(server, stub)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()
	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	wp := newTestWizardPage(t)
	wp.agent = client
	wp.state = wizard.State{CloudProvider: "anthropic"}
	err = wp.commitAPIKey("fake-test-key")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected preservation error, got %v", err)
	}
	if stub.writes != 0 {
		t.Fatalf("mutated an existing account before refusing: %d writes", stub.writes)
	}
}

func TestWizardSubscriptionLoginIsCreateOnly(t *testing.T) {
	for _, auth := range []string{"claude", "chatgpt"} {
		t.Run(auth, func(t *testing.T) {
			wp := newTestWizardPage(t)
			wp.state.Step = wizard.StepCloud
			wp.state.LocusMode = "cloud_only"
			wp.authPick = true
			wp.state.CloudProvider = "anthropic"
			if auth == "chatgpt" {
				wp.state.CloudProvider = "openai"
			}
			found := false
			for i, row := range wp.rows() {
				if row.Key == auth {
					wp.cursor = i
					found = true
					break
				}
			}
			if !found {
				t.Fatal("auth row missing")
			}
			cmd, _ := wp.selectRow()
			if cmd == nil {
				t.Fatal("no login command")
			}
			switch msg := cmd().(type) {
			case openClaudeLoginModalMsg:
				if !msg.createOnly {
					t.Fatal("Claude sign-in may replace credentials")
				}
			case openChatGPTLoginModalMsg:
				if !msg.createOnly {
					t.Fatal("ChatGPT sign-in may replace credentials")
				}
			default:
				t.Fatalf("unexpected login message %T", msg)
			}
		})
	}
}

// wizardKeyCollisionServer uses only in-memory RPC fixtures, never host credentials.
func (*wizardKeyCollisionServer) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
