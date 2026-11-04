package agentclient

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func remoteFixture(t *testing.T, addr string) (string, func()) {
	t.Helper()
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	healthpb.RegisterHealthServer(s, health.NewServer())
	go s.Serve(l)
	return l.Addr().String(), s.Stop
}

func checkRemoteHealth(t *testing.T, c *Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := healthpb.NewHealthClient(c.readConn()).Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestDialExistingConnectOnlyLoopbackRecovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	old := launchServerProcess
	var launches atomic.Int32
	launchServerProcess = func(string) (string, error) { launches.Add(1); return "", errors.New("unexpected local launch") }
	defer func() { launchServerProcess = old }()
	addr, stop := remoteFixture(t, "127.0.0.1:0")
	c, err := DialExisting(context.Background(), addr)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	defer c.Close()
	if c.addr != addr || !c.connectOnly || c.AutoLaunched {
		t.Fatal("lost connect-only selection")
	}
	checkRemoteHealth(t, c)
	stop()
	// Observe actual background recovery, including one unavailable redial.
	deadline := time.Now().Add(5 * time.Second)
	for c.State() != ConnStateReconnecting && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if c.State() != ConnStateReconnecting {
		t.Fatal("watcher did not detect lost endpoint")
	}
	time.Sleep(3500 * time.Millisecond)
	if launches.Load() != 0 {
		t.Fatal("remote recovery attempted local spawn")
	}
	_, stop = remoteFixture(t, addr)
	defer stop()
	deadline = time.Now().Add(15 * time.Second)
	for c.State() != ConnStateConnected && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if c.State() != ConnStateConnected {
		t.Fatal("watcher did not recover existing endpoint")
	}
	ctx := context.Background()
	checkRemoteHealth(t, c)
	if launches.Load() != 0 {
		t.Fatal("recovery launched locally")
	}
	if _, _, err := c.launchReplacement(ctx, func() (string, bool, error) { t.Fatal("replacement callback reached"); return "", false, nil }); err == nil {
		t.Fatal("replacement guard absent")
	}
}

func TestUnavailableExistingNeverLaunchesAndDefaultStillLaunches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	old := launchServerProcess
	calls := 0
	launchServerProcess = func(string) (string, error) { calls++; return "", errors.New("fixture: no local server installed") }
	defer func() { launchServerProcess = old }()
	if _, err := DialExisting(context.Background(), addr); err == nil {
		t.Fatal("missing endpoint succeeded")
	}
	if calls != 0 {
		t.Fatal("connect-only launched")
	}
	if _, err := Dial(context.Background(), addr); err == nil {
		t.Fatal("default missing-server launch unexpectedly succeeded")
	}
	if calls != 1 {
		t.Fatalf("default local auto-launch calls=%d", calls)
	}
}
