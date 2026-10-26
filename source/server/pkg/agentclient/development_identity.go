package agentclient

import (
	"context"
	"errors"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// DevelopmentHostConnection is a read-only snapshot of the current transport.
// Equality detects replacement even when the dial endpoint is unchanged.
// It does not dial, launch, or trigger client recovery.
type DevelopmentHostConnection struct {
	Endpoint string
	conn     *grpc.ClientConn
}

func (c *Client) DevelopmentHostConnection() DevelopmentHostConnection {
	if c == nil {
		return DevelopmentHostConnection{}
	}
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return DevelopmentHostConnection{Endpoint: c.addr, conn: c.conn}
}

func (c *Client) ReadDevelopmentHostIdentity(ctx context.Context, connection DevelopmentHostConnection) (*proto.GetDevelopmentHostIdentityResponse, error) {
	if connection.conn == nil {
		return nil, errors.New("agent connection unavailable")
	}
	return proto.NewAgentClient(connection.conn).GetDevelopmentHostIdentity(ctx, &proto.GetDevelopmentHostIdentityRequest{})
}
