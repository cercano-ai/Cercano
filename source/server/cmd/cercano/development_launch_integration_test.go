package main

// This uses the real orchestration in separate OS processes, with an explicitly
// authenticated fake host. It performs no production dependency construction.
import (
	"bytes"
	"cercano/source/server/internal/brewrestart"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type launchTestHost struct {
	pb.UnimplementedAgentServer
	identity         *pb.LifecycleIdentity
	reject           atomic.Bool
	entered          chan struct{}
	ready            chan struct{}
	acquired         sync.Map // Fixture ownership, including acquisitions never delivered.
	releases         atomic.Int64
	selectionFailure atomic.Value // Optional fixture-only exclusive-create collision directory.
}

func (h *launchTestHost) LifecycleList(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if h.reject.Load() {
		return nil, fmt.Errorf("not ready")
	}
	if h.entered != nil {
		select {
		case h.entered <- struct{}{}:
		default:
		}
		select {
		case <-h.ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if !proto.Equal(r.Target, h.identity) {
		return nil, fmt.Errorf("wrong target")
	}
	return &pb.LifecycleControlResponse{Identity: h.identity}, nil
}
func (h *launchTestHost) LifecycleAcquire(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if !proto.Equal(r.Target, h.identity) {
		return nil, fmt.Errorf("wrong target")
	}
	owner := &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: h.identity.Host, Epoch: "fresh-" + r.Owner.Conversation}
	h.acquired.Store(owner.Conversation, owner)
	if dir, _ := h.selectionFailure.Load().(string); dir != "" {
		if err := os.Mkdir(filepath.Join(dir, owner.Conversation+".json"), 0700); err != nil {
			return nil, err
		}
	}
	return &pb.LifecycleControlResponse{Identity: h.identity, Owner: owner}, nil
}
func (h *launchTestHost) LifecycleRelease(ctx context.Context, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if !proto.Equal(r.Target, h.identity) {
		return nil, fmt.Errorf("wrong target")
	}
	owner, ok := h.acquired.Load(r.GetOwner().GetConversation())
	if !ok || !proto.Equal(owner.(*pb.LifecycleOwner), r.Owner) {
		return nil, fmt.Errorf("wrong exact owner")
	}
	h.acquired.Delete(r.Owner.Conversation)
	h.releases.Add(1)
	return &pb.LifecycleControlResponse{Identity: h.identity}, nil
}
func TestDevelopmentLaunchChild(t *testing.T) {
	if os.Getenv("CERCANO_LAUNCH_TEST_CHILD") != "1" {
		return
	}
	var args []string
	if e := json.Unmarshal([]byte(os.Getenv("CERCANO_LAUNCH_TEST_ARGS")), &args); e != nil {
		panic(e)
	}
	if e := runDevelopmentLaunchWithRecovery(context.Background(), args, os.Stdout, func(_ context.Context, _, _, _ string, _ launchSelection, cause error) error {
		return fmt.Errorf("fixture host uncertain; refusing replacement: %w", cause)
	}); e != nil {
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
	host := &launchTestHost{identity: id, entered: make(chan struct{}, 2), ready: make(chan struct{})}
	pb.RegisterAgentServer(server, host)
	go server.Serve(listener)
	defer server.Stop()
	record := launchSelection{ConfigAuthority: filepath.Join(state, "config.yaml"), Endpoint: listener.Addr().String(), TokenFile: token, Identity: id}
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
	previous := launchSelection{Identity: &pb.LifecycleIdentity{Host: "A", Store: id.Store, Contract: id.Contract, Schema: id.Schema}, Generation: "A"}
	if e = publishCurrentTarget(state, previous); e != nil {
		t.Fatal(e)
	}
	assertA := func() {
		t.Helper()
		var target launchSelection
		if err := launchRead(filepath.Join(state, "current-target.json"), &target); err != nil || target.Identity.GetHost() != "A" {
			t.Fatalf("A target lost before B readiness: %+v %v", target, err)
		}
	}
	host.reject.Store(true)
	if output, err := invoke(); err == nil {
		t.Fatalf("unready B succeeded: %s", output)
	}
	assertA()
	host.reject.Store(false)
	var wg sync.WaitGroup
	paths := make([]string, 2)
	errs := make([]error, 2)
	for i := range paths {
		wg.Add(1)
		go func(i int) { defer wg.Done(); paths[i], errs[i] = invoke() }(i)
	}
	<-host.entered
	assertA()
	close(host.ready)
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
	var current launchSelection
	if e = launchRead(filepath.Join(state, "current-target.json"), &current); e != nil {
		t.Fatalf("successful new-session launch did not publish current target: %v", e)
	}
	if !proto.Equal(current.Identity, id) || current.Generation != generation || current.Conversation != "" || current.Epoch != "" {
		t.Fatalf("invalid current target: %+v", current)
	}
	// Characterization only: record the leak before choosing a safe delivery
	// protocol. These assertions intentionally describe the unfixed behavior.
	for _, boundary := range []string{"selection-write", "target-publication"} {
		t.Run("ReproduceUndeliveredOwnershipLeak/"+boundary, func(t *testing.T) {
			count := func() int {
				n := 0
				host.acquired.Range(func(_, _ any) bool { n++; return true })
				return n
			}
			beforeCount, beforeReleases := count(), host.releases.Load()
			target := filepath.Join(state, "current-target.json")
			if boundary == "selection-write" {
				host.selectionFailure.Store(filepath.Join(state, "selections"))
				defer host.selectionFailure.Store("")
			} else {
				if err := os.Rename(target, target+".fixture-backup"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Remove(target); err != nil {
						t.Error(err)
					}
					if err := os.Rename(target+".fixture-backup", target); err != nil {
						t.Error(err)
					}
				}()
			}
			output, err := invoke()
			if err == nil {
				t.Fatalf("expected fixture launch failure, got %s", output)
			}
			if count() != beforeCount+1 || host.releases.Load() != beforeReleases {
				t.Fatalf("baseline changed: owners %d -> %d; releases %d -> %d", beforeCount, count(), beforeReleases, host.releases.Load())
			}
			t.Logf("REPRODUCED: %s failed, exact acquired ownership remains, no release RPC: %s", boundary, output)
		})
	}
	targetBefore, _ := os.ReadFile(filepath.Join(state, "current-target.json"))
	before, _ := os.ReadFile(paths[0])
	server.Stop()
	output, e := invoke()
	if e == nil || !strings.Contains(output, "refusing replacement") {
		t.Fatalf("unreachable record: %v %s", e, output)
	}
	targetAfter, _ := os.ReadFile(filepath.Join(state, "current-target.json"))
	if string(targetBefore) != string(targetAfter) {
		t.Fatal("failed launch changed current target")
	}
	after, _ := os.ReadFile(paths[0])
	if string(before) != string(after) {
		t.Fatal("A rewritten")
	}
	if _, e = os.Stat(base + ".starting"); !os.IsNotExist(e) {
		t.Fatal("attempted replacement of unreachable host")
	}
	// Restore a live but admission-closed predecessor. Force launch must neither
	// stop its listener nor rewrite its records or existing conversation selection.
	oldListener, err := net.Listen("tcp", record.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	oldServer := grpc.NewServer(auth.ServerOptions()...)
	host.reject.Store(true)
	pb.RegisterAgentServer(oldServer, host)
	go oldServer.Serve(oldListener)
	defer oldServer.Stop()
	cmd := exec.Command(filepath.Join(generation, "cercano"), "-test.run=^TestDevelopmentLaunchRecoveryChild$")
	cmd.Env = append(os.Environ(), "CERCANO_RECOVERY_TEST_CHILD=1", "CERCANO_LAUNCH_TEST_ARGS="+string(args), "CERCANO_OLD_SELECTION="+paths[0])
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("force recovery integration: %v %s", err, output)
	}
}

func TestDevelopmentLaunchRecoveryChild(t *testing.T) {
	if os.Getenv("CERCANO_RECOVERY_TEST_CHILD") != "1" {
		return
	}
	args := []byte(os.Getenv("CERCANO_LAUNCH_TEST_ARGS"))
	var decoded []string
	json.Unmarshal(args, &decoded)
	state, generation := decoded[1], decoded[3]
	key := sha256.Sum256([]byte(generation))
	base := filepath.Join(state, "hosts", hex.EncodeToString(key[:]))
	var record launchSelection
	if err := launchRead(base+".json", &record); err != nil {
		t.Fatal(err)
	}
	id := record.Identity
	paths := []string{os.Getenv("CERCANO_OLD_SELECTION")}
	before, _ := os.ReadFile(paths[0])
	var a launchSelection
	launchRead(paths[0], &a)
	invoke := func() (string, error) {
		cmd := exec.Command(filepath.Join(generation, "cercano"), "-test.run=^TestDevelopmentLaunchChild$")
		cmd.Env = append(os.Environ(), "CERCANO_LAUNCH_TEST_CHILD=1", "CERCANO_LAUNCH_TEST_ARGS="+string(args))
		b, e := cmd.CombinedOutput()
		return strings.TrimSpace(string(b)), e
	}

	// Missing credentials must reach the same conservative recovery decision.
	if err := os.Remove(record.TokenFile); err != nil {
		t.Fatal(err)
	}
	oldRecord, _ := os.ReadFile(base + ".json")
	var launchArgs []string
	json.Unmarshal(args, &launchArgs)
	spawned := 0
	command := func(_ string, state, gen, hostID, token, recordPath, _ string, _ io.Writer, authority ...string) *exec.Cmd {
		spawned++
		fixtureAuth, err := devhostauth.Load(hostID, token)
		if err != nil {
			t.Fatal(err)
		}
		fixtureID := proto.Clone(id).(*pb.LifecycleIdentity)
		fixtureID.Host = hostID
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		srv := grpc.NewServer(fixtureAuth.ServerOptions()...)
		pb.RegisterAgentServer(srv, &launchTestHost{identity: fixtureID})
		go srv.Serve(l)
		t.Cleanup(srv.Stop)
		if err := launchExclusive(recordPath, launchSelection{ConfigAuthority: authority[0], Endpoint: l.Addr().String(), TokenFile: token, Identity: fixtureID}); err != nil {
			t.Fatal(err)
		}
		return exec.Command("/usr/bin/true")
	}
	uncertain := developmentRecoveryOps{
		absent: func(context.Context, string) error { return fmt.Errorf("fixture uncertainty") },
		prepare: func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error) {
			return brewrestart.Identity{}, fmt.Errorf("unverified fixture process")
		},
		stop: func(context.Context, string, netip.AddrPort, brewrestart.Identity, func() error) error {
			t.Fatal("force attempted stop")
			return nil
		},
	}
	recoverWith := func(yes bool) developmentBlockedRecovery {
		return func(ctx context.Context, base, gen, authority string, s launchSelection, cause error) error {
			return developmentReplaceBlockedWith(ctx, base, gen, authority, s, cause, func(string) bool { return yes }, uncertain)
		}
	}
	verifiedButUnauthenticated := func(ctx context.Context, base, gen, authority string, s launchSelection, cause error) error {
		ops := uncertain
		ops.prepare = func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error) {
			return brewrestart.Identity{}, nil
		}
		return developmentReplaceBlockedWith(ctx, base, gen, authority, s, cause, func(string) bool { return false }, ops)
	}
	var selected bytes.Buffer
	if err := runDevelopmentLaunchWithHooks(context.Background(), launchArgs, &selected, verifiedButUnauthenticated, command); err == nil || !strings.Contains(err.Error(), "force launch declined") || spawned != 0 {
		t.Fatalf("missing token granted stop authority: %v", err)
	}
	// Independent verified absence is sufficient, without consent or process stop.
	absenceChecked := false
	safeRecovery := func(ctx context.Context, base, gen, authority string, s launchSelection, cause error) error {
		ops := uncertain
		ops.absent = func(context.Context, string) error { absenceChecked = true; return nil }
		ops.prepare = func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error) {
			t.Fatal("absence attempted process inspection")
			return brewrestart.Identity{}, nil
		}
		err := developmentReplaceBlockedWith(ctx, base, gen, authority, s, cause, func(string) bool { t.Fatal("absence prompted"); return false }, ops)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Errorf("fixture verified absence; stop before spawn")
	}
	if err := runDevelopmentLaunchWithHooks(context.Background(), launchArgs, &selected, safeRecovery, command); err == nil || !absenceChecked || spawned != 0 {
		t.Fatalf("missing token bypassed safe recovery: %v", err)
	}
	// An unusable credential is also uncertainty, never proof of death.
	if err := os.WriteFile(record.TokenFile, []byte("unusable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runDevelopmentLaunchWithHooks(context.Background(), launchArgs, &selected, verifiedButUnauthenticated, command); err == nil || !strings.Contains(err.Error(), "force launch declined") || spawned != 0 {
		t.Fatalf("unusable token bypassed consent: %v", err)
	}
	if err := os.Remove(record.TokenFile); err != nil {
		t.Fatal(err)
	}
	if err := runDevelopmentLaunchWithHooks(context.Background(), launchArgs, &selected, recoverWith(false), command); err == nil || !strings.Contains(err.Error(), "force launch declined") || spawned != 0 {
		t.Fatalf("declined force spawned host: %v %d", err, spawned)
	}
	if err := runDevelopmentLaunchWithHooks(context.Background(), launchArgs, &selected, recoverWith(true), command); err != nil {
		t.Fatal(err)
	}
	if spawned != 1 {
		t.Fatalf("spawns=%d", spawned)
	}
	var fresh launchSelection
	if err := launchRead(strings.TrimSpace(selected.String()), &fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.Identity.Host == id.Host || fresh.Generation != generation || fresh.Conversation == a.Conversation {
		t.Fatalf("not a distinct same-build owner: %+v", fresh)
	}
	for _, path := range []string{base + ".json", paths[0]} {
		got, _ := os.ReadFile(path)
		want := oldRecord
		if path == paths[0] {
			want = before
		}
		if string(got) != string(want) {
			t.Fatalf("force rewrote predecessor %s", path)
		}
	}
	if c, err := net.Dial("tcp", record.Endpoint); err != nil {
		t.Fatalf("old host stopped: %v", err)
	} else {
		c.Close()
	}
	// Subsequent launches (including separate OS processes) reuse the healthy
	// selected instance, without another prompt or spawn.
	result, err := invoke()
	if err != nil {
		t.Fatalf("instance reuse: %v %s", err, result)
	}
	var reused launchSelection
	if err := launchRead(result, &reused); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(reused.Identity, fresh.Identity) || reused.Conversation == fresh.Conversation {
		t.Fatal("new instance reuse/ownership failed")
	}
	guard := developmentRecoveryGuardWithVerifier(state, fresh.Identity, func(_ context.Context, host string) error {
		if host != id.Host {
			t.Fatalf("wrong exact source %s", host)
		}
		return fmt.Errorf("still alive")
	})
	// Legacy record-only fixtures lack an intent: exact source resolution fails
	// closed rather than inferring authority from current discovery.
	if release, err := guard(context.Background(), id.Host); err == nil {
		release()
		t.Fatal("old live source ownership seized")
	}

}
