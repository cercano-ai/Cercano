package main

// This deliberately bounded fixture host exposes durable lifecycle controls,
// not inference. It must never enter the ordinary agent startup graph.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/conversation"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/hostsvc/permissions"
	"cercano/source/server/internal/modelhost"
	runnersvc "cercano/source/server/internal/runner"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/server"
	"cercano/source/server/internal/worker"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type devHostPeer struct{ Endpoint, Host, TokenFile string }
type devHostManifest struct{ Build, SHA256 string }

func devLoopback(addr string) bool {
	h, _, err := net.SplitHostPort(addr)
	ip := net.ParseIP(h)
	return err == nil && ip != nil && ip.IsLoopback()
}

func runDevHost(ctx context.Context, args []string, out io.Writer) error {
	// Validate before even reading fixture configuration, let alone a backend.
	if err := secrets.ValidateEnvironment(); err != nil {
		return err
	}
	if os.Getenv(secrets.DisableEnv) != "1" {
		return fmt.Errorf("dev-host requires CERCANO_DISABLE_CREDENTIALS=1")
	}
	fs := flag.NewFlagSet("dev-host", flag.ContinueOnError)
	root := fs.String("fixture", "", "existing private fixture directory")
	addr := fs.String("listen", "", "explicit loopback TCP address")
	host := fs.String("host", "", "unique host instance")
	generation := fs.String("generation", "", "exact immutable generation directory containing cercano and manifest.json")
	tokenName := fs.String("token-file", "token", "private token basename inside fixture")
	peersName := fs.String("peers-file", "peers.json", "peer declaration basename inside fixture")
	modelDir := fs.String("modelhost", "", "explicit protected fixture modelhost record directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *host == "" || !devLoopback(*addr) {
		return fmt.Errorf("explicit host, loopback listen address and no positional arguments required")
	}
	for _, p := range []string{*root, *generation, *modelDir} {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("absolute fixture, generation and modelhost paths required")
		}
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return err
		}
		if resolved != filepath.Clean(p) {
			return fmt.Errorf("fixture paths must not be aliases")
		}
	}
	for _, name := range []string{*tokenName, *peersName} {
		if name == "." || name == ".." || filepath.Base(name) != name {
			return fmt.Errorf("token and peers must be fixture basenames")
		}
		info, err := os.Lstat(filepath.Join(*root, name))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("token and peers must be regular files")
		}
	}
	st, err := os.Stat(*root)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("fixture directory must be private")
	}
	// No ambient HOME/config/temp roots, even for downstream library helpers.
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "TMPDIR"} {
		p, err := filepath.EvalSymlinks(os.Getenv(name))
		if err != nil {
			return fmt.Errorf("%s must name an existing fixture directory", name)
		}
		rel, err := filepath.Rel(*root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("%s must be inside fixture", name)
		}
	}
	if rel, err := filepath.Rel(*root, *modelDir); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("modelhost must be an explicitly provisioned fixture participant inside fixture")
	}
	// Existing fixture inputs and SQLite sidecars must not redirect into user data.
	for _, name := range []string{"config.json", "peers.json", "conversations.db", "conversations.db-wal", "conversations.db-shm", "routing.jsonl", "failures.jsonl"} {
		p := filepath.Join(*root, name)
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fixture file must be regular, not an alias: %s", name)
		}
	}
	auth, err := devhostauth.Load(*host, filepath.Join(*root, *tokenName))
	if err != nil {
		return err
	}
	var manifest devHostManifest
	if err := readDevJSON(filepath.Join(*generation, "manifest.json"), &manifest); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	if exe != filepath.Join(*generation, "cercano") {
		return fmt.Errorf("execute the exact generation/cercano binary")
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(binary)
	if manifest.Build == "" || manifest.SHA256 != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("generation manifest build and executable SHA256 required")
	}
	var cfg config.Config
	if err := readDevJSON(filepath.Join(*root, "config.json"), &cfg); err != nil {
		return err
	}
	cfg.WorkerIdleTimeoutSeconds = -1
	cfg = cfg.WithSharedReadOnly()
	var peers []devHostPeer
	if err := readDevJSON(filepath.Join(*root, *peersName), &peers); err != nil {
		return err
	}
	peerAuth := map[string]*devhostauth.Auth{}
	for _, p := range peers {
		if !devLoopback(p.Endpoint) || p.Host == *host || peerAuth[p.Endpoint] != nil {
			return fmt.Errorf("invalid explicit peer")
		}
		a, e := devhostauth.Load(p.Host, p.TokenFile)
		if e != nil {
			return e
		}
		peerAuth[p.Endpoint] = a
	}
	database := filepath.Join(*root, "conversations.db")
	contract := conversation.CurrentCoexistenceDeclaration()
	if err := conversation.PreflightCoexistence(ctx, database, contract); err != nil {
		return err
	}
	// A fresh namespace prevents host lifetime reuse. No legacy reaper is called.
	spawnPolicy, err := worker.NewSpawnPolicy(worker.SpawnOptions{GenerationDir: *generation, ManifestPath: filepath.Join(*generation, "manifest.json"), HostInstance: *host, ResourceRoot: *root})
	if err != nil {
		return err
	}
	// Freeze the explicitly selected record in a private namespace. Reconnect can
	// only re-verify this record, never discover a replacement in the source dir.
	record, err := modelhost.ReadRecord(*modelDir)
	if err != nil {
		return err
	}
	pinned, err := os.MkdirTemp(*root, "modelhost-pin-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(pinned)
	if err := modelhost.WriteRecord(pinned, record); err != nil {
		return err
	}
	remote, err := modelhost.NewRemote(ctx, modelhost.RemoteOptions{CoexistenceProtection: true, ClientID: *host, Profile: "dev-host-fixture", Ensure: modelhost.EnsureOptions{RunDir: pinned, ClientVersion: manifest.Build, CoexistenceProtection: true}})
	if err != nil {
		return err
	}
	defer remote.Close()
	// An in-memory rejecting boundary: no native credential opener is invoked.
	secretStore := secrets.Guard(secrets.NewMemory(), secrets.Options{ReadOnly: true})
	var preparedStore conversation.Store
	srv, err := server.NewPreparedDurableServer(ctx, server.PreparedServerOptions{
		Coexistence: server.CoexistenceOptions{StorePath: database, Build: manifest.Build, Host: *host, Contract: contract,
			ControlAuthorize: auth.Authorize,
			ControlDial: func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
				a := peerAuth[endpoint]
				if a == nil {
					return nil, fmt.Errorf("peer not explicitly selected")
				}
				return grpc.NewClient(endpoint, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, a.DialOptions()...)...)
			},
			CloseParticipation: func(context.Context) error { return remote.Close() },
		},
		WorkerRunner: func(history runnersvc.TurnHistory, frozen cfgsvc.Service, perms permissions.Broker) runnersvc.TurnRunner {
			// Execution remains gated; no provider or child-launch callbacks are installed.
			wr := worker.NewWorkerRunnerWithPolicy(spawnPolicy, history, frozen, perms, nil, nil, nil, nil, nil)
			configureDevHostWorker(wr.(worker.ProfileReaderSetter))
			return wr
		},
		Config: cfg, ConfigPath: filepath.Join(*root, "config.json"), Secrets: secretStore,
		RoutingPath: filepath.Join(*root, "routing.jsonl"), FailurePath: filepath.Join(*root, "failures.jsonl"),
		Agent: func(st conversation.Store) (*agent.Agent, error) {
			preparedStore = st
			return agent.NewAgent(nil, nil, agent.WithPersistentStore(st)), nil
		},
	}, nil, nil, nil, nil)
	if err != nil {
		return err
	}
	configureDevHostExecution(srv)
	srv.SetBuildVersion(manifest.Build)
	defer preparedStore.Close()
	defer srv.Shutdown()
	// Worker/provider execution remains disabled; lifecycle reload is now durable.
	// Never call legacy SelectExecutionMode or install background generators.
	opts := auth.ServerOptions()
	opts = append(opts, grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		switch filepath.Base(info.FullMethod) {
		case "GetCredentialPolicy", "GetDevelopmentHostIdentity", "LifecycleList", "LifecycleRetire", "LifecycleAcquire", "LifecyclePrepare", "LifecycleValidateReceipt", "LifecycleRelinquish", "LifecycleClaim", "LifecycleRelease", "ResumeConversation", "CheckRetainedSession",
			"GetConfig", "ListConversations", "GetConversation", "GetConversationTurns", "ListSubAgents", "GetSessionProfile", "GetContextUsage", "GetTokenMetrics", "GetCompactionState", "GetToolCall":
			return next(ctx, req)
		default:
			return nil, status.Error(codes.FailedPrecondition, "dev-host startup fixture: execution disabled")
		}
	}), grpc.ChainStreamInterceptor(func(s any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		if devHostExecutionAllowed(info.FullMethod) {
			return next(s, stream)
		}
		return status.Error(codes.FailedPrecondition, "dev-host startup fixture: execution disabled")
	}))
	transport := grpc.NewServer(opts...)
	proto.RegisterAgentServer(transport, srv)
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer transport.Stop()
	fmt.Fprintf(out, "dev-host ready %s build=%s host=%s\n", listener.Addr(), manifest.Build, *host)
	done := make(chan error, 1)
	go func() { done <- transport.Serve(listener) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}

func readDevJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
