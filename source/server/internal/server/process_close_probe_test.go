package server

// This probe deliberately uses the production callback installation. In contrast
// to prepared_worker_mutation_test it never installs failure-evidence wrappers.
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/conversation"
	persistsvc "cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
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
	op := "create"
	if strings.HasPrefix(os.Getenv("CERCANO_CLOSE_PROBE_MODE"), "update-") {
		var err error
		raw, err = os.ReadFile(filepath.Join(os.Getenv("CERCANO_CLOSE_PROBE_ROOT"), "autonomy-run"))
		if err != nil {
			return err
		}
		op = "update"
	}
	request := &pb.WorkerToHost{Msg: &pb.WorkerToHost_AutonomyRequest{AutonomyRequest: &pb.AutonomyLedgerRequest{Id: 1, Op: op, RunJson: raw}}}
	if strings.HasPrefix(os.Getenv("CERCANO_CLOSE_PROBE_MODE"), "route-") {
		request = &pb.WorkerToHost{Msg: &pb.WorkerToHost_SessionModelRequest{SessionModelRequest: &pb.SessionModelRequest{Id: "1", ConversationId: "parent", Action: "set", Profile: "fixture", Model: "exact-model"}}}
	}
	if err := stream.Send(request); err != nil {
		return err
	}
	response, err := stream.Recv()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "callback response: %v\n", response)
	root := os.Getenv("CERCANO_CLOSE_PROBE_ROOT")
	if err := os.WriteFile(filepath.Join(root, "callback-returned"), []byte(response.String()), 0600); err != nil {
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
	for _, mode := range []string{"success", "write-failure", "update-success", "update-write-failure", "blocked-success", "route-success", "route-write-failure"} {
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
	cfg := config.Config{OpenRuntime: "synthetic"}
	cfg.CloudProfiles = []config.CloudProfile{{Name: "fixture", Provider: "deepinfra", Flavor: "chat_completions", Model: "default", BaseURL: "https://example.invalid/v1"}}
	s, owned, owner, _ := preparedWakeFixtureWithConfig(t, cfg)
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
	entered, unblock := make(chan struct{}), make(chan struct{})
	blocked := os.Getenv("CERCANO_CLOSE_PROBE_MODE") == "blocked-success"
	if blocked {
		s.persistSvc = closeProbePersistence{Service: s.persistSvc, store: closeProbeBlockedStore{Store: s.persistSvc.Store(), entered: entered, unblock: unblock}}
	}
	must(s.ConfigurePreparedWorker(policy))
	defer s.workerRunner.(interface{ CleanupOwnedWorkers() }).CleanupOwnedWorkers()
	mode := os.Getenv("CERCANO_CLOSE_PROBE_MODE")
	operation := "INSERT"
	table := "autonomy_runs"
	if strings.HasPrefix(mode, "route-") {
		t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
		table = "conversation_chat_routes"
		cfg := config.Defaults()
		cfg.CloudProfiles = []config.CloudProfile{{Name: "fixture", Provider: "deepinfra", Flavor: "chat_completions", Model: "default", BaseURL: "https://example.invalid/v1"}}
		s.SetConfigPersistence("", cfg)
		keys := secrets.NewMemory()
		must(keys.Set("fixture", "test-only"))
		s.SetSecrets(keys)
	}
	if strings.HasPrefix(mode, "update-") {
		run, err := s.persistSvc.Store().CreateAutonomyRun(owned, conversation.AutonomyRun{ConversationID: owner.Conversation, State: "running"})
		must(err)
		run.State = "completed"
		raw, err := json.Marshal(run)
		must(err)
		must(os.WriteFile(filepath.Join(root, "autonomy-run"), raw, 0600))
		operation = "UPDATE"
	}
	if strings.HasSuffix(os.Getenv("CERCANO_CLOSE_PROBE_MODE"), "write-failure") {
		db, err := sql.Open("sqlite", s.coexistence.options.StorePath)
		must(err)
		defer db.Close()
		_, err = db.Exec(fmt.Sprintf(`CREATE TRIGGER injected BEFORE %s ON %s BEGIN SELECT RAISE(ABORT, 'real callback failure'); END`, operation, table))
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
	if blocked {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("callback entry deadline")
		}
	} else {
		wait(func() bool { _, err := os.Stat(filepath.Join(root, "callback-returned")); return err == nil })
	}

	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
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
	served := make(chan struct{})
	go func() { defer close(served); _ = g.Serve(lis) }()
	defer func() { g.Stop(); <-served }()
	opts := append(auth.DialOptions(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(lis.Addr().String(), opts...)
	must(err)
	defer conn.Close()
	identity, err := pb.NewAgentClient(conn).GetDevelopmentHostIdentity(ctx, &pb.GetDevelopmentHostIdentityRequest{})
	must(err)
	pinned, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: lis.Addr().String(), HostInstance: owner.Host, Contract: identity.GetContract(), SchemaVersion: identity.GetSchemaVersion(), ConversationID: owner.Conversation, OwnershipEpoch: owner.Epoch}, token)
	must(err)
	defer pinned.Close()
	must(pinned.ReconnectPinnedSession(ctx))
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
	if blocked {
		close(unblock)
		wait(func() bool { _, err := os.Stat(filepath.Join(root, "callback-returned")); return err == nil })
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
	if strings.HasSuffix(os.Getenv("CERCANO_CLOSE_PROBE_MODE"), "write-failure") {
		snap := s.coexistence.admission.Snapshot(owner)
		if snap.UnresolvedWork == 0 {
			response, _ := os.ReadFile(filepath.Join(root, "callback-returned"))
			t.Logf("response: %s", response)
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
		db, err := sql.Open("sqlite", s.coexistence.options.StorePath)
		must(err)
		defer db.Close()
		var count int
		if strings.HasPrefix(mode, "route-") {
			must(db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_chat_routes WHERE conversation_id='parent' AND profile='fixture' AND model='exact-model'`).Scan(&count))
		} else if strings.HasPrefix(mode, "update-") {
			must(db.QueryRowContext(ctx, `SELECT COUNT(*) FROM autonomy_runs WHERE conversation_id='parent' AND state='completed'`).Scan(&count))
		} else {
			must(db.QueryRowContext(ctx, `SELECT COUNT(*) FROM autonomy_runs WHERE conversation_id='parent'`).Scan(&count))
		}
		if count != 1 {
			t.Fatalf("actual persisted callback rows = %d", count)
		}
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
		if err := pinned.ReconnectPinnedSession(ctx); !errors.Is(err, agentclient.ErrPinnedSession) {
			t.Fatalf("stale source binding accepted after replacement: %v", err)
		}

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
	root := t.TempDir()
	s, err := NewPreparedServer(ctx, PreparedServerOptions{
		Coexistence: CoexistenceOptions{StorePath: path, Host: "replacement", Build: "B", Contract: conversation.CurrentCoexistenceDeclaration(), Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }, CloseParticipation: func(context.Context) error { return nil }},
		Config:      config.Config{OpenRuntime: "synthetic"}, ConfigPath: filepath.Join(root, "config"), RoutingPath: filepath.Join(root, "routing"), FailurePath: filepath.Join(root, "failures"),
		Agent: func(st conversation.Store) (*agent.Agent, error) {
			return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
		},
	}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.CloseCoexistenceStore()
	defer s.Shutdown()
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
	token := filepath.Join(root, "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load(owner.Host, token)
	if err != nil {
		t.Fatal(err)
	}
	s.coexistence.options.ControlAuthorize = auth.Authorize
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer(auth.ServerOptions()...)
	pb.RegisterAgentServer(g, s)
	served := make(chan struct{})
	go func() { defer close(served); _ = g.Serve(lis) }()
	defer func() { g.Stop(); <-served }()
	id := s.coexistence.controlIdentity()
	client, err := agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: lis.Addr().String(), HostInstance: owner.Host, Contract: id.GetContract(), SchemaVersion: int32(conversation.CurrentCoexistenceDeclaration().SchemaVersion), ConversationID: owner.Conversation, OwnershipEpoch: row.Epoch}, token)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	turns, err := client.ResumeConversation(ctx, owner.Conversation)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, turn := range turns {
		if turn.Content == "process close evidence" {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit pinned resume lost transcript: %+v", turns)
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

// This barrier delays the actual installed ledger callback at its store call.
// It does not install mutation scopes or report failures on production's behalf.
type closeProbePersistence struct {
	persistsvc.Service
	store conversation.Store
}

func (p closeProbePersistence) Store() conversation.Store { return p.store }

type closeProbeBlockedStore struct {
	conversation.Store
	entered chan struct{}
	unblock <-chan struct{}
}

func (s closeProbeBlockedStore) CreateAutonomyRun(ctx context.Context, run conversation.AutonomyRun) (conversation.AutonomyRun, error) {
	close(s.entered)
	select {
	case <-s.unblock:
	case <-ctx.Done():
		return conversation.AutonomyRun{}, ctx.Err()
	}
	return s.Store.CreateAutonomyRun(ctx, run)
}
