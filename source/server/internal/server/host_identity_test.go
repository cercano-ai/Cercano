package server

import (
	"context"
	"testing"

	"cercano/source/server/pkg/proto"
)

func TestGetHostIdentityReportsProfile(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil, nil)
	srv.SetHostIdentity(HostIdentity{Profile: "work", PID: 4242, InstanceID: "abc123", Version: "1.2.3"})

	got, err := srv.GetHostIdentity(context.Background(), &proto.GetHostIdentityRequest{})
	if err != nil {
		t.Fatalf("GetHostIdentity: %v", err)
	}
	if got.GetProfile() != "work" || got.GetPid() != 4242 || got.GetInstanceId() != "abc123" || got.GetVersion() != "1.2.3" {
		t.Fatalf("GetHostIdentity = %+v, want profile work, pid 4242, instance abc123, version 1.2.3", got)
	}
}

// GetHostIdentity must answer during shutdown too: clients use it to tell a
// draining host from a different one, and it takes no lock that a turn holds.
func TestGetHostIdentityAnswersWhileShuttingDown(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil, nil)
	srv.SetHostIdentity(HostIdentity{Profile: "default", PID: 1, InstanceID: "i", Version: "v"})
	srv.BeginShutdown()
	got, err := srv.GetHostIdentity(context.Background(), &proto.GetHostIdentityRequest{})
	if err != nil {
		t.Fatalf("GetHostIdentity after BeginShutdown: %v", err)
	}
	if got.GetProfile() != "default" {
		t.Fatalf("profile = %q, want default", got.GetProfile())
	}
}
