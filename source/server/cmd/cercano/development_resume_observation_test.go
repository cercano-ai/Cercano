package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/brewrestart"
	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/server"
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

// Real lifecycle handlers, private registry, token authentication, and production
// containment adapter. Only process inspection/stop/absence are disposable seams.
func TestDevelopmentResumeTimeoutConfirmedContinuation(t *testing.T) {
	state, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(state, "hosts"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}})
	path := filepath.Join(state, "owned.db")
	if err := conversation.PrepareCoexistence(ctx, path); err != nil {
		t.Fatal(err)
	}
	contract := conversation.CurrentCoexistenceDeclaration()
	id := func(host string) *pb.LifecycleIdentity {
		return &pb.LifecycleIdentity{Host: host, Build: "build-" + host, Store: path, Contract: contract.Contract, Schema: int32(contract.SchemaVersion)}
	}
	source, destination := id("A"), id("B")
	store, err := conversation.OpenCoexistence(ctx, path, contract)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cap, err := conversation.AcquireOwnership(ctx, store, "chat", "A")
	if err != nil {
		t.Fatal(err)
	}
	defer cap.Close()
	if err := store.EnsureConversation(conversation.WithOwnership(ctx, cap), "chat", "", ""); err != nil {
		t.Fatal(err)
	}
	generation := filepath.Join(state, "generation-A")
	base := filepath.Join(state, "hosts", fmt.Sprintf("%x", sha256.Sum256([]byte(generation))))
	token := base + ".token"
	if err := devhostauth.Create(token); err != nil {
		t.Fatal(err)
	}
	auth, err := devhostauth.Load("A", token)
	if err != nil {
		t.Fatal(err)
	}
	options := server.CoexistenceOptions{StorePath: path, Build: source.Build, Host: source.Host, Contract: contract, CloseParticipation: func(context.Context) error { return nil }, Reload: func(context.Context, conversation.Store, *conversation.Ownership) error { return nil }}
	a, err := server.NewCoexistenceServer(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseCoexistenceStore()
	transport := grpc.NewServer(auth.ServerOptions()...)
	pb.RegisterAgentServer(transport, a)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go transport.Serve(listener)
	defer transport.Stop()
	if err := launchExclusive(base+".json", launchSelection{Identity: source, Endpoint: listener.Addr().String(), TokenFile: token}); err != nil {
		t.Fatal(err)
	}
	if err := launchExclusive(base+".starting", map[string]string{"host": "A", "generation": generation}); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	adapter := newDevelopmentContainedSource(state, destination)
	process := brewrestart.Identity{PID: 42, StartSeconds: 1, Executable: filepath.Join(generation, "cercano")}
	adapter.inspect = func(context.Context, string, netip.AddrPort) (brewrestart.Identity, error) { return process, nil }
	stops := 0
	adapter.stop = func(ctx context.Context, _ string, _ netip.AddrPort, _ brewrestart.Identity, validate func() error) error {
		if err := validate(); err != nil {
			return err
		}
		stops++
		return cap.Close()
	}
	adapter.absent = func(context.Context, string) error { return nil }
	options.Host, options.Build = destination.Host, destination.Build
	options.ControlAuthorize = func(context.Context) error { return nil }
	options.ResolveSource = func(context.Context, string) (*pb.LifecycleIdentity, string, error) {
		return nil, "", context.DeadlineExceeded
	}
	options.RecoveryResolveSource = developmentSourceResolver(state, destination)
	options.ControlDial = developmentPeerDial(state, destination)
	options.RecoverySource = adapter
	options.RecoverySelection = func(context.Context, string) (*pb.LifecycleRecoverySelection, error) {
		return &pb.LifecycleRecoverySelection{Identity: destination, Endpoint: "127.0.0.1:12345", TokenFile: token}, nil
	}
	b, err := server.NewCoexistenceServer(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer b.CloseCoexistenceStore()
	request := &pb.LifecycleControlRequest{Target: destination, Requester: destination, Owner: &pb.LifecycleOwner{Conversation: "chat"}, DestinationEndpoint: "127.0.0.1:12345"}
	offer, err := b.LifecycleResume(ctx, request)
	if err != nil || !offer.GetConfirmationRequired() || stops != 0 {
		t.Fatalf("offer: %v %v stops=%d", offer, err, stops)
	}
	request.ConfirmOriginalRecovery = true
	request.RecoveryChallenge = offer.OriginalRecovery.Challenge
	result, err := b.LifecycleResume(ctx, request)
	if err != nil || !result.GetReady() || stops != 1 {
		t.Fatalf("continuation: %v %v stops=%d", result, err, stops)
	}
	if err := b.ReleaseRetained(ctx, broker.Owner{Conversation: result.Owner.Conversation, Host: result.Owner.Host, Epoch: result.Owner.Epoch}, nil); err != nil {
		t.Fatal(err)
	}
}
