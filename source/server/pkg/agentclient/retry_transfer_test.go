package agentclient

import (
	"context"
	"testing"

	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type retryOnlyClient struct {
	pb.AgentClient
	calls   []string
	fail    string
	request *pb.LifecycleControlRequest
}

func (c *retryOnlyClient) reply(name string, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	c.calls = append(c.calls, name)
	if name == c.fail {
		return nil, status.Error(codes.Unimplemented, "old server")
	}
	if r.HandoverId != c.request.HandoverId || r.Receipt != c.request.Receipt || !proto.Equal(r.Owner, c.request.Owner) {
		panic("operation changed")
	}
	response := &pb.LifecycleControlResponse{Identity: r.Target, Receipt: r.Receipt}
	if name == "claim" {
		response.Owner = &pb.LifecycleOwner{Conversation: r.Owner.Conversation, Host: r.Destination.Host, Epoch: "result"}
		response.CurrentOwner = true
	}
	return response, nil
}
func (c *retryOnlyClient) LifecycleRetryReceipt(_ context.Context, r *pb.LifecycleControlRequest, _ ...grpc.CallOption) (*pb.LifecycleControlResponse, error) {
	return c.reply("receipt", r)
}
func (c *retryOnlyClient) LifecycleRetryRelinquish(_ context.Context, r *pb.LifecycleControlRequest, _ ...grpc.CallOption) (*pb.LifecycleControlResponse, error) {
	return c.reply("relinquish", r)
}
func (c *retryOnlyClient) LifecycleRetryClaim(_ context.Context, r *pb.LifecycleControlRequest, _ ...grpc.CallOption) (*pb.LifecycleControlResponse, error) {
	return c.reply("claim", r)
}
func TestRetryTransferOnly(t *testing.T) {
	for _, fail := range []string{"", "receipt", "relinquish", "claim"} {
		t.Run("unsupported-"+fail, func(t *testing.T) {
			a := &pb.LifecycleIdentity{Host: "A", Build: "a"}
			b := &pb.LifecycleIdentity{Host: "B", Build: "b"}
			r := &pb.LifecycleControlRequest{Target: a, Source: a, Destination: b, Owner: &pb.LifecycleOwner{Conversation: "chat", Host: "A", Epoch: "epoch"}, HandoverId: "original", Receipt: "receipt", Management: true, DestinationEndpoint: "127.0.0.1:1234"}
			before := proto.Clone(r)
			src := &retryOnlyClient{fail: fail, request: r}
			dst := &retryOnlyClient{fail: fail, request: r}
			got, err := (&LifecycleControls{AgentClient: src}).RetryTransfer(context.Background(), &LifecycleControls{AgentClient: dst}, r)
			if fail != "" && status.Code(err) != codes.Unimplemented {
				t.Fatalf("must fail closed: %v", err)
			}
			if fail == "" && (err != nil || !got.GetCurrentOwner()) {
				t.Fatalf("%v %v", got, err)
			}
			if !proto.Equal(r, before) {
				t.Fatal("caller operation mutated")
			}
			if fail == "receipt" && len(src.calls) != 0 {
				t.Fatal("source mutated before destination capability check")
			}
			if fail == "relinquish" && len(dst.calls) != 1 {
				t.Fatal("claim after failure")
			}
			// Any normal mutating RPC would panic through the nil embedded interface.
		})
	}
}
