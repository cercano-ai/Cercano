// Package devhostauth provides small same-user development authentication on
// existing gRPC transports. It is not a boundary against a malicious same-user
// process, and must not be used on non-loopback TCP endpoints.
package devhostauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const tokenHeader = "x-cercano-development-token"
const hostHeader = "x-cercano-development-host"

// Auth is immutable and deliberately has no exported token or formatting API.
type Auth struct {
	host  string
	token string
}

func (*Auth) String() string   { return "development-host-auth(redacted)" }
func (*Auth) GoString() string { return "development-host-auth(redacted)" }

// Create writes a fresh token without replacing an existing host's token. The
// caller must supply a dedicated private fixture directory. Tokens never travel
// through argv or environment, including child environments.
func Create(path string) error {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(hex.EncodeToString(b))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Load refuses symlinks, non-regular files and permissions exposing the token.
// This check is hygiene, not protection from same-user filesystem races.
func Load(host, path string) (*Auth, error) {
	if host == "" {
		return nil, errors.New("exact development host instance required")
	}
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("development token must be a private regular file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(string(b))
	if err != nil || len(raw) != 32 {
		return nil, errors.New("invalid development token file")
	}
	return &Auth{host: host, token: string(b)}, nil
}

// Authorize must run before lifecycle controls as well as ordinary operations.
func (a *Auth) Authorize(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	tokens, hosts := md.Get(tokenHeader), md.Get(hostHeader)
	if a == nil || len(tokens) != 1 || len(hosts) != 1 || hosts[0] != a.host || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(a.token)) != 1 {
		return status.Error(codes.Unauthenticated, "development host authentication failed")
	}
	return nil
}

// ServerOptions guards every registered service, including streaming methods,
// before its handler runs. No identity, worker or lifecycle method is exempt.
func (a *Auth) ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			if err := a.Authorize(ctx); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}),
		grpc.ChainStreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, next grpc.StreamHandler) error {
			if err := a.Authorize(stream.Context()); err != nil {
				return err
			}
			return next(srv, stream)
		}),
	}
}
func (a *Auth) outgoing(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(tokenHeader, a.token)
	md.Set(hostHeader, a.host)
	return metadata.NewOutgoingContext(ctx, md)
}

// DialOptions binds every RPC, including transparent reconnects, to the exact
// selected host. It neither discovers nor starts any process.
func (a *Auth) DialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, next grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return next(a.outgoing(ctx), method, req, reply, cc, opts...)
		}),
		grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, next grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return next(a.outgoing(ctx), desc, cc, method, opts...)
		}),
	}
}
