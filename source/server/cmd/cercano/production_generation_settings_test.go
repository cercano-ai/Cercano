package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
)

// No process, inference, native credentials or dialing dependencies: the same
// audited fake graph used by the existing production-composition tests.
func TestProductionGenerationSettingsConvergence(t *testing.T) {
	o, auth := developmentProductionFixture(t)
	path := filepath.Join(o.State, "config.yaml")
	permissions := filepath.Join(o.State, "permissions.yaml")
	if err := os.WriteFile(permissions, []byte("mode: strict\n"), 0600); err != nil {
		t.Fatal(err)
	}
	srv, cleanup, err := newProductionDevelopmentHostWithDependencies(context.Background(), o, auth, enabledDevelopmentFakes(t, developmentProductionDependencies{}))
	if cleanup != nil {
		defer cleanup.Wait(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	owner, err := srv.Retain(context.Background(), "settings")
	if err != nil {
		t.Fatal(err)
	}
	bound := metadata.NewIncomingContext(context.Background(), metadata.Pairs("cercano-host-instance", owner.Host, "cercano-conversation-id", owner.Conversation, "cercano-ownership-epoch", owner.Epoch, "cercano-terminal-attachment", "settings-terminal"))
	patch := &proto.UpdateConfigRequest{ToolLoopMaxIterations: "17", ElideToolResults: "true"}
	before, _ := os.ReadFile(path)
	if _, err := srv.UpdateConfig(context.Background(), patch); err == nil {
		t.Fatal("unbound mutation admitted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("refused mutation published")
	}
	if reply, err := srv.UpdateConfig(bound, patch); err != nil || !reply.GetSuccess() {
		t.Fatalf("save/apply: %v %v", reply, err)
	}
	disk, err := config.Load(path)
	if err != nil || disk.ToolLoop.MaxIterations != 17 || srv.ConfigSnapshot().ToolLoop.MaxIterations != 17 {
		t.Fatalf("save/apply: %v %v", disk.ToolLoop, err)
	}
	if _, err := srv.UpdateConfig(bound, &proto.UpdateConfigRequest{OpenRuntime: "ollama"}); err == nil {
		t.Fatal("runtime gate removed")
	}
	response, err := srv.SetPermissionMode(bound, &proto.SetPermissionModeRequest{Mode: "permissive"})
	if err != nil || !response.GetOk() {
		t.Fatalf("permission mutation: %v %v", response, err)
	}
	peer, err := agent.LoadPermissionStore(permissions)
	if err != nil || peer.Mode() != agent.ModePermissive {
		t.Fatalf("permission publication: %v", err)
	}
	if err := peer.SetMode(agent.ModeStrict); err != nil {
		t.Fatal(err)
	}
	mode, err := srv.GetPermissionMode(bound, &proto.GetPermissionModeRequest{})
	if err != nil || mode.GetMode() != "strict" {
		t.Fatalf("permission convergence: %v %v", mode, err)
	}
	// An ordinary cooperating writer publishes; fsnotify must apply, not resave.
	disk.ToolLoop.MaxIterations = 19
	if err := config.Save(disk, path); err != nil {
		t.Fatal(err)
	}
	published, _ := os.ReadFile(path)
	deadline := time.Now().Add(3 * time.Second)
	for srv.ConfigSnapshot().ToolLoop.MaxIterations != 19 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.ConfigSnapshot().ToolLoop.MaxIterations != 19 {
		t.Fatal("watcher did not apply published setting")
	}
	after, _ = os.ReadFile(path)
	if string(after) != string(published) {
		t.Fatal("watcher republished file")
	}
	// Unsupported file edits stay durable but unapplied. They must not advance
	// the host's save baseline and permit a later unrelated stale whole-file save.
	disk, err = config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	disk.Port = "17778"
	disk.ToolLoop.MaxIterations = 23
	if err := config.Save(disk, path); err != nil {
		t.Fatal(err)
	}
	published, _ = os.ReadFile(path)
	time.Sleep(400 * time.Millisecond)
	if srv.ConfigSnapshot().ToolLoop.MaxIterations != 19 {
		t.Fatal("partially adopted unsupported revision")
	}
	if reply, err := srv.UpdateConfig(bound, patch); err == nil && reply.GetSuccess() {
		t.Fatal("stale host overwrote published unapplied revision")
	}
	after, _ = os.ReadFile(path)
	if string(after) != string(published) {
		t.Fatal("conflict destroyed file authority")
	}
	if err := srv.ReleaseRetained(context.Background(), owner, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.UpdateConfig(bound, patch); err == nil {
		t.Fatal("released owner mutated settings")
	}
	if err := srv.RetireCoexistence(context.Background()); err != nil {
		t.Fatal(err)
	}
}
