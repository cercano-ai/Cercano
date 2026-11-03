package ui

import (
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type deadOwnerFixture struct {
	pb.UnimplementedAgentServer
	identity *pb.LifecycleIdentity
	confirm  atomic.Int64
	inspect  atomic.Int64
}

func (s *deadOwnerFixture) LifecycleResume(_ context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	s.confirm.Add(1)
	return &pb.LifecycleControlResponse{Identity: s.identity, Owner: &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: s.identity.Host, Epoch: "recovered"}, Ready: true}, nil
}
func (s *deadOwnerFixture) LifecycleAcquire(context.Context, *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	return nil, status.Error(codes.FailedPrecondition, "owned by exited A")
}
func (s *deadOwnerFixture) LifecycleRecover(_ context.Context, r *pb.LifecycleRecoverRequest) (*pb.LifecycleControlResponse, error) {
	owner := &pb.LifecycleOwner{Conversation: r.Conversation, Host: "A", Epoch: "old"}
	if r.Confirm {
		if r.GetExpectedOwner().GetHost() != "A" || r.GetExpectedOwner().GetEpoch() != "old" {
			return nil, status.Error(codes.FailedPrecondition, "stale")
		}
		s.confirm.Add(1)
		owner.Host = s.identity.Host
		owner.Epoch = "recovered"
	} else {
		s.inspect.Add(1)
	}
	return &pb.LifecycleControlResponse{Identity: s.identity, Owner: owner, Ready: r.Confirm}, nil
}
func (s *deadOwnerFixture) GetDevelopmentHostIdentity(context.Context, *pb.GetDevelopmentHostIdentityRequest) (*pb.GetDevelopmentHostIdentityResponse, error) {
	return &pb.GetDevelopmentHostIdentityResponse{HostInstance: s.identity.Host, Build: s.identity.Build, Contract: s.identity.Contract, SchemaVersion: s.identity.Schema}, nil
}
func (s *deadOwnerFixture) CheckRetainedSession(context.Context, *pb.CheckRetainedSessionRequest) (*pb.CheckRetainedSessionResponse, error) {
	return &pb.CheckRetainedSessionResponse{State: pb.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED}, nil
}
func (s *deadOwnerFixture) GetCredentialPolicy(context.Context, *pb.GetCredentialPolicyRequest) (*pb.GetCredentialPolicyResponse, error) {
	return &pb.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
func TestDeadOwnerResumeAutomaticallyRecovers(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
	token := filepath.Join(root, "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("B", token)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer(auth.ServerOptions()...)
	defer g.Stop()
	id := &pb.LifecycleIdentity{Host: "B", Build: "fixture", Contract: "fixture", Schema: 2, Store: filepath.Join(root, "conversations.db")}
	fixture := &deadOwnerFixture{identity: id}
	pb.RegisterAgentServer(g, fixture)
	go g.Serve(lis)
	dest := DevHostSelection{Identity: id, Endpoint: lis.Addr().String(), TokenFile: token, Conversation: "current", Epoch: "current-epoch"}
	selected, err := reopenDevHost(context.Background(), dest, "history")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Identity.Host != "B" || selected.Epoch != "recovered" || fixture.confirm.Load() != 1 {
		t.Fatalf("recovery=%v confirmations=%d", selected, fixture.confirm.Load())
	}
}
