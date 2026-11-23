package server

// This probe deliberately uses the production callback installation. In contrast
// to prepared_worker_mutation_test it never installs failure-evidence wrappers.
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Only the worker protocol peer is synthetic. The host runner, spawn policy,
// callback wiring, SQLite and authenticated control transport are production.
type closeProbePeer struct{ pb.UnimplementedWorkerServer }

func (closeProbePeer) RunTurn(stream pb.Worker_RunTurnServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	raw, _ := json.Marshal(conversation.AutonomyRun{ConversationID: "parent", State: "running"})
	if err := stream.Send(&pb.WorkerToHost{Msg: &pb.WorkerToHost_AutonomyRequest{AutonomyRequest: &pb.AutonomyLedgerRequest{Id: 1, Op: "create", RunJson: raw}}}); err != nil {
		return err
	}
	if _, err := stream.Recv(); err != nil {
		return err
	}
	root := os.Getenv("CERCANO_CLOSE_PROBE_ROOT")
	if err := os.WriteFile(filepath.Join(root, "callback-returned"), nil, 0600); err != nil {
		return err
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		if _, err := os.Stat(filepath.Join(root, "drop-response")); err == nil {
			return nil
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("probe gate timeout")
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-time.After(time.Millisecond):
		}
	}
}
func init() {
	if os.Getenv("CERCANO_CLOSE_PROBE_WORKER") != "1" || len(os.Args) < 4 || os.Args[1] != "worker" {
		return
	}
	time.AfterFunc(22*time.Second, func() { os.Exit(94) }) // self-bound test-owned worker even if host watchdog fires
	lis, err := net.Listen("unix", os.Args[3])
	if err != nil {
		os.Exit(91)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("CERCANO_CLOSE_PROBE_ROOT"), "worker-socket"), []byte(os.Args[3]), 0600); err != nil {
		os.Exit(93)
	}
	g := grpc.NewServer()
	pb.RegisterWorkerServer(g, closeProbePeer{})
	if g.Serve(lis) != nil {
		os.Exit(92)
	}
	os.Exit(0)
}

