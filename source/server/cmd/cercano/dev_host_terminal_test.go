package main

import (
	"bufio"
	"bytes"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/modelhost"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestDevHostTerminalPTY uses only private seeded stores, empty synthetic modelhost,
// tagged restricted workers and actual standalone terminal processes.
func TestDevHostTerminalPTY(t *testing.T) {
	execution := true
	if testing.Short() {
		t.Skip("builds dedicated CLI fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	tempRoot := t.TempDir()
	if execution {
		var e error
		tempRoot, e = os.MkdirTemp("/tmp", "dh-")
		if e != nil {
			t.Fatal(e)
		}
		defer os.RemoveAll(tempRoot)
		defer func() {
			if t.Failed() {
				paths, _ := filepath.Glob(filepath.Join(tempRoot, "w-*", "*.log"))
				for _, p := range paths {
					b, _ := os.ReadFile(p)
					t.Logf("worker log: %s", b)
				}
			}
		}()
	}
	root, err := filepath.EvalSymlinks(tempRoot)
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
	if execution {
		build.Args = append(build.Args[:2], append([]string{"-tags=devhostfixture"}, build.Args[2:]...)...)
	}
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
	command.Env = []string{"CERCANO_DISABLE_CREDENTIALS=1", "CERCANO_DEV_FIXTURE_BARRIER=" + root, "HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "TMPDIR=" + filepath.Join(root, "tmp"), "PATH=/usr/bin:/bin"}
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
	if execution {
		for _, barrier := range []string{"relative", root + "/../outside", t.TempDir()} {
			bad := exec.CommandContext(ctx, binary, command.Args[1:]...)
			bad.Env = append(append([]string{}, command.Env...), "CERCANO_DEV_FIXTURE_BARRIER="+barrier)
			out, err := bad.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "invalid fixture barrier") || backendEntries.Load() != 0 {
				t.Fatalf("invalid barrier reached backend: %q %v %s", barrier, err, out)
			}
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
		if t.Failed() {
			t.Logf("host stderr: %s", stderr.String())
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
	bGeneration := filepath.Join(root, "generation-B")
	if err := os.Mkdir(bGeneration, 0700); err != nil {
		t.Fatal(err)
	}
	bBinary := filepath.Join(bGeneration, "cercano")
	// Compile a separate immutable artifact with a distinct build identity.
	buildB := exec.CommandContext(ctx, "go", "build", "-ldflags=-X main.version=fixture-build-B", "-o", bBinary, ".")
	if execution {
		buildB.Args = append(buildB.Args[:2], append([]string{"-tags=devhostfixture"}, buildB.Args[2:]...)...)
	}
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

	cliBinary := filepath.Join(root, "terminal")
	cliBuild := exec.CommandContext(ctx, "go", "build", "-ldflags=-X main.version=fixture-TUI-unchanged", "-o", cliBinary, ".")
	cliBuild.Dir = "../../../clients/cli"
	if out, e := cliBuild.CombinedOutput(); e != nil {
		t.Fatalf("terminal build: %v %s", e, out)
	}
	writeJSON(filepath.Join(root, "a.json"), map[string]any{"Endpoint": endpoint, "TokenFile": tokenFile, "Identity": aIdentity, "Conversation": "journey", "Epoch": owned.Owner.Epoch})
	bIdentity := &proto.LifecycleIdentity{Build: bID.Build, Host: bID.HostInstance, Contract: aIdentity.Contract, Schema: aIdentity.Schema, Store: aIdentity.Store}
	writeJSON(filepath.Join(root, "b.json"), map[string]any{"Endpoint": bEndpoint, "TokenFile": bToken, "Identity": bIdentity, "Conversation": "journey"})
	harness := exec.CommandContext(ctx, "python3", "../../../../efforts/dev-host-generations/terminal-pty.py", cliBinary, root)
	harness.Env = append(command.Env, "TERM=xterm-256color")
	out, e := harness.CombinedOutput()
	t.Logf("PTY output: %s", out)
	if e != nil {
		t.Fatal(e)
	}
}
