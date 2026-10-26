package agentclient

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type authenticatedPinnedFake struct{ *pinnedFake }

func (*authenticatedPinnedFake) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}

func TestAuthenticatedPinnedReconnect(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	forbidLaunch(t)
	path := filepath.Join(t.TempDir(), "token")
	if err := devhostauth.Create(path); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("A", path)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(auth.ServerOptions()...)
	defer srv.Stop()
	f := &pinnedFake{host: "A"}
	f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED))
	proto.RegisterAgentServer(srv, &authenticatedPinnedFake{f})
	go srv.Serve(lis)
	pin := pinAt(lis.Addr().String())
	wrong := pin
	wrong.HostInstance = "B"
	if _, err := DialAuthenticatedPinnedSession(context.Background(), wrong, path); err == nil {
		t.Fatal("accepted wrong host")
	}
	if f.identities.Load() != 0 || f.checks.Load() != 0 {
		t.Fatal("wrong host reached handler")
	}
	c, err := DialAuthenticatedPinnedSession(context.Background(), pin, path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.ReconnectPinnedSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.identities.Load() != 2 || f.checks.Load() != 2 {
		t.Fatal("fresh authenticated validation missing")
	}
	// Lifecycle methods have the same token protection, not merely session RPCs.
	if _, err := c.agent.ResetSetup(context.Background(), &proto.ResetSetupRequest{}); err != nil {
		t.Fatal(err)
	}
	f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_RELEASED))
	var refusal *SessionCheckError
	if err := c.ReconnectPinnedSession(context.Background()); !errors.As(err, &refusal) {
		t.Fatalf("release: %v", err)
	}
	// Raw client remains unauthorized before a protected lifecycle effect.
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = proto.NewAgentClient(conn).ResetSetup(context.Background(), &proto.ResetSetupRequest{})
	if status.Code(err) != codes.Unauthenticated || f.requests.Load() != 1 {
		t.Fatalf("unauthenticated lifecycle: %v effects=%d", err, f.requests.Load())
	}
}
