package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Opt-in cross-module probe. The CLI owns this disposable subprocess, paths and
// lifetime. No installed executable, user configuration or ambient endpoint.
func TestProductionResumeBridge(t *testing.T) {
	manifest := os.Getenv("CERCANO_RESUME_PROBE_MANIFEST")
	if manifest == "" {
		t.Skip("requires explicit disposable CLI probe")
	}
	mode := os.Getenv("CERCANO_RESUME_PROBE_MODE")
	startOrdinaryGenerationObserved(t, func(s *server.Server, o developmentHostOptions, endpoint string, stop func()) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		token := filepath.Join(filepath.Dir(o.State), "token")
		auth, err := devhostauth.Load(o.Host, token)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := grpc.NewClient(endpoint, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, auth.DialOptions()...)...)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		acquired, err := proto.NewAgentClient(conn).LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: o.Identity, Owner: &proto.LifecycleOwner{Conversation: "resume-probe"}})
		if err != nil {
			t.Fatal(err)
		}
		client, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: endpoint, HostInstance: o.Host, Build: o.Identity.Build, Contract: o.Identity.Contract, SchemaVersion: o.Identity.Schema, ConversationID: "resume-probe", OwnershipEpoch: acquired.Owner.Epoch}, token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = client.AttachConversation(ctx, "resume-probe"); err != nil {
			t.Fatal(err)
		}
		if mode == "released" {
			if err = client.ReleaseSession(ctx, o.Identity); err != nil {
				t.Fatal(err)
			}
		}
		client.Close()
		record, err := conversation.ReadOwnership(ctx, s.GenerationStore(), "resume-probe")
		if err != nil {
			t.Fatal(err)
		}
		want := "owned"
		if mode == "released" {
			want = "released"
		}
		if record.State != want {
			t.Fatalf("ownership=%+v", record)
		}
		if mode == "dead" {
			data, _ := json.Marshal(o.Identity)
			if err := os.WriteFile(manifest, data, 0600); err != nil {
				t.Fatal(err)
			}
			os.Exit(0) // deliberate test-owned process death, no cleanup/release
		}
		stop() // closed listener is NOT a dead process; owned capability remains locked
		nextToken := filepath.Join(filepath.Dir(o.State), "next-token")
		if err = devhostauth.Create(nextToken); err != nil {
			t.Fatal(err)
		}
		nextAuth, err := devhostauth.Load("next-fixture-host", nextToken)
		if err != nil {
			t.Fatal(err)
		}
		next, err := server.NewCoexistenceServer(ctx, server.CoexistenceOptions{StorePath: o.Identity.Store, Host: "next-fixture-host", Build: o.Identity.Build, Contract: conversation.DevelopmentCoexistenceDeclaration(), ControlAuthorize: nextAuth.Authorize, Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }})
		if err != nil {
			t.Fatal(err)
		}
		defer next.GenerationStore().Close()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		gs := grpc.NewServer(nextAuth.ServerOptions()...)
		proto.RegisterAgentServer(gs, next)
		go gs.Serve(listener)
		defer gs.Stop()
		identity := &proto.LifecycleIdentity{Host: "next-fixture-host", Build: o.Identity.Build, Contract: o.Identity.Contract, Schema: o.Identity.Schema, Store: o.Identity.Store}
		// An unrelated, genuinely unreachable saved host: bind then close a test-owned port.
		dead, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		deadEndpoint := dead.Addr().String()
		dead.Close()
		selection := func(ep, tok string, id *proto.LifecycleIdentity) map[string]any {
			return map[string]any{"Endpoint": ep, "TokenFile": tok, "Identity": id}
		}
		hosts := filepath.Join(filepath.Dir(o.Identity.Store), "hosts")
		if err = os.MkdirAll(hosts, 0700); err != nil {
			t.Fatal(err)
		}
		unrelated := &proto.LifecycleIdentity{Host: "unrelated-fixture-host", Build: o.Identity.Build, Contract: o.Identity.Contract, Schema: o.Identity.Schema, Store: o.Identity.Store}
		stale, _ := json.Marshal(selection(deadEndpoint, token, unrelated))
		if err = os.WriteFile(filepath.Join(hosts, "unrelated.json"), stale, 0600); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(selection(listener.Addr().String(), nextToken, identity))
		if err = os.WriteFile(manifest, data, 0600); err != nil {
			t.Fatal(err)
		}
		// Parent requests actual CLI reopen before allowing cleanup.
		io.Copy(io.Discard, os.Stdin)
	})
}

func TestProductionResumeDeadOwner(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	manifest := filepath.Join(root, "dead.json")
	cmd := exec.Command(binary, "-test.run=^TestProductionResumeBridge$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "TMPDIR="+root, "CERCANO_RESUME_PROBE_MANIFEST="+manifest, "CERCANO_RESUME_PROBE_MODE=dead")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v %s", err, out)
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var identity proto.LifecycleIdentity
	if err = json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	next, err := server.NewCoexistenceServer(ctx, server.CoexistenceOptions{StorePath: identity.Store, Host: "after-process-death", Build: identity.Build, Contract: conversation.DevelopmentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer next.GenerationStore().Close()
	record, err := conversation.ReadOwnership(ctx, next.GenerationStore(), "resume-probe")
	if err != nil || record.State != "owned" {
		t.Fatalf("dead owner: %+v %v", record, err)
	}
	_, err = next.Retain(ctx, "resume-probe")
	if err == nil || !strings.Contains(err.Error(), "crash recovery unsupported") {
		t.Fatalf("expected durable refusal AFTER child exit, not OS lock contention: %v", err)
	}
	t.Logf("process exited; durable owner %+v; acquisition refused: %v", record, err)
}
