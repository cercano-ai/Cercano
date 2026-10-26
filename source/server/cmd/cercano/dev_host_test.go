package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestDevHostCredentialGate(t *testing.T) {
	for _, value := range []string{"", "0", "true", " 1"} {
		t.Run(fmt.Sprintf("value=%q", value), func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", value)
			err := runDevHost(context.Background(), []string{"--fixture", "/does-not-exist"}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "CERCANO_DISABLE_CREDENTIALS") {
				t.Fatalf("did not reject at credential gate: %v", err)
			}
		})
	}
}

// Builds and executes the real CLI, not the Go test executable. The sole runtime
// participant is a protected synthetic server with an empty manager: no provider
// registered, no native runtime, no keychain or paid backend available.
func TestDevHostCompiledStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("builds dedicated CLI fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(root, "generation")
	for _, d := range []string{generation, filepath.Join(root, "home"), filepath.Join(root, "tmp"), filepath.Join(root, "config")} {
		if err := os.Mkdir(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(generation, "cercano")
	build := exec.CommandContext(ctx, "go", "build", "-ldflags=-X main.version=fixture-build-exact", "-o", binary, ".")
	if b, e := build.CombinedOutput(); e != nil {
		t.Fatalf("compile: %v\n%s", e, b)
	}
	b, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	writeJSON := func(path string, v any) {
		t.Helper()
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	writeJSON(filepath.Join(generation, "manifest.json"), devHostManifest{Build: "fixture-build-exact", SHA256: hex.EncodeToString(sum[:])})
	writeJSON(filepath.Join(root, "config.json"), map[string]any{})
	// Reserve the explicit B endpoint before A freezes its peer declaration.
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bEndpoint := reserved.Addr().String()
	reserved.Close()
	bToken := filepath.Join(root, "token-B")
	if err := devhostauth.Create(bToken); err != nil {
		t.Fatal(err)
	}
	writeJSON(filepath.Join(root, "peers.json"), []devHostPeer{{Endpoint: bEndpoint, Host: "fixture-host-B", TokenFile: bToken}})
	writeJSON(filepath.Join(root, "peers-B.json"), []devHostPeer{})
	tokenFile := filepath.Join(root, "token")
	if err := devhostauth.Create(tokenFile); err != nil {
		t.Fatal(err)
	}
	if err := conversation.PrepareCoexistence(ctx, filepath.Join(root, "conversations.db")); err != nil {
		t.Fatal(err)
	}
	// Explicitly seed durable fixture history/outcomes, not an execution claim.
	store, err := conversation.OpenCoexistence(ctx, filepath.Join(root, "conversations.db"), conversation.CurrentCoexistenceDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{"journey", "sibling"} {
		cap, err := conversation.AcquireOwnership(ctx, store, id, "fixture-seed")
		if err != nil {
			t.Fatal(err)
		}
		ownedCtx := conversation.WithOwnership(ctx, cap)
		if err := store.EnsureConversation(ownedCtx, id, "", "fixture"); err != nil {
			t.Fatal(err)
		}
		if err := store.Append(ownedCtx, conversation.Turn{ID: id + "-turn", ConversationID: id, Role: "assistant", Content: "durable fixture history"}); err != nil {
			t.Fatal(err)
		}
		tasks := store.(conversation.TaskStore)
		if err := tasks.InsertTask(ownedCtx, conversation.TaskRecord{ID: id + "-task", ParentID: id, Status: "accepted", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if ok, err := tasks.FinishTask(ownedCtx, conversation.TaskRecord{ID: id + "-task", ParentID: id, Status: "completed", ResultJSON: json.RawMessage(`{"output":"fixture"}`)}); err != nil || !ok {
			t.Fatal(ok, err)
		}
		if _, err := tasks.ClaimTaskWake(ownedCtx, id, id+"-task"); err != nil {
			t.Fatal(err)
		}
		if err := tasks.HandleTaskNotification(ownedCtx, id, id+"-task"); err != nil {
			t.Fatal(err)
		}
		if err := cap.Release(ctx); err != nil {
			t.Fatal(err)
		}
		cap.Close()
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var backendEntries atomic.Int64
	var participants atomic.Int64
	runtimeServer := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		backendEntries.Add(1)
		return next(ctx, req)
	}), grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		participants.Add(1)
		defer participants.Add(-1)
		return next(srv, stream)
	}))
	modelhost.NewServer(localruntime.NewManager(), modelhost.ServerOptions{Token: "fixture-modelhost-token", Version: "fixture", CoexistenceProtection: true}).Register(runtimeServer)
	go runtimeServer.Serve(listener)
	defer runtimeServer.Stop()
	modelDir := filepath.Join(root, "modelhost")
	if err := modelhost.WriteRecord(modelDir, modelhost.Record{PID: os.Getpid(), Addr: listener.Addr().String(), Token: "fixture-modelhost-token", APIVersion: modelhost.APIVersion, Version: "fixture"}); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, "dev-host", "--fixture", root, "--generation", generation, "--modelhost", modelDir, "--host", "fixture-host-exact", "--listen", "127.0.0.1:0")
	// Deliberately do not inherit any provider, credential, config, or launch env.
	command.Env = []string{"CERCANO_DISABLE_CREDENTIALS=1", "HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "TMPDIR=" + filepath.Join(root, "tmp"), "PATH=/usr/bin:/bin"}
	// The actual binary must refuse malformed/absent/enabled credentials before
	// any modelhost entry, even with otherwise complete explicit fixture args.
	for _, value := range []string{"", "0", "true"} {
		bad := exec.CommandContext(ctx, binary, command.Args[1:]...)
		bad.Env = append([]string(nil), command.Env[1:]...)
		if value != "" {
			bad.Env = append(bad.Env, "CERCANO_DISABLE_CREDENTIALS="+value)
		}
		output, err := bad.CombinedOutput()
		if err == nil || !bytes.Contains(output, []byte("CERCANO_DISABLE_CREDENTIALS")) || backendEntries.Load() != 0 {
			t.Fatalf("credential gate reached backend: %q %v %s entries=%d", value, err, output, backendEntries.Load())
		}
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Signal(os.Interrupt)
		if e := command.Wait(); e != nil {
			t.Errorf("child exit: %v\n%s", e, stderr.String())
		}
	}()
	lines := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			if strings.HasPrefix(s.Text(), "dev-host ready ") {
				lines <- s.Text()
				return
			}
		}
		close(lines)
	}()
	var line string
	select {
	case line = <-lines:
	case <-ctx.Done():
		t.Fatal("startup timed out")
	}
	fields := strings.Fields(line)
	if len(fields) < 4 {
		t.Fatalf("no ready identity: %q", line)
	}
	endpoint := fields[2]
	auth, err := devhostauth.Load("fixture-host-exact", tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	opts := append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, auth.DialOptions()...)
	conn, err := grpc.NewClient(endpoint, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := proto.NewAgentClient(conn)
	identity, err := client.GetDevelopmentHostIdentity(ctx, &proto.GetDevelopmentHostIdentityRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if identity.Build != "fixture-build-exact" || identity.HostInstance != "fixture-host-exact" || identity.Contract != conversation.CurrentCoexistenceDeclaration().Contract {
		t.Fatalf("wrong identity: %v", identity)
	}
	owned, err := client.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: &proto.LifecycleIdentity{Build: identity.Build, Host: identity.HostInstance, Contract: identity.Contract, Schema: int32(conversation.CurrentCoexistenceDeclaration().SchemaVersion), Store: filepath.Join(root, "conversations.db")}, Owner: &proto.LifecycleOwner{Conversation: "journey"}})
	if err != nil {
		t.Fatalf("native acquire: %v", err)
	}
	aIdentity := &proto.LifecycleIdentity{Build: identity.Build, Host: identity.HostInstance, Contract: identity.Contract, Schema: int32(conversation.CurrentCoexistenceDeclaration().SchemaVersion), Store: filepath.Join(root, "conversations.db")}
	pinned, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: endpoint, HostInstance: aIdentity.Host, Build: aIdentity.Build, Contract: aIdentity.Contract, SchemaVersion: aIdentity.Schema, ConversationID: "journey", OwnershipEpoch: owned.Owner.Epoch}, tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	sibling, err := client.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: aIdentity, Owner: &proto.LifecycleOwner{Conversation: "sibling"}})
	if err != nil {
		t.Fatal(err)
	}
	bGeneration := filepath.Join(root, "generation-B")
	if err := os.Mkdir(bGeneration, 0700); err != nil {
		t.Fatal(err)
	}
	bBinary := filepath.Join(bGeneration, "cercano")
	// Compile a separate immutable artifact with a distinct build identity.
	buildB := exec.CommandContext(ctx, "go", "build", "-ldflags=-X main.version=fixture-build-B", "-o", bBinary, ".")
	if output, err := buildB.CombinedOutput(); err != nil {
		t.Fatalf("build B: %v %s", err, output)
	}
	bBytes, err := os.ReadFile(bBinary)
	if err != nil {
		t.Fatal(err)
	}
	bSum := sha256.Sum256(bBytes)
	if bSum == sum {
		t.Fatal("generation artifacts not distinct")
	}
	writeJSON(filepath.Join(bGeneration, "manifest.json"), devHostManifest{Build: "fixture-build-B", SHA256: hex.EncodeToString(bSum[:])})
	bCommand := exec.CommandContext(ctx, bBinary, "dev-host", "--fixture", root, "--generation", bGeneration, "--modelhost", modelDir, "--host", "fixture-host-B", "--listen", bEndpoint, "--token-file", "token-B", "--peers-file", "peers-B.json")
	bCommand.Env = append([]string(nil), command.Env...)
	var bStderr bytes.Buffer
	bCommand.Stderr = &bStderr
	bStdout, err := bCommand.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := bCommand.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = bCommand.Process.Signal(os.Interrupt)
		if err := bCommand.Wait(); err != nil {
			t.Errorf("B exit: %v %s", err, bStderr.String())
		}
	}()
	bReady := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(bStdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "dev-host ready ") {
				bReady <- true
				return
			}
		}
		bReady <- false
	}()
	select {
	case ready := <-bReady:
		if !ready {
			t.Fatal("B not ready")
		}
	case <-ctx.Done():
		t.Fatal("B timeout")
	}
	bAuth, err := devhostauth.Load("fixture-host-B", bToken)
	if err != nil {
		t.Fatal(err)
	}
	bConn, err := grpc.NewClient(bEndpoint, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, bAuth.DialOptions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	defer bConn.Close()
	if participants.Load() != 2 {
		t.Fatalf("expected A+B runtime participation: %d", participants.Load())
	}
	bc := proto.NewAgentClient(bConn)
	bID, err := bc.GetDevelopmentHostIdentity(ctx, &proto.GetDevelopmentHostIdentityRequest{})
	if err != nil || bID.Build != "fixture-build-B" || bID.HostInstance != "fixture-host-B" {
		t.Fatal("B identity", bID, err)
	}
	wrongConn, err := grpc.NewClient(bEndpoint, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer wrongConn.Close()
	if _, err := proto.NewAgentClient(wrongConn).GetDevelopmentHostIdentity(ctx, &proto.GetDevelopmentHostIdentityRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("A token authenticated to B: %v", err)
	}
	bIdentity := &proto.LifecycleIdentity{Build: "fixture-build-B", Host: "fixture-host-B", Contract: aIdentity.Contract, Schema: aIdentity.Schema, Store: aIdentity.Store}
	for _, id := range []string{"journey", "sibling"} {
		turns, err := store.GetTurns(ctx, id)
		if err != nil || len(turns) != 1 || turns[0].Content != "durable fixture history" {
			t.Fatalf("B startup changed history: %v %v", turns, err)
		}
		task, err := store.(conversation.TaskStore).GetTask(ctx, id, id+"-task")
		if err != nil || task.Status != "completed" || string(task.ResultJSON) != `{"output":"fixture"}` {
			t.Fatalf("B startup changed task: %v %v", task, err)
		}
	}
	transfer := &proto.LifecycleControlRequest{Target: bIdentity, Source: aIdentity, Destination: bIdentity, DestinationEndpoint: bEndpoint, Owner: owned.Owner, HandoverId: "native-A-to-B", Management: true}
	accepted, err := bc.LifecyclePrepare(ctx, transfer)
	if err != nil {
		t.Fatal(err)
	}
	transfer.Receipt = accepted.Receipt
	if _, err := bc.LifecycleClaim(ctx, transfer); err == nil {
		t.Fatal("claimed before source relinquished")
	}
	transfer.Target = aIdentity
	if _, err := client.LifecycleRelinquish(ctx, transfer); err != nil {
		t.Fatal(err)
	}
	transfer.Target = bIdentity
	claimed, err := bc.LifecycleClaim(ctx, transfer)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Owner.Host != bIdentity.Host || claimed.Owner.Epoch == owned.Owner.Epoch {
		t.Fatal("wrong claimed ownership", claimed)
	}
	if err := pinned.ReconnectPinnedSession(ctx); !errors.Is(err, agentclient.ErrPinnedSession) {
		t.Fatalf("old A reconnect did not refuse: %v", err)
	}
	resumed, err := bc.ResumeConversation(ctx, &proto.ResumeConversationRequest{ConversationId: "journey"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resumed.String(), "durable fixture history") {
		t.Fatal("B did not resume history", resumed)
	}
	if _, err := client.LifecycleRetire(ctx, &proto.LifecycleControlRequest{Target: aIdentity}); err == nil {
		t.Fatal("retired with sibling")
	}
	if _, err := client.LifecycleRelease(ctx, &proto.LifecycleControlRequest{Target: aIdentity, Owner: sibling.Owner, Management: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.LifecycleRetire(ctx, &proto.LifecycleControlRequest{Target: aIdentity}); err != nil {
		t.Fatal(err)
	}
	if _, err := bc.ResumeConversation(ctx, &proto.ResumeConversationRequest{ConversationId: "journey"}); err != nil {
		t.Fatal("A retirement disturbed B", err)
	}
	if err := pinned.ReconnectPinnedSession(ctx); !errors.Is(err, agentclient.ErrPinnedSession) {
		t.Fatalf("retired A reconnect did not refuse: %v", err)
	}
	bPolicy, err := bc.GetCredentialPolicy(ctx, &proto.GetCredentialPolicyRequest{})
	if err != nil || !bPolicy.GetCredentialsDisabled() {
		t.Fatal("B credential exclusion", bPolicy, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for participants.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if participants.Load() != 1 {
		t.Fatalf("A retirement disturbed shared participation: %d", participants.Load())
	}
	t.Log("separate compiled A/B: retained parent+sibling, B preserved history/tasks, quiescent handover claimed fresh B epoch, resumed history, released sibling and retired A; B remains usable")
	policy, err := client.GetCredentialPolicy(ctx, &proto.GetCredentialPolicyRequest{})
	if err != nil || !policy.GetCredentialsDisabled() {
		t.Fatalf("credential exclusion: %v %v", policy, err)
	}
	bare, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer bare.Close()
	_, err = proto.NewAgentClient(bare).GetDevelopmentHostIdentity(ctx, &proto.GetDevelopmentHostIdentityRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing auth identity: %v", err)
	}
	_, err = proto.NewAgentClient(bare).LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: aIdentity, Owner: &proto.LifecycleOwner{Conversation: "unauthorized"}})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated mutation: %v", err)
	}
	turns, err := store.GetTurns(ctx, "unauthorized")
	if err != nil || len(turns) != 0 {
		t.Fatal("unauthorized persistence", turns, err)
	}
	protected, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, proto.Agent_InstallOpenRuntime_FullMethodName)
	if err == nil {
		_ = protected.CloseSend()
		err = protected.RecvMsg(&proto.GetCredentialPolicyResponse{})
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("authenticated execution must remain disabled: %v", err)
	}
	// Authentication precedes the fixture stream execution guard.
	stream, err := bare.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, proto.Agent_InstallOpenRuntime_FullMethodName)
	if err == nil {
		_ = stream.CloseSend()
		err = stream.RecvMsg(&proto.GetCredentialPolicyResponse{})
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing stream auth: %v", err)
	}
	configBytes, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil || string(configBytes) != "{}" {
		t.Fatal("frozen config mutated", string(configBytes), err)
	}
	for _, dir := range []string{"home", "config"} {
		entries, e := os.ReadDir(filepath.Join(root, dir))
		if e != nil || len(entries) != 0 {
			t.Fatalf("ambient %s touched: %v %v", dir, entries, e)
		}
	}
}
