package devhostauth

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	pb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type probe struct {
	pb.UnimplementedHealthServer
	effects atomic.Int32
}

func (p *probe) Check(context.Context, *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	p.effects.Add(1)
	return &pb.HealthCheckResponse{}, nil
}
func (p *probe) Watch(_ *pb.HealthCheckRequest, s pb.Health_WatchServer) error {
	p.effects.Add(1)
	return s.Send(&pb.HealthCheckResponse{})
}

func TestAuthenticationBeforeUnaryAndStreamEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := Create(path); err != nil {
		t.Fatal(err)
	}
	auth, err := Load("host-A", path)
	if err != nil {
		t.Fatal(err)
	}
	wrongHost, err := Load("host-B", path)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "token")
	if err := Create(other); err != nil {
		t.Fatal(err)
	}
	wrongToken, err := Load("host-A", other)
	if err != nil {
		t.Fatal(err)
	}
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer(auth.ServerOptions()...)
	p := &probe{}
	pb.RegisterHealthServer(srv, p)
	go srv.Serve(lis)
	defer srv.Stop()
	for _, tc := range []struct {
		name string
		auth *Auth
		want codes.Code
	}{{"missing", nil, codes.Unauthenticated}, {"wrong-token", wrongToken, codes.Unauthenticated}, {"wrong-host", wrongHost, codes.Unauthenticated}, {"valid", auth, codes.OK}} {
		t.Run(tc.name, func(t *testing.T) {
			opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() })}
			if tc.auth != nil {
				opts = append(opts, tc.auth.DialOptions()...)
			}
			conn, err := grpc.NewClient("passthrough:///fixture", opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			before := p.effects.Load()
			client := pb.NewHealthClient(conn)
			_, err = client.Check(ctx, &pb.HealthCheckRequest{})
			if status.Code(err) != tc.want {
				t.Fatalf("unary: %v", err)
			}
			stream, err := client.Watch(ctx, &pb.HealthCheckRequest{})
			if err == nil {
				_, err = stream.Recv()
			}
			if status.Code(err) != tc.want {
				t.Fatalf("stream: %v", err)
			}
			want := int32(0)
			if tc.want == codes.OK {
				want = 2
			}
			if got := p.effects.Load() - before; got != want {
				t.Fatalf("handler effects = %d, want %d", got, want)
			}
		})
	}
}
func TestPrivateTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := Create(path); err != nil {
		t.Fatal(err)
	}
	if err := Create(path); err == nil {
		t.Fatal("replaced token")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("A", path); err == nil {
		t.Fatal("accepted exposed token")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := path + "-link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("A", link); err == nil {
		t.Fatal("accepted symlink")
	}
}
