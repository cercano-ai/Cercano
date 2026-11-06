package main

import (
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	gproto "google.golang.org/protobuf/proto"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDevelopmentHostPreflightNeverConstructsDependencies(t *testing.T) {
	called := false
	factory := func(context.Context, developmentHostOptions, *devhostauth.Auth) (*server.Server, *preparedShutdown, error) {
		called = true
		t.Fatal("dependency/native boundary entered")
		return nil, nil, nil
	}
	for _, args := range [][]string{{}, {"--state", "relative"}, {"--host", "../escape"}, {"--listen", "0.0.0.0:0"}, {"--listen", "127.0.0.1:1234"}} {
		if err := runDevelopmentHostWithFactory(context.Background(), args, io.Discard, factory); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	if called {
		t.Fatal("factory called")
	}
}
func TestDevelopmentPrivatePath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(root, 0700)
	p := filepath.Join(root, "token")
	if err := os.WriteFile(p, []byte("not a real credential"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := developmentPrivatePath(p, false); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(p, alias); err != nil {
		t.Fatal(err)
	}
	if err := developmentPrivatePath(alias, false); err == nil {
		t.Fatal("accepted alias")
	}
	os.Chmod(p, 0644)
	if err := developmentPrivatePath(p, false); err == nil {
		t.Fatal("accepted public file")
	}
}
func TestDevelopmentRecordAtomicNoReplace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "host.json")
	record := developmentHostSelection{Endpoint: "127.0.0.1:1234", TokenFile: "/private/token"}
	if err := publishDevelopmentRecord(p, record); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	before, _ := os.ReadFile(p)
	if err := publishDevelopmentRecord(p, developmentHostSelection{Endpoint: "replacement"}); err == nil {
		t.Fatal("replaced existing record")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("changed existing record")
	}
	var read developmentHostSelection
	if err := readDevJSON(p, &read); err != nil {
		t.Fatal(err)
	}
	if read.Conversation != "" || read.Epoch != "" {
		t.Fatal("record contains initial ownership")
	}
}
func TestDevelopmentPeerRequiresPrivateRecord(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(root, 0700)
	if err := os.Mkdir(filepath.Join(root, "hosts"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := developmentPeerDial(root, nil)(context.Background(), "127.0.0.1:1", nil); err == nil {
		t.Fatal("dialed unregistered caller endpoint")
	}
}

type developmentIdentityFake struct {
	proto.UnimplementedAgentServer
	identity *proto.LifecycleIdentity
}

func (f *developmentIdentityFake) GetDevelopmentHostIdentity(context.Context, *proto.GetDevelopmentHostIdentityRequest) (*proto.GetDevelopmentHostIdentityResponse, error) {
	i := f.identity
	return &proto.GetDevelopmentHostIdentityResponse{HostInstance: i.Host, Build: i.Build, Contract: i.Contract, SchemaVersion: i.Schema}, nil
}
func (f *developmentIdentityFake) LifecycleList(_ context.Context, r *proto.LifecycleControlRequest) (*proto.LifecycleControlResponse, error) {
	if !gproto.Equal(r.Target, f.identity) {
		return nil, status.Error(codes.FailedPrecondition, "identity mismatch")
	}
	return &proto.LifecycleControlResponse{Identity: f.identity}, nil
}
func TestDevelopmentAuthenticatedIdentityIncludesStore(t *testing.T) {
	root := t.TempDir()
	token := filepath.Join(root, "token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("host", token)
	if err != nil {
		t.Fatal(err)
	}
	id := &proto.LifecycleIdentity{Host: "host", Build: "build", Contract: "contract", Schema: 2, Store: "/private/store"}
	srv := grpc.NewServer(auth.ServerOptions()...)
	defer srv.Stop()
	proto.RegisterAgentServer(srv, &developmentIdentityFake{identity: id})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go srv.Serve(listener)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := developmentDialIdentity(ctx, listener.Addr().String(), auth, id)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	wrong := gproto.Clone(id).(*proto.LifecycleIdentity)
	wrong.Store = "/other/store"
	if conn, err := developmentDialIdentity(ctx, listener.Addr().String(), auth, wrong); err == nil {
		conn.Close()
		t.Fatal("accepted wrong store")
	}
	otherToken := filepath.Join(root, "other-token")
	if err := devhostauth.Create(otherToken); err != nil {
		t.Fatal(err)
	}
	other, err := devhostauth.Load("host", otherToken)
	if err != nil {
		t.Fatal(err)
	}
	if conn, err := developmentDialIdentity(ctx, listener.Addr().String(), other, id); err == nil {
		conn.Close()
		t.Fatal("accepted wrong token")
	}
}

// Host publication and launcher selection retain only explicit fixture marking.
// Ordinary and legacy records need no permission flag.
func TestDevelopmentSelectionExplicitFixture(t *testing.T) {
	for _, fixture := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "host.json")
		if err := publishDevelopmentRecord(path, developmentHostSelection{Fixture: fixture}); err != nil {
			t.Fatal(err)
		}
		var selected launchSelection
		if err := launchRead(path, &selected); err != nil {
			t.Fatal(err)
		}
		if selected.Fixture != fixture {
			t.Fatal("launcher lost fixture marking")
		}
	}
}

func TestDevelopmentSourceResolverExactPrivateRecord(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "hosts")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	own := &proto.LifecycleIdentity{Host: "destination", Build: "new", Contract: "contract", Schema: 2, Store: "/private/store"}
	source := gproto.Clone(own).(*proto.LifecycleIdentity)
	source.Host, source.Build = "source", "old"
	record := developmentHostSelection{Identity: source, Endpoint: "127.0.0.1:1234"}
	if err := publishDevelopmentRecord(filepath.Join(dir, "source.json"), record); err != nil {
		t.Fatal(err)
	}
	resolver := developmentSourceResolver(root, own)
	got, endpoint, err := resolver(context.Background(), "source")
	if err != nil || !gproto.Equal(got, source) || endpoint != record.Endpoint {
		t.Fatalf("wrong source: %v %s %v", got, endpoint, err)
	}
	if _, _, err := resolver(context.Background(), "absent"); err == nil {
		t.Fatal("resolved absent source")
	}
	if err := publishDevelopmentRecord(filepath.Join(dir, "duplicate.json"), record); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolver(context.Background(), "source"); err == nil {
		t.Fatal("accepted ambiguous source")
	}
	if err := os.Remove(filepath.Join(dir, "duplicate.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "source.json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolver(context.Background(), "source"); err == nil {
		t.Fatal("accepted public source record")
	}
}

func TestDevelopmentExactSuccessorReusedEndpoint(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "hosts"), 0700); err != nil {
		t.Fatal(err)
	}
	own := &proto.LifecycleIdentity{Host: "local", Store: "/store", Contract: "contract", Schema: 2}
	live := gproto.Clone(own).(*proto.LifecycleIdentity)
	live.Host = "successor"
	token := filepath.Join(root, "live.token")
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load(live.Host, token)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(auth.ServerOptions()...)
	defer srv.Stop()
	proto.RegisterAgentServer(srv, &developmentIdentityFake{identity: live})
	go srv.Serve(listener)
	dead := gproto.Clone(live).(*proto.LifecycleIdentity)
	dead.Host = "predecessor"
	for name, id := range map[string]*proto.LifecycleIdentity{"dead": dead, "live": live} {
		if err := publishDevelopmentRecord(filepath.Join(root, "hosts", name+".json"), developmentHostSelection{Identity: id, Endpoint: listener.Addr().String(), TokenFile: token}); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, endpoint, err := developmentSourceResolver(root, own)(ctx, live.Host)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := developmentPeerDial(root, own)(ctx, endpoint, live)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	// The predecessor record points at the same socket and token, but its exact
	// host identity must still fail authentication rather than select the live peer.
	if conn, err := developmentPeerDial(root, own)(ctx, endpoint, dead); err == nil {
		conn.Close()
		t.Fatal("authenticated wrong host")
	}
	wrong := gproto.Clone(live).(*proto.LifecycleIdentity)
	wrong.Build = "wrong-build"
	if conn, err := developmentPeerDial(root, own)(ctx, endpoint, wrong); err == nil {
		conn.Close()
		t.Fatal("accepted changed exact identity")
	}
}
