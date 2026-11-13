//go:build darwin

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Only cooperative relinquish is stalled. Identity, authentication, native
// process attribution, TERM/KILL, writer inventory and recovery are production.
type nativeRecoverySource struct{ *server.Server }

func (s nativeRecoverySource) LifecycleForceRelinquish(ctx context.Context, _ *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	<-ctx.Done()
	return nil, status.FromContextError(ctx.Err()).Err()
}

func TestNativeRecoveryBridge(t *testing.T) {
	root := os.Getenv("CERCANO_NATIVE_RECOVERY_ROOT")
	role := os.Getenv("CERCANO_NATIVE_RECOVERY_ROLE")
	if root == "" || (role != "source" && role != "destination") {
		t.Skip("explicit disposable native recovery harness only")
	}
	if os.Getenv("CERCANO_DISABLE_CREDENTIALS") != "1" {
		t.Fatal("native recovery bridge requires disabled credentials")
	}
	ctx := context.Background()
	state := filepath.Join(root, "state")
	generation := filepath.Join(root, role)
	host := "native-recovery-" + role
	contract := conversation.DevelopmentCoexistenceDeclaration()
	identity := &pb.LifecycleIdentity{Host: host, Build: role, Store: filepath.Join(state, "conversations.db"), Contract: contract.Contract, Schema: int32(contract.SchemaVersion)}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(state, "hosts"), 0700))
	if role == "source" {
		must(conversation.PrepareDevelopmentStore(ctx, identity.Store))
	}
	base := filepath.Join(state, "hosts", fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
	token := base + ".token"
	must(devhostauth.Create(token))
	auth, err := devhostauth.Load(host, token)
	must(err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	selection := launchSelection{Identity: identity, Endpoint: listener.Addr().String(), TokenFile: token}
	must(launchExclusive(base+".starting", map[string]string{"host": host, "generation": generation}))
	must(os.WriteFile(base+".lock", nil, 0600))
	var srv *server.Server
	if role == "source" {
		var cap *conversation.Ownership
		srv, err = server.NewCoexistenceServer(ctx, server.CoexistenceOptions{StorePath: identity.Store, Host: host, Build: role, Contract: contract, ControlAuthorize: auth.Authorize, Reload: func(_ context.Context, _ conversation.Store, c *conversation.Ownership) error { cap = c; return nil }, CloseParticipation: func(context.Context) error { return nil }})
		must(err)
		_, err = srv.Retain(ctx, "native-history")
		must(err)
		must(srv.GenerationStore().Append(conversation.WithOwnership(ctx, cap), conversation.Turn{ConversationID: "native-history", Role: "assistant", Content: "committed native recovery history"}))
	} else {
		o, _ := developmentProductionFixture(t)
		o.State, o.Generation, o.Host, o.Identity = state, generation, host, identity
		// Synthetic backends and a private config; no model download or credentials.
		must(os.WriteFile(filepath.Join(state, "config.yaml"), []byte("worker_idle_timeout_seconds: -1\nopen_runtime: llama_server\nlocus_mode: open_only\n"), 0600))
		o.ConfigAuthority = filepath.Join(state, "config.yaml")
		binding := developmentBinding(o)
		o.startupAdmission = productionStartupAdmission{paths: productionAuthorityPaths{config: o.ConfigAuthority, conversations: identity.Store}, generationPreflight: true, bounded: &binding}
		s, resources, e := newProductionDevelopmentHostWithDependencies(ctx, o, auth, enabledDevelopmentFakes(t, developmentProductionDependencies{}))
		must(e)
		// enabledDevelopmentFakes permits construction only with injected memory
		// credentials and synthetic backends. Restore the harness policy before
		// exposing any RPCs; the production attestation must report that boundary.
		t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
		srv = s
		defer resources.Wait(ctx)
	}
	defer srv.CloseCoexistenceStore()
	policy, err := srv.GetCredentialPolicy(ctx, &pb.GetCredentialPolicyRequest{})
	must(err)
	if !policy.GetCredentialsDisabled() {
		t.Fatal("native recovery bridge must attest disabled credentials before publication")
	}
	transport := grpc.NewServer(auth.ServerOptions()...)
	if role == "source" {
		pb.RegisterAgentServer(transport, nativeRecoverySource{srv})
	} else {
		pb.RegisterAgentServer(transport, srv)
	}
	go transport.Serve(listener)
	defer transport.Stop()
	must(launchExclusive(base+".json", selection))
	must(launchExclusive(filepath.Join(root, role+".json"), selection))
	// Parent closes stdin only after UI assertions. Source is normally terminated
	// by the production adapter, never by this protocol.
	input, e := io.ReadAll(os.Stdin)
	must(e)
	if role == "destination" {
		row, e := conversation.ReadOwnership(ctx, srv.GenerationStore(), "native-history")
		must(e)
		if row.Owner != host || row.Epoch == "" || string(input) != row.Epoch {
			t.Fatalf("destination ownership: %+v; CLI epoch %q", row, input)
		}
	}
}
