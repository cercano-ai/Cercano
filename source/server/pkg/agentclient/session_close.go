package agentclient

import (
	"cercano/source/server/pkg/proto"
	"context"
	"fmt"
)

// ReleaseSession intentionally relinquishes this terminal's pinned ownership.
// Close remains transport-only: connection loss must preserve resumability.
func (c *Client) ReleaseSession(ctx context.Context, target *proto.LifecycleIdentity) error {
	if c == nil || c.pinned == nil {
		return nil
	}
	p := c.pinned
	c.connMu.Lock()
	agent := c.agent
	c.connMu.Unlock()
	response, err := agent.LifecycleRelease(ctx, &proto.LifecycleControlRequest{
		Target: target, Source: target,
		Owner: &proto.LifecycleOwner{Conversation: p.ConversationID, Host: p.HostInstance, Epoch: p.OwnershipEpoch},
	})
	if err != nil {
		return err
	}
	if response == nil || response.Pending {
		return fmt.Errorf("session release incomplete")
	}
	return nil
}
