package agentclient

import (
	"context"
	"fmt"

	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

// RetryTransfer explicitly retries only the supplied original transfer. Both
// clients MUST have been dialed with authentication pinned to r.Source and
// r.Destination respectively. This function does not discover or dial hosts.
// Unimplemented (including old servers) is returned without any fallback.
// A deadline/cancellation is an uncertain outcome, not permission to start a
// new move. Retain the unchanged request and retry only this operation.
func (c *LifecycleControls) RetryTransfer(ctx context.Context, destination *LifecycleControls, r *pb.LifecycleControlRequest) (*pb.LifecycleControlResponse, error) {
	if c == nil || destination == nil || c.AgentClient == nil || destination.AgentClient == nil || r == nil || !r.Management || !proto.Equal(r.Target, r.Source) || r.GetOwner().GetConversation() == "" || r.GetOwner().GetEpoch() == "" || r.GetOwner().GetHost() != r.GetSource().GetHost() || r.GetSource().GetHost() == "" || r.GetSource().GetBuild() == "" || r.GetDestination().GetHost() == "" || r.GetDestination().GetBuild() == "" || r.Source.Host == r.Destination.Host || r.HandoverId == "" || r.Receipt == "" {
		return nil, fmt.Errorf("exact original source, owner epoch, destination, handover ID and receipt required")
	}
	source := proto.Clone(r).(*pb.LifecycleControlRequest)
	target := proto.Clone(r).(*pb.LifecycleControlRequest)
	target.Target = target.Destination
	proof, err := destination.LifecycleRetryReceipt(ctx, target)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(proof.GetIdentity(), r.Destination) || proof.GetReceipt() != r.Receipt {
		return nil, fmt.Errorf("destination retry identity/receipt mismatch")
	}
	released, err := c.LifecycleRetryRelinquish(ctx, source)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(released.GetIdentity(), r.Source) || released.GetReceipt() != r.Receipt {
		return nil, fmt.Errorf("source retry identity/receipt mismatch; outcome uncertain")
	}
	result, err := destination.LifecycleRetryClaim(ctx, target)
	if err != nil {
		return nil, err
	}
	if !proto.Equal(result.GetIdentity(), r.Destination) || result.GetReceipt() != r.Receipt || result.GetOwner().GetConversation() != r.Owner.Conversation || result.GetOwner().GetHost() != r.Destination.Host || result.GetOwner().GetEpoch() == "" {
		return nil, fmt.Errorf("destination retry result mismatch; outcome uncertain")
	}
	return result, nil
}
