package agentclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
)

// Generated clients built on Conn send every call over the client's current
// connection, so they keep working after reconnect swaps it.
func TestConnFollowsReconnect(t *testing.T) {
	a := mustStartTestHost(t, "work", "instance-a")
	b := mustStartTestHost(t, "work", "instance-b")
	ctx := context.Background()
	c, err := connect(ctx, a.addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	agent := proto.NewAgentClient(c.Conn())

	id, err := agent.GetHostIdentity(ctx, &proto.GetHostIdentityRequest{})
	if err != nil || id.GetInstanceId() != "instance-a" {
		t.Fatalf("before reconnect: id=%v err=%v, want instance-a", id, err)
	}
	fresh, err := connect(ctx, b.addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.writeConn(fresh.conn, fresh.agent, b.addr)
	id, err = agent.GetHostIdentity(ctx, &proto.GetHostIdentityRequest{})
	if err != nil || id.GetInstanceId() != "instance-b" {
		t.Fatalf("after reconnect: id=%v err=%v, want instance-b", id, err)
	}
}

func TestDialProfileExistingNeverLaunches(t *testing.T) {
	isolateProfileHome(t)
	launches := 0
	useLaunchFunc(t, func(string, string) (<-chan struct{}, error) {
		launches++
		return nil, errors.New("launch not expected")
	})

	if _, err := DialProfileExisting(context.Background(), "work"); !errors.Is(err, ErrNoProfileAgent) {
		t.Fatalf("no agent: err = %v, want ErrNoProfileAgent", err)
	}
	h := mustStartTestHost(t, "work", newTestInstanceID())
	if err := writeTestRecord("work", "work", h.addr, h.instance); err != nil {
		t.Fatal(err)
	}
	c, err := DialProfileExisting(context.Background(), "work")
	if err != nil {
		t.Fatalf("running agent: %v", err)
	}
	defer c.Close()
	if c.Profile() != "work" || identityOf(t, c).GetInstanceId() != h.instance {
		t.Errorf("connected to the wrong host")
	}
	if launches != 0 {
		t.Fatalf("launches = %d, want 0", launches)
	}
}

// A launch failure reads the same way as Dial's, naming the profile.
func TestDialProfileLaunchFailureSaysCouldNotAutoLaunch(t *testing.T) {
	isolateProfileHome(t)
	useLaunchFunc(t, func(string, string) (<-chan struct{}, error) {
		return nil, errors.New("`cercano` binary not found")
	})
	_, err := DialProfile(context.Background(), "work")
	if err == nil || !strings.Contains(err.Error(), "could not auto-launch") || !strings.Contains(err.Error(), `"work"`) {
		t.Fatalf("err = %v, want it to say it could not auto-launch the agent for \"work\"", err)
	}
}

// A client from DialProfileExisting may re-resolve the profile's running
// agent when its connection drops, but never launches one.
func TestDialProfileExistingReconnectNeverLaunches(t *testing.T) {
	isolateProfileHome(t)
	launches := 0
	useLaunchFunc(t, func(string, string) (<-chan struct{}, error) {
		launches++
		return nil, errors.New("launch not expected")
	})
	h := mustStartTestHost(t, "work", newTestInstanceID())
	if err := writeTestRecord("work", "work", h.addr, h.instance); err != nil {
		t.Fatal(err)
	}
	c, err := DialProfileExisting(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	h.srv.Stop()

	// Attempt 1 is on the respawn schedule.
	if !shouldRespawn(1) {
		t.Fatal("test assumes attempt 1 may respawn")
	}
	if _, err := c.redial(context.Background(), 1); err == nil {
		t.Fatal("redial succeeded with no agent running")
	}
	if launches != 0 {
		t.Fatalf("launches = %d, want 0", launches)
	}

	// A new agent for the profile is still picked up.
	h2 := mustStartTestHost(t, "work", newTestInstanceID())
	if err := writeTestRecord("work", "work", h2.addr, h2.instance); err != nil {
		t.Fatal(err)
	}
	fresh, err := c.redial(context.Background(), 1)
	if err != nil {
		t.Fatalf("redial to the new agent: %v", err)
	}
	defer fresh.Close()
	if fresh.addr != h2.addr {
		t.Fatalf("redial reached %s, want %s", fresh.addr, h2.addr)
	}
}
