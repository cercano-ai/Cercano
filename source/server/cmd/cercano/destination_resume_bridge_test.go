package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// Test-owned process bridge: real lifecycle, auth, launch exclusion and SQLite;
// the inference backend is replaced by an explicit next-turn persistence probe.
// The only injected death verifier checks Wait's actual child exit, not a closed
// endpoint, lock availability or caller assertion. No ambient process inspection.
func TestDestinationResumeProcessBridge(t *testing.T) {
	manifest := os.Getenv("CERCANO_DESTINATION_RESUME_MANIFEST")
	if manifest == "" {
		t.Skip("requires explicit disposable CLI probe")
	}
	ctx := context.Background()
	if os.Getenv("CERCANO_DESTINATION_RESUME_SOURCE") == "1" {
		o, _ := developmentProductionFixture(t)
		var cap *conversation.Ownership
		s, err := server.NewCoexistenceServer(ctx, server.CoexistenceOptions{StorePath: o.Identity.Store, Host: o.Host, Build: o.Identity.Build, Contract: conversation.DevelopmentCoexistenceDeclaration(), Reload: func(_ context.Context, _ conversation.Store, c *conversation.Ownership) error { cap = c; return nil }, CloseParticipation: func(context.Context) error { return nil }})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Retain(ctx, "resume-probe"); err != nil {
			t.Fatal(err)
		}
		if err = s.GenerationStore().Append(conversation.WithOwnership(ctx, cap), conversation.Turn{ConversationID: "resume-probe", Role: "assistant", Content: "history before source exit"}); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(o.Identity)
		if err = os.WriteFile(manifest, data, 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // deliberately abandon owned row; OS releases locks, not ownership
	}
	root := t.TempDir()
	sourceManifest := filepath.Join(root, "source.json")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestDestinationResumeProcessBridge$", "-test.timeout=25s")
	child.Env = append(os.Environ(), "CERCANO_DESTINATION_RESUME_SOURCE=1", "CERCANO_DESTINATION_RESUME_MANIFEST="+sourceManifest)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	data, err := os.ReadFile(sourceManifest)
	if err != nil {
		t.Fatal(err)
	}
	var source proto.LifecycleIdentity
	if err = json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	destination := &proto.LifecycleIdentity{Host: "destination-after-exit", Build: source.Build, Contract: source.Contract, Schema: source.Schema, Store: source.Store}
	// Register an unavailable historical endpoint, with the exact launch intent
	// required by the production RecoveryGuard. No historical RPC is necessary.
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := dead.Addr().String()
	dead.Close()
	state := filepath.Dir(source.Store)
	hosts := filepath.Join(state, "hosts")
	if err = os.MkdirAll(hosts, 0700); err != nil {
		t.Fatal(err)
	}
	generation := "disposable-process-exit"
	base := filepath.Join(hosts, fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
	selection := launchSelection{Endpoint: unavailable, Identity: &source}
	data, _ = json.Marshal(selection)
	if err = os.WriteFile(base+".json", data, 0600); err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(map[string]string{"host": source.Host, "generation": generation})
	if err = os.WriteFile(base+".starting", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(base+".lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(root, "destination-token")
	if err = devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load(destination.Host, token)
	if err != nil {
		t.Fatal(err)
	}
	var cap *conversation.Ownership
	s, err := server.NewCoexistenceServer(ctx, server.CoexistenceOptions{StorePath: source.Store, Host: destination.Host, Build: source.Build, Contract: conversation.DevelopmentCoexistenceDeclaration(), ControlAuthorize: auth.Authorize, RecoveryGuard: developmentRecoveryGuardWithVerifier(state, destination, func(_ context.Context, host string) error {
		if host != source.Host || child.ProcessState == nil || !child.ProcessState.Exited() {
			return fmt.Errorf("exact source process not exited")
		}
		return nil
	}), Reload: func(_ context.Context, _ conversation.Store, c *conversation.Ownership) error { cap = c; return nil }, CloseParticipation: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.CloseCoexistenceStore()
	before, err := conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
	if err != nil || before.State != "owned" {
		t.Fatalf("abandoned row: %+v %v", before, err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer(auth.ServerOptions()...)
	proto.RegisterAgentServer(gs, s)
	go gs.Serve(lis)
	defer gs.Stop()
	data, _ = json.Marshal(launchSelection{Endpoint: lis.Addr().String(), TokenFile: token, Identity: destination})
	if err = os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	// CLI confirms through real authenticated lifecycle RPC and binds/reconnects
	// before requesting this synthetic next-turn write. No inference/replay.
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	if string(input) != "persist next turn\n" {
		t.Fatalf("missing CLI continuation: %q", input)
	}
	after, err := conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
	if err != nil || cap == nil || after.Owner != destination.Host || after.Epoch == before.Epoch {
		t.Fatalf("not freshly recovered: %+v %v", after, err)
	}
	if err = s.GenerationStore().Append(conversation.WithOwnership(ctx, cap), conversation.Turn{ConversationID: "resume-probe", Role: "user", Content: "next turn after CLI recovery"}); err != nil {
		t.Fatal(err)
	}
	turns, err := s.GenerationStore().GetTurns(ctx, "resume-probe")
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Content != "history before source exit" || turns[1].Content != "next turn after CLI recovery" {
		t.Fatalf("history/continuation: %+v", turns)
	}
	t.Logf("actual child exit; CLI recovered same conversation; fresh epoch %s; preserved history and persisted next turn", after.Epoch)
}
