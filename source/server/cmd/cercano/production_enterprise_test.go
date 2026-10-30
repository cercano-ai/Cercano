package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/internal/enterprise"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func enterpriseFixture(t *testing.T) (*productionEnterprise, string) {
	t.Helper()
	dir := t.TempDir()
	e, err := composeProductionEnterprise(productionEnterpriseDependencies{options: &enterprise.HostOptions{
		Directory: dir,
		OpenStore: func() (enterprise.CredentialStore, error) {
			t.Fatal("unexpected credential opener")
			return nil, errors.New("trap")
		},
	}}, enterpriseLegacy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	return e, dir
}

func TestProductionEnterpriseRejectsBeforeOpen(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	for _, policy := range []productionEnterprisePolicy{0, enterprisePreparedUnsupported} {
		e, err := composeProductionEnterprise(productionEnterpriseDependencies{options: &enterprise.HostOptions{
			OpenStore: func() (enterprise.CredentialStore, error) {
				t.Fatal("credential open before rejection")
				return nil, nil
			},
		}}, policy)
		if err == nil || e != nil {
			t.Fatal("unsupported policy accepted")
		}
		if policy == enterprisePreparedUnsupported && !errors.Is(err, errPreparedEnterpriseUnsupported) {
			t.Fatal(err)
		}
	}
	// Preserve legacy constructor errors, not a policy error or a credential open.
	_, err := composeProductionEnterprise(productionEnterpriseDependencies{options: &enterprise.HostOptions{}}, enterpriseLegacy)
	if err == nil || err.Error() != "enterprise state directory required" {
		t.Fatalf("legacy error: %v", err)
	}
}

type enterpriseTestStream struct{ grpc.ServerStream }

func (enterpriseTestStream) Context() context.Context { return context.Background() }

func TestProductionEnterpriseRegistrationAdmissionAndCleanup(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	a, dir := enterpriseFixture(t)
	b, _ := enterpriseFixture(t)
	s := grpc.NewServer(a.serverOptions()...)
	defer s.Stop()
	a.register(s)
	if _, ok := s.GetServiceInfo()[proto.Enterprise_ServiceDesc.ServiceName]; !ok {
		t.Fatal("enterprise RPC missing")
	}
	check := func(blocked bool) {
		t.Helper()
		_, finish, err := a.Begin(context.Background())
		if (err != nil) != blocked {
			t.Fatalf("background admission: %v", err)
		}
		if err == nil {
			finish()
		}
		called := false
		_, err = a.UnaryInterceptor()(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: proto.Agent_ProcessRequest_FullMethodName}, func(context.Context, any) (any, error) { called = true; return nil, nil })
		if called == blocked || (status.Code(err) == codes.PermissionDenied) != blocked {
			t.Fatalf("unary admission: %v, called %v", err, called)
		}
		called = false
		err = a.StreamInterceptor()(nil, enterpriseTestStream{}, &grpc.StreamServerInfo{FullMethod: proto.Agent_StreamProcessRequest_FullMethodName}, func(any, grpc.ServerStream) error { called = true; return nil })
		if called == blocked || (status.Code(err) == codes.PermissionDenied) != blocked {
			t.Fatalf("stream admission: %v, called %v", err, called)
		}
	}
	check(false)
	// Existing host notices activation by another owner and fails closed without
	// opening a competing credential store. No issuer or network is contacted.
	if err := os.WriteFile(filepath.Join(dir, "managed"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	check(true)
	a.Close()
	a.Close()
	_, finish, err := b.Begin(context.Background())
	if err != nil {
		t.Fatalf("sibling closed: %v", err)
	}
	finish()
}
