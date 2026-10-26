package agentclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// PinnedSessionIntent names an existing retained session. It is never permission
// to acquire, attach, discover an executable, launch, or migrate to another host.
// Endpoint is selected by the caller, not discovered during retries.
type PinnedSessionIntent struct {
	Endpoint, HostInstance, Build, Contract string
	SchemaVersion                           int32
	ConversationID, OwnershipEpoch          string
}

var ErrPinnedSession = errors.New("pinned session validation failed; explicit resume required")

// SessionCheckError preserves the explicit lifecycle refusal for callers.
type SessionCheckError struct{ State proto.RetainedSessionState }

func (e *SessionCheckError) Error() string { return fmt.Sprintf("%v: %s", ErrPinnedSession, e.State) }
func (e *SessionCheckError) Unwrap() error { return ErrPinnedSession }

// These keys are binding metadata, not authority. Normal handlers must validate
// the exact owner and hold their own work/attachment tickets atomically.
const (
	PinnedHostMetadata         = "cercano-host-instance"
	PinnedConversationMetadata = "cercano-conversation-id"
	PinnedEpochMetadata        = "cercano-ownership-epoch"
)

// DialPinnedSession performs only TCP + read-only validation. No watcher launches
// or replaces hosts. Explicit reconnect (and ordinary internal retry) retains the
// same immutable intent and revalidates on a fresh single-use transport.
func DialPinnedSession(ctx context.Context, intent PinnedSessionIntent) (*Client, error) {
	if intent.Endpoint == "" || intent.HostInstance == "" || intent.Build == "" || intent.Contract == "" || intent.SchemaVersion <= 0 || intent.ConversationID == "" || intent.OwnershipEpoch == "" {
		return nil, fmt.Errorf("%w: incomplete intent", ErrPinnedSession)
	}
	c, err := connectIntent(ctx, intent.Endpoint, 2*time.Second, &intent)
	if err != nil {
		return nil, err
	}
	c.addr, c.pinned, c.stopWatch = intent.Endpoint, &intent, make(chan struct{})
	return c, nil
}

// ReconnectPinnedSession always revalidates, even if gRPC still reports Ready.
// Failure closes the old transport and cannot change the original intent.
func (c *Client) ReconnectPinnedSession(ctx context.Context) error {
	if c.pinned == nil {
		return fmt.Errorf("%w: not a pinned client", ErrPinnedSession)
	}
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()
	select {
	case <-c.stopWatch:
		return errClientClosed
	default:
	}
	if old := c.readConn(); old != nil {
		_ = old.Close()
	}
	fresh, err := connectIntent(ctx, c.pinned.Endpoint, 2*time.Second, c.pinned)
	if err != nil {
		c.setState(ConnStateFailed, 1, err)
		return err
	}
	select {
	case <-c.stopWatch:
		_ = fresh.Close()
		return errClientClosed
	default:
	}
	c.writeConn(fresh.conn, fresh.agent)
	c.setState(ConnStateConnected, 1, nil)
	return nil
}

func validatePinnedSession(ctx context.Context, agent proto.AgentClient, p PinnedSessionIntent) error {
	id, err := agent.GetDevelopmentHostIdentity(ctx, &proto.GetDevelopmentHostIdentityRequest{})
	if err != nil {
		return fmt.Errorf("%w: identity: %w", ErrPinnedSession, err)
	}
	if id.GetHostInstance() != p.HostInstance || id.GetBuild() != p.Build || id.GetContract() != p.Contract || id.GetSchemaVersion() != p.SchemaVersion {
		return fmt.Errorf("%w: host/build/store contract mismatch", ErrPinnedSession)
	}
	if id.GetRetired() {
		return &SessionCheckError{proto.RetainedSessionState_RETAINED_SESSION_STATE_RETIRED}
	}
	check, err := agent.CheckRetainedSession(ctx, &proto.CheckRetainedSessionRequest{HostInstance: p.HostInstance, ConversationId: p.ConversationID, OwnershipEpoch: p.OwnershipEpoch})
	if err != nil {
		return fmt.Errorf("%w: session: %w", ErrPinnedSession, err)
	}
	if check.GetState() != proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED {
		return &SessionCheckError{check.GetState()}
	}
	return nil
}

func pinnedMetadataOptions(p PinnedSessionIntent) []grpc.DialOption {
	bind := func(ctx context.Context) context.Context {
		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		md.Set(PinnedHostMetadata, p.HostInstance)
		md.Set(PinnedConversationMetadata, p.ConversationID)
		md.Set(PinnedEpochMetadata, p.OwnershipEpoch)
		return metadata.NewOutgoingContext(ctx, md)
	}
	return []grpc.DialOption{
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoke(bind(ctx), method, req, reply, cc, opts...)
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, stream grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return stream(bind(ctx), desc, cc, method, opts...)
		}),
	}
}
