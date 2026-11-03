package server

import (
	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"sync/atomic"
	"testing"
)

func TestDeadOwnerRecoveryRPC(t *testing.T) {
	ctx := context.Background()
	a, b, _ := lifecycleFixture(t)
	owner, err := a.Retain(ctx, "dead")
	if err != nil {
		t.Fatal(err)
	}
	cap := a.coexistence.owners["dead"]
	if err = a.coexistence.store.EnsureConversation(conversation.WithOwnership(ctx, cap), "dead", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = cap.Close(); err != nil {
		t.Fatal(err)
	} // process-loss model; no clean release
	b.coexistence.options.ControlAuthorize = func(context.Context) error { return nil }
	var dead atomic.Bool
	var exclusions atomic.Int64
	b.coexistence.options.RecoveryGuard = func(context.Context, string) (func(), error) {
		if !dead.Load() {
			return nil, fmt.Errorf("source may be alive")
		}
		exclusions.Add(1)
		return func() { exclusions.Add(-1) }, nil
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	g := grpc.NewServer()
	pb.RegisterAgentServer(g, b)
	go g.Serve(lis)
	defer g.Stop()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewAgentClient(conn)
	r := &pb.LifecycleRecoverRequest{Target: b.coexistence.controlIdentity(), Conversation: "dead"}
	if _, err = client.LifecycleRecover(ctx, r); err == nil {
		t.Fatal("unreachable treated as dead")
	}
	dead.Store(true)
	offer, err := client.LifecycleRecover(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if offer.Owner.Epoch != owner.Epoch || offer.Owner.Host != owner.Host {
		t.Fatal(offer)
	}
	before, err := conversation.ReadOwnership(ctx, b.coexistence.store, "dead")
	if err != nil {
		t.Fatal(err)
	}
	if before.Owner != owner.Host {
		t.Fatal("inspection mutated owner")
	}
	r.Confirm = true
	r.ExpectedOwner = &pb.LifecycleOwner{Conversation: "dead", Host: owner.Host, Epoch: "stale"}
	if _, err = client.LifecycleRecover(ctx, r); err == nil {
		t.Fatal("stale confirmation accepted")
	}
	r.ExpectedOwner = offer.Owner
	got, err := client.LifecycleRecover(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner.Host != b.coexistence.options.Host || got.Owner.Epoch == owner.Epoch {
		t.Fatal(got)
	}
	retry, err := client.LifecycleRecover(ctx, r)
	if err != nil || retry.GetOwner().GetEpoch() != got.Owner.Epoch {
		t.Fatalf("retry %+v %v", retry, err)
	}
	if exclusions.Load() != 0 {
		t.Fatal("launch exclusion leaked")
	}
	bound, err := agentclient.DialPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: lis.Addr().String(), HostInstance: got.Owner.Host, Build: b.coexistence.options.Build, Contract: b.coexistence.options.Contract.Contract, SchemaVersion: int32(b.coexistence.options.Contract.SchemaVersion), ConversationID: "dead", OwnershipEpoch: got.Owner.Epoch})
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if err = bound.ReconnectPinnedSession(ctx); err != nil {
		t.Fatal(err)
	}
	newcap := b.coexistence.owners["dead"]
	if err = b.coexistence.store.Append(conversation.WithOwnership(ctx, newcap), conversation.Turn{ConversationID: "dead", Role: "assistant", Content: "continued on B"}); err != nil {
		t.Fatal(err)
	}
	if err = a.coexistence.store.Rename(conversation.WithOwnership(ctx, cap), "dead", "stale"); err == nil {
		t.Fatal("stale owner retained authority")
	}
}
