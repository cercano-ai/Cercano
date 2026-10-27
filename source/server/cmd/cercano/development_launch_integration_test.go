package main

// This uses the real orchestration in separate OS processes, with an explicitly
// authenticated fake host. It performs no production dependency construction.
import (
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
	"sync"
	"testing"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type launchTestHost struct {
	pb.UnimplementedAgentServer
	identity *pb.LifecycleIdentity
}

func (h *launchTestHost) LifecycleList(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if !proto.Equal(r.Target, h.identity) {
		return nil, fmt.Errorf("wrong target")
	}
	return &pb.LifecycleControlResponse{Identity: h.identity}, nil
}
func (h *launchTestHost) LifecycleAcquire(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if !proto.Equal(r.Target, h.identity) {
		return nil, fmt.Errorf("wrong target")
	}
	return &pb.LifecycleControlResponse{Identity: h.identity, Owner: &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: h.identity.Host, Epoch: "fresh-" + r.Owner.Conversation}}, nil
}
func TestDevelopmentLaunchChild(t *testing.T) {
	if os.Getenv("CERCANO_LAUNCH_TEST_CHILD") != "1" {
		return
	}
	var args []string
	if e := json.Unmarshal([]byte(os.Getenv("CERCANO_LAUNCH_TEST_ARGS")), &args); e != nil {
		panic(e)
	}
	if e := runDevelopmentLaunch(context.Background(), args, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(19)
	}
	os.Exit(0)
}
func TestDevelopmentLaunchAuthenticatedConcurrentReuse(t *testing.T) {
	root, e := os.MkdirTemp("/tmp", "cl-")
	if e != nil {
		t.Fatal(e)
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(root)
	state := filepath.Join(root, "state")
	generation := filepath.Join(root, "generation")
	runtime := filepath.Join(root, "rt")
	for _, p := range []string{state, generation, runtime, filepath.Join(state, "hosts")} {
		if e = os.Mkdir(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e = conversation.PrepareDevelopmentStore(context.Background(), filepath.Join(state, "conversations.db")); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(state, "config.yaml"), []byte("{}"), 0600)
	exe, _ := os.Executable()
	binary, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(generation, "cercano"), binary, 0700)
	os.WriteFile(filepath.Join(generation, "cercano-cli"), []byte("fake cli"), 0700)
	serverSum := sha256.Sum256(binary)
	cliSum := sha256.Sum256([]byte("fake cli"))
	manifest := fmt.Sprintf(`{"build_id":"test-build","binaries":{"cercano":{"sha256":"%x"},"cercano-cli":{"sha256":"%x"}}}`, serverSum, cliSum)
	os.WriteFile(filepath.Join(generation, "manifest.json"), []byte(manifest), 0600)
	key := sha256.Sum256([]byte(generation))
	base := filepath.Join(state, "hosts", hex.EncodeToString(key[:]))
	token := base + ".token"
	if e = devhostauth.Create(token); e != nil {
		t.Fatal(e)
	}
	auth, e := devhostauth.Load("test-host", token)
	if e != nil {
		t.Fatal(e)
	}
	d := conversation.DevelopmentCoexistenceDeclaration()
	id := &pb.LifecycleIdentity{Host: "test-host", Build: "test-build", Store: filepath.Join(state, "conversations.db"), Contract: d.Contract, Schema: int32(d.SchemaVersion)}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	server := grpc.NewServer(auth.ServerOptions()...)
	pb.RegisterAgentServer(server, &launchTestHost{identity: id})
	go server.Serve(listener)
	defer server.Stop()
	record := launchSelection{Endpoint: listener.Addr().String(), TokenFile: token, Identity: id}
	if e = launchExclusive(base+".json", record); e != nil {
		t.Fatal(e)
	}
	args, _ := json.Marshal([]string{"--state", state, "--generation", generation, "--runtime-root", runtime})
	invoke := func() (string, error) {
		cmd := exec.Command(filepath.Join(generation, "cercano"), "-test.run=^TestDevelopmentLaunchChild$")
		cmd.Env = append(os.Environ(), "CERCANO_LAUNCH_TEST_CHILD=1", "CERCANO_LAUNCH_TEST_ARGS="+string(args))
		b, e := cmd.CombinedOutput()
		return strings.TrimSpace(string(b)), e
	}
	var wg sync.WaitGroup
	paths := make([]string, 2)
	errs := make([]error, 2)
	for i := range paths {
		wg.Add(1)
		go func(i int) { defer wg.Done(); paths[i], errs[i] = invoke() }(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Fatalf("launch %d: %v: %s", i, e, paths[i])
		}
	}
	if paths[0] == paths[1] {
		t.Fatal("selection reused")
	}
	var a, b launchSelection
	if e = launchRead(paths[0], &a); e != nil {
		t.Fatal(e)
	}
	if e = launchRead(paths[1], &b); e != nil {
		t.Fatal(e)
	}
	if a.Conversation == b.Conversation || a.Epoch == b.Epoch || !proto.Equal(a.Identity, b.Identity) {
		t.Fatal("fresh ownership and same-host reuse required")
	}
	before, _ := os.ReadFile(paths[0])
	server.Stop()
	output, e := invoke()
	if e == nil || !strings.Contains(output, "refusing replacement") {
		t.Fatalf("unreachable record: %v %s", e, output)
	}
	after, _ := os.ReadFile(paths[0])
	if string(before) != string(after) {
		t.Fatal("A rewritten")
	}
	if _, e = os.Stat(base + ".starting"); !os.IsNotExist(e) {
		t.Fatal("attempted replacement of unreachable host")
	}
}
