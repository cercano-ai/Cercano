package main

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
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type developmentHostSelection struct {
	Endpoint     string
	TokenFile    string
	Identity     *proto.LifecycleIdentity
	Conversation string
	Epoch        string
}
type developmentHostOptions struct {
	State, Generation, Host, TokenFile, Record, RuntimeRoot, Listen string
	Identity                                                        *proto.LifecycleIdentity
}

// Factory injection is invocation-local, never an environment switch. Preflight
// and executable verification run before any dependency factory is entered.
type developmentHostFactory func(context.Context, developmentHostOptions, *devhostauth.Auth) (*server.Server, func(), error)

func runDevelopmentHost(ctx context.Context, args []string, out io.Writer) error {
	return runDevelopmentHostWithFactory(ctx, args, out, newProductionDevelopmentHost)
}
func developmentPrivatePath(p string, directory bool) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return fmt.Errorf("absolute canonical path required: %s", p)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return err
	}
	if resolved != p {
		return fmt.Errorf("symlink aliases refused: %s", p)
	}
	st, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if st.IsDir() != directory || (!directory && !st.Mode().IsRegular()) || st.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("private %s required", p)
	}
	return nil
}
func runDevelopmentHostWithFactory(ctx context.Context, args []string, out io.Writer, factory developmentHostFactory) error {
	var o developmentHostOptions
	fs := flag.NewFlagSet("development-host", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&o.State, "state", "", "absolute prepared private state")
	fs.StringVar(&o.Generation, "generation", "", "absolute immutable published generation")
	fs.StringVar(&o.Host, "host", "", "exact host instance")
	fs.StringVar(&o.TokenFile, "token-file", "", "absolute private token")
	fs.StringVar(&o.Record, "record", "", "absolute output record")
	fs.StringVar(&o.RuntimeRoot, "runtime-root", "", "short private runtime root")
	fs.StringVar(&o.Listen, "listen", "127.0.0.1:0", "ephemeral loopback listener")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || o.Listen != "127.0.0.1:0" || o.Host == "" || filepath.Base(o.Host) != o.Host || o.Host == "." || o.Host == ".." {
		return fmt.Errorf("exact host and ephemeral 127.0.0.1:0 required")
	}
	for _, p := range []string{o.State, o.RuntimeRoot} {
		if err := developmentPrivatePath(p, true); err != nil {
			return err
		}
	}
	if len(o.RuntimeRoot) > 38 {
		return fmt.Errorf("runtime-root exceeds 38 bytes")
	}
	if err := developmentPrivatePath(o.TokenFile, false); err != nil {
		return err
	}
	if !filepath.IsAbs(o.Record) || filepath.Clean(o.Record) != o.Record {
		return fmt.Errorf("absolute canonical record required")
	}
	if err := developmentPrivatePath(filepath.Dir(o.Record), true); err != nil {
		return err
	}
	if _, err := os.Lstat(o.Record); !os.IsNotExist(err) {
		return fmt.Errorf("record must not already exist")
	}
	if !filepath.IsAbs(o.Generation) || filepath.Clean(o.Generation) != o.Generation {
		return fmt.Errorf("absolute canonical generation required")
	}
	resolved, err := filepath.EvalSymlinks(o.Generation)
	if err != nil {
		return err
	}
	if resolved != o.Generation {
		return fmt.Errorf("generation aliases refused")
	}
	var manifest devHostManifest
	if err := readDevJSON(filepath.Join(o.Generation, "manifest.json"), &manifest); err != nil {
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
	if exe != filepath.Join(o.Generation, "cercano") {
		return fmt.Errorf("execute exact generation/cercano binary")
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(binary)
	if manifest.Build == "" || manifest.Binaries["cercano"].SHA256 != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("published executable digest mismatch")
	}
	database := filepath.Join(o.State, "conversations.db")
	for _, p := range []string{database, filepath.Join(o.State, "config.yaml")} {
		if err := developmentPrivatePath(p, false); err != nil {
			return err
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(database + suffix); err == nil {
			if err := developmentPrivatePath(database+suffix, false); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	contract := conversation.DevelopmentCoexistenceDeclaration()
	if err := conversation.PreflightCoexistence(ctx, database, contract); err != nil {
		return err
	}
	o.Identity = &proto.LifecycleIdentity{Build: manifest.Build, Host: o.Host, Store: database, Contract: contract.Contract, Schema: int32(contract.SchemaVersion)}
	auth, err := devhostauth.Load(o.Host, o.TokenFile)
	if err != nil {
		return err
	}
	srv, closeDependencies, err := factory(ctx, o, auth)
	if err != nil {
		return err
	}
	if closeDependencies != nil {
		defer closeDependencies()
	}
	if srv == nil {
		return fmt.Errorf("dependency factory returned no server")
	}
	defer srv.Shutdown()
	transport := grpc.NewServer(auth.ServerOptions()...)
	proto.RegisterAgentServer(transport, srv)
	defer transport.Stop()
	listener, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() { done <- transport.Serve(listener) }()
	endpoint := listener.Addr().String()
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := developmentDialIdentity(probe, endpoint, auth, o.Identity)
	if err != nil {
		return fmt.Errorf("authenticated readiness: %w", err)
	}
	conn.Close()
	record := developmentHostSelection{Endpoint: endpoint, TokenFile: o.TokenFile, Identity: o.Identity}
	if err := publishDevelopmentRecord(o.Record, record); err != nil {
		return err
	}
	fmt.Fprintf(out, "development-host ready %s build=%s host=%s\n", endpoint, manifest.Build, o.Host)
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		return err
	}
}
func developmentDialIdentity(ctx context.Context, endpoint string, auth *devhostauth.Auth, want *proto.LifecycleIdentity) (*grpc.ClientConn, error) {
	if !devLoopback(endpoint) || want == nil {
		return nil, fmt.Errorf("invalid peer identity or endpoint")
	}
	conn, err := grpc.NewClient(endpoint, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, auth.DialOptions()...)...)
	if err != nil {
		return nil, err
	}
	got, err := proto.NewAgentClient(conn).GetDevelopmentHostIdentity(ctx, &proto.GetDevelopmentHostIdentityRequest{})
	if err != nil {
		conn.Close()
		return nil, err
	}
	if got.HostInstance != want.Host || got.Build != want.Build || got.Contract != want.Contract || got.SchemaVersion != want.Schema || got.Retired {
		conn.Close()
		return nil, fmt.Errorf("authenticated peer identity mismatch")
	}
	if _, err := proto.NewAgentClient(conn).LifecycleList(ctx, &proto.LifecycleControlRequest{Target: want}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("exact lifecycle identity: %w", err)
	}
	return conn, nil
}
func developmentPeerDial(state string, own *proto.LifecycleIdentity) func(context.Context, string) (*grpc.ClientConn, error) {
	return func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		if own == nil {
			return nil, fmt.Errorf("local lifecycle identity required")
		}
		dir := filepath.Join(state, "hosts")
		if err := developmentPrivatePath(dir, true); err != nil {
			return nil, err
		}
		files, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			return nil, err
		}
		var selected *developmentHostSelection
		for _, p := range files {
			if err := developmentPrivatePath(p, false); err != nil {
				return nil, err
			}
			var r developmentHostSelection
			if err := readDevJSON(p, &r); err != nil {
				return nil, err
			}
			if r.Endpoint == endpoint {
				if selected != nil {
					return nil, fmt.Errorf("ambiguous peer endpoint")
				}
				selected = &r
			}
		}
		if selected == nil || selected.Identity == nil {
			return nil, fmt.Errorf("peer absent from private host records")
		}
		id := selected.Identity
		if id.Host == own.Host || id.Store != own.Store || id.Contract != own.Contract || id.Schema != own.Schema {
			return nil, fmt.Errorf("incompatible peer record")
		}
		// Tokens are selected by trusted records, never from the control request.
		if err := developmentPrivatePath(selected.TokenFile, false); err != nil {
			return nil, err
		}
		auth, err := devhostauth.Load(id.Host, selected.TokenFile)
		if err != nil {
			return nil, err
		}
		probe, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return developmentDialIdentity(probe, endpoint, auth, id)
	}
}
func publishDevelopmentRecord(path string, record developmentHostSelection) error {
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".host-record-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Link publishes atomically without overwriting another host's record.
	if err = os.Link(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
