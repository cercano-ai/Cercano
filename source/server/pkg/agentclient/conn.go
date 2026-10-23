package agentclient

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Conn returns a connection for generated gRPC clients (proto.NewAgentClient,
// proto.NewEnterpriseClient, ...) that sends each call over the client's
// current connection. Calls made after a reconnect go to the recovered host,
// so a long-lived caller keeps working when the agent restarts.
func (c *Client) Conn() grpc.ClientConnInterface { return currentConn{c} }

type currentConn struct{ c *Client }

func (f currentConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	conn := f.c.readConn()
	if conn == nil {
		return status.Error(codes.Unavailable, "agentclient: client is closed")
	}
	return conn.Invoke(ctx, method, args, reply, opts...)
}

func (f currentConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	conn := f.c.readConn()
	if conn == nil {
		return nil, status.Error(codes.Unavailable, "agentclient: client is closed")
	}
	return conn.NewStream(ctx, desc, method, opts...)
}
