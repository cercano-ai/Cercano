package agentclient

import (
	"context"
	"errors"
	"testing"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type sessionReleaseFake struct {
	proto.AgentClient
	request *proto.LifecycleControlRequest
	err     error
	pending bool
}

func (f *sessionReleaseFake) LifecycleRelease(_ context.Context, r *proto.LifecycleControlRequest, _ ...grpc.CallOption) (*proto.LifecycleControlResponse, error) {
	f.request = r
	return &proto.LifecycleControlResponse{Pending: f.pending}, f.err
}
func TestReleaseSessionUsesPinnedOwnerWithoutManagementOrCancellation(t *testing.T) {
	f := &sessionReleaseFake{}
	c := &Client{agent: f, pinned: &PinnedSessionIntent{HostInstance: "A", ConversationID: "conversation", OwnershipEpoch: "epoch"}}
	target := &proto.LifecycleIdentity{Host: "A", Build: "build"}
	if err := c.ReleaseSession(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	r := f.request
	if r == nil || r.Management || r.Target != target || r.Source != target || r.Owner.Conversation != "conversation" || r.Owner.Host != "A" || r.Owner.Epoch != "epoch" {
		t.Fatalf("wrong bound release: %+v", r)
	}
	// Any RPC other than LifecycleRelease panics through the nil embedded client.
	f.err = errors.New("release refused")
	if err := c.ReleaseSession(context.Background(), target); !errors.Is(err, f.err) {
		t.Fatalf("lost refusal: %v", err)
	}
	f.err = nil
	f.pending = true
	if err := c.ReleaseSession(context.Background(), target); err == nil {
		t.Fatal("pending release reported complete")
	}
}
func TestReleaseSessionUnpinnedNoop(t *testing.T) {
	if err := (&Client{}).ReleaseSession(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