// Each case runs in a disposable independent host process. A parent watchdog
// bounds even a broken synchronous Shutdown/join (the regression under probe).
func TestProcessCloseInstalledCallbacks(t *testing.T) {
	for _, mode := range []string{"success", "write-failure"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestProcessCloseHostProbe$", "-test.v")
			cmd.Env = append(os.Environ(), "CERCANO_CLOSE_PROBE_ROOT="+root, "CERCANO_CLOSE_PROBE_MODE="+mode, "CERCANO_DISABLE_CREDENTIALS=1", "HOME="+root, "XDG_CONFIG_HOME="+root)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("disposable host %s: %v (watchdog=%v)\n%s", mode, err, ctx.Err(), out)
			}
			t.Logf("%s", out)
		})
	}
}
func TestProcessCloseHostProbe(t *testing.T) {
	root := os.Getenv("CERCANO_CLOSE_PROBE_ROOT")
	if root == "" {
		return
	} // helper, not a skipped acceptance
	ctx, cancel := context.WithTimeout(t.Context(), 18*time.Second)
	defer cancel()
	s, owned, owner, _ := preparedWakeFixture(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	short, err := os.MkdirTemp("/tmp", "cp-")
	must(err)
	defer os.RemoveAll(short)
	short, err = filepath.EvalSymlinks(short)
	must(err)
	gen := filepath.Join(short, "gen")
	must(os.Mkdir(gen, 0700))
	exe, err := os.Executable()
	must(err)
	data, err := os.ReadFile(exe)
	must(err)
	must(os.WriteFile(filepath.Join(gen, "cercano"), data, 0700))
	manifest := filepath.Join(gen, "manifest.json")
	must(os.WriteFile(manifest, []byte(`{"fixture":true}`), 0600))
	must(s.persistSvc.Store().Append(owned, conversation.Turn{ConversationID: owner.Conversation, Role: "assistant", Content: "process close evidence"}))
	policy, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: gen, ManifestPath: manifest, ResourceRoot: short, HostInstance: owner.Host})
	must(err)
	t.Setenv("CERCANO_CLOSE_PROBE_WORKER", "1")
	s.SetPermissions(agent.NewStaticPermissionStore(agent.ModeBypass), nil)
	must(s.ConfigurePreparedWorker(policy))
	defer s.workerRunner.(interface{ CleanupOwnedWorkers() }).CleanupOwnedWorkers()
	if os.Getenv("CERCANO_CLOSE_PROBE_MODE") == "write-failure" {
		db, err := sql.Open("sqlite", s.coexistence.options.StorePath)
		must(err)
		defer db.Close()
		_, err = db.Exec(`CREATE TRIGGER injected BEFORE INSERT ON autonomy_runs BEGIN SELECT RAISE(ABORT, 'real callback failure'); END`)
		must(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.workerRunner.RunTurn(owned, runner.Request{ConversationID: owner.Conversation, Input: "probe"}, nil, nil, nil)
		done <- err
	}()
	wait := func(check func() bool) {
		t.Helper()
		for !check() {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(time.Millisecond):
			}
		}
	}
	wait(func() bool { _, err := os.Stat(filepath.Join(root, "callback-returned")); return err == nil })
	attachment, err := s.coexistence.admission.AttachTerminal(owner, "probe")
	must(err)
	defer attachment.Detach()
	token := filepath.Join(root, "token")
	must(devhostauth.Create(token))
	auth, err := devhostauth.Load(owner.Host, token)
	must(err)
	s.coexistence.options.ControlAuthorize = auth.Authorize
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	g := grpc.NewServer(auth.ServerOptions()...)
	pb.RegisterAgentServer(g, s)
	go g.Serve(lis)
	defer g.Stop()
	opts := append(auth.DialOptions(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(lis.Addr().String(), opts...)
	must(err)
	defer conn.Close()
	rpcctx := metadata.AppendToOutgoingContext(ctx, agentclient.PinnedHostMetadata, owner.Host, agentclient.PinnedConversationMetadata, owner.Conversation, agentclient.PinnedEpochMetadata, owner.Epoch, agentclient.PinnedTerminalMetadata, "probe")
	_, err = pb.NewAgentClient(conn).LifecycleRelease(rpcctx, &pb.LifecycleControlRequest{Target: s.coexistence.controlIdentity(), Source: s.coexistence.controlIdentity(), Owner: wireOwner(owner)})
	must(err)
	// Transport close has not granted authority: the pending worker still owns a ticket.
	row, err := conversation.ReadOwnership(ctx, s.coexistence.store, owner.Conversation)
	must(err)
	if row.State != "owned" {
		t.Fatalf("released before worker completion: %+v", row)
	}
	cap, err := conversation.AcquireOwnership(ctx, s.coexistence.store, owner.Conversation, "replacement")
	if cap != nil {
		_ = cap.Close()
	}
	if err == nil {
		t.Fatal("acquired before drain")
	}
	must(os.WriteFile(filepath.Join(root, "drop-response"), nil, 0600))
	select {
	case err := <-done:
		t.Logf("worker outcome: %v", err)
	case <-ctx.Done():
		t.Fatal("worker completion join exceeded deadline")
	}
	s.workerRunner.(interface{ CleanupOwnedWorkers() }).CleanupOwnedWorkers()
	socket, err := os.ReadFile(filepath.Join(root, "worker-socket"))
	must(err)
	if _, err := os.Stat(string(socket)); !os.IsNotExist(err) {
		t.Fatalf("old worker socket remains: %v", err)
	}
	if os.Getenv("CERCANO_CLOSE_PROBE_MODE") == "write-failure" {
		snap := s.coexistence.admission.Snapshot(owner)
		if snap.UnresolvedWork == 0 {
			t.Fatalf("production callback lost mutation uncertainty: %+v", snap)
		}
		cap, err := conversation.AcquireOwnership(ctx, s.coexistence.store, owner.Conversation, "replacement")
		if cap != nil {
			_ = cap.Close()
		}
		if err == nil {
			t.Fatal("acquired uncertain mutation")
		}
	} else {
		wait(func() bool {
			row, err := conversation.ReadOwnership(ctx, s.coexistence.store, owner.Conversation)
			return err == nil && row.State == "released"
		})
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestProcessResumeProbe$", "-test.v")
		cmd.Env = append(os.Environ(), "CERCANO_RESUME_PROBE_STORE="+s.coexistence.options.StorePath, "CERCANO_RESUME_PROBE_OLD="+owner.Epoch)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("replacement: %v: %s", err, out)
		}
		t.Logf("replacement: %s", out)
	}
}

// The replacement is a separate host process opening the same SQLite store, not
// an in-process capability fabricated by the source host.
func TestProcessResumeProbe(t *testing.T) {
	path := os.Getenv("CERCANO_RESUME_PROBE_STORE")
	if path == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	s, err := NewCoexistenceServer(ctx, CoexistenceOptions{StorePath: path, Host: "replacement", Build: "B", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.CloseCoexistenceStore()
	owner, err := s.Retain(ctx, "parent")
	if err != nil {
		t.Fatal(err)
	}
	if owner.Host != "replacement" || owner.Epoch == "" || owner.Epoch == os.Getenv("CERCANO_RESUME_PROBE_OLD") {
		t.Fatalf("replacement owner: %+v", owner)
	}
	row, err := conversation.ReadOwnership(ctx, s.GenerationStore(), "parent")
	if err != nil || row.Epoch != owner.Epoch || row.Owner != owner.Host {
		t.Fatalf("durable replacement: %+v %v", row, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM delegated_tasks WHERE id='one' AND status='completed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("task evidence: %d %v", count, err)
	}
	history, err := s.GenerationStore().ReadHistory(ctx, "parent", conversation.HistoryQuery{Query: "process close evidence", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) == 0 {
		t.Fatal("transcript lost")
	}
}
