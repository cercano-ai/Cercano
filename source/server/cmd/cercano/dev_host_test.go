package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
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
	writeJSON(filepath.Join(root, "peers.json"), []devHostPeer{})
	tokenFile := filepath.Join(root, "token")
	if err := devhostauth.Create(tokenFile); err != nil {
		t.Fatal(err)
	}
	if err := conversation.PrepareCoexistence(ctx, filepath.Join(root, "conversations.db")); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var backendEntries atomic.Int64
	runtimeServer := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		backendEntries.Add(1)
		return next(ctx, req)
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
	for _, dir := range []string{"home", "config"} {
		entries, e := os.ReadDir(filepath.Join(root, dir))
		if e != nil || len(entries) != 0 {
			t.Fatalf("ambient %s touched: %v %v", dir, entries, e)
		}
	}
}
