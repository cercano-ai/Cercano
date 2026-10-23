package setupresetcmd

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

func TestCredentialIsolationResetNoFallback(t *testing.T) {
	t.Setenv(secrets.DisableEnv, "1")
	var mutations atomic.Int32
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod != proto.Agent_GetAgentHealth_FullMethodName {
			mutations.Add(1)
		}
		return next(ctx, req)
	}))
	proto.RegisterAgentServer(s, &proto.UnimplementedAgentServer{})
	go s.Serve(l)
	defer s.Stop()
	_, err = TryExistingAgent(context.Background(), l.Addr().String())
	if !errors.Is(err, agentclient.ErrCredentialIsolation) || errors.Is(err, ErrNoAgent) {
		t.Fatalf("wrong refusal: %v", err)
	}
	root := t.TempDir()
	config := filepath.Join(root, "config.yaml")
	_, port, _ := net.SplitHostPort(l.Addr().String())
	original := []byte("port: '" + port + "'\n")
	if err := os.WriteFile(config, original, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = Perform(context.Background(), Dependencies{ConfigPath: config, WizardPath: filepath.Join(root, "wizard.yaml"), TryAgent: TryExistingAgent, OpenStore: func() (secrets.Store, error) { t.Fatal("local credential fallback"); return nil, nil }})
	if !errors.Is(err, agentclient.ErrCredentialIsolation) {
		t.Fatal(err)
	}
	if mutations.Load() != 0 {
		t.Fatal("sent ResetSetup or other mutation RPC")
	}
	got, _ := os.ReadFile(config)
	if string(got) != string(original) {
		t.Fatal("config mutated")
	}
}
