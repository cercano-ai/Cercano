package agentclient

import (
	"cercano/source/server/pkg/proto"
	"context"
	"errors"
)

// ResetSetup uses only the connection admitted by the central connect policy.
func (c *Client) ResetSetup(ctx context.Context, req *proto.ResetSetupRequest) (*proto.ResetSetupResponse, error) {
	c.connMu.Lock()
	agent := c.agent
	c.connMu.Unlock()
	if agent == nil {
		return nil, errors.New("agent client is not connected")
	}
	return agent.ResetSetup(ctx, req)
}
