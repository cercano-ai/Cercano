package agentclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPinnedIdleHostExit(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	f := &pinnedFake{host: "A"}
	f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED))
	s, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.Stop()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for c.State() != ConnStateFailed {
		select {
		case <-deadline:
			t.Fatalf("idle transport loss not detected: %v", c.State())
		case <-tick.C:
		}
	}
	if f.requests.Load() != 0 {
		t.Fatal("request replay")
	}
}

func TestPinnedIdleValidationBarrier(t *testing.T) {
	for _, mode := range []string{"valid", "stale", "retired", "auth", "denied", "close"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
			f := &pinnedFake{host: "A", validationStarted: make(chan struct{}), validationRelease: make(chan struct{})}
			if mode == "auth" {
				f.validationCode = codes.Unauthenticated
			}
			if mode == "denied" {
				f.validationCode = codes.PermissionDenied
			}
			f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED))
			_, addr := pinnedHost(t, "127.0.0.1:0", f)
			c, err := DialPinnedSession(context.Background(), pinAt(addr))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			events, unsub := c.ConnStateChanges()
			defer unsub()
			failed := c.readConn()
			failed.Close()
			select {
			case <-f.validationStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("no idle recovery")
			}
			if c.State() != ConnStateReconnecting {
				t.Fatal("connected before validation")
			}
			// A second detector shares the in-flight generation, never a second dial.
			result := make(chan error, 1)
			go func() { result <- c.reconnectPinnedTransport(context.Background(), failed) }()
			if mode == "close" {
				c.Close()
			} else {
				if mode == "stale" {
					f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_STALE))
				}
				if mode == "retired" {
					f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETIRED))
				}
				close(f.validationRelease)
			}
			select {
			case err = <-result:
			case <-time.After(5 * time.Second):
				t.Fatal("recovery did not stop")
			}
			if mode == "valid" && err != nil {
				t.Fatal(err)
			}
			if mode != "valid" && mode != "close" && err == nil {
				t.Fatal("refusal lost")
			}
			if mode == "auth" && status.Code(err) != codes.Unauthenticated {
				t.Fatalf("auth identity lost: %v", err)
			}
			if mode == "denied" && status.Code(err) != codes.PermissionDenied {
				t.Fatalf("policy identity lost: %v", err)
			}
			if mode != "close" {
				want := ConnStateConnected
				if mode != "valid" {
					want = ConnStateFailed
				}
				for {
					select {
					case e := <-events:
						if e.State == want {
							goto checked
						}
					case <-time.After(5 * time.Second):
						t.Fatal("missing validated state")
					}
				}
			}
		checked:
			if mode == "close" && c.State() == ConnStateConnected {
				t.Fatal("late install")
			}
			if mode != "valid" && mode != "close" {
				// A real new user action must not blindly retry auth/lifecycle refusals.
				for i := 0; i < 5; i++ {
					if _, demandErr := c.StreamChat(context.Background(), "conversation", "retry", ""); demandErr == nil {
						t.Fatal("demand bypassed refusal")
					}
				}
			}
			if f.checks.Load() != 2 || f.requests.Load() != 0 {
				t.Fatalf("checks=%d requests=%d", f.checks.Load(), f.requests.Load())
			}
		})
	}
}

func TestConnectionFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		cause error
		text  string
	}{
		{status.Error(codes.Unavailable, "offline"), "unreachable"},
		{status.Error(codes.Unauthenticated, "token"), "Authentication failed"},
		{status.Error(codes.PermissionDenied, "policy"), "Access refused"},
		{&SessionCheckError{proto.RetainedSessionState_RETAINED_SESSION_STATE_RETIRED}, "reopen/resume"},
		{ErrCredentialIsolation, "credential isolation policy"},
	} {
		err := fmt.Errorf("diagnostic: %w", tc.cause)
		if !strings.Contains(ConnectionFailureMessage(err), tc.text) {
			t.Fatalf("%v: %s", err, ConnectionFailureMessage(err))
		}
		if !errors.Is(err, tc.cause) {
			t.Fatal("identity lost")
		}
	}
}

// A failed idle attempt is cached for observers, not for a later user send.
func TestPinnedDemandAfterFailedIdleRecovery(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	f := &pinnedFake{host: "A"}
	f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED))
	s, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for c.State() != ConnStateFailed {
		if time.Now().After(deadline) {
			t.Fatal("no failure")
		}
		time.Sleep(time.Millisecond)
	}
	// Synchronize with publication of the cached attempt outcome.
	if err := c.reconnectPinnedTransport(context.Background(), c.readConn()); err == nil {
		t.Fatal("lost failure")
	}
	if _, err := c.StreamChat(context.Background(), "conversation", "must not be queued", ""); err == nil || !strings.Contains(err.Error(), "request not sent") {
		t.Fatalf("failed demand must report not sent: %v", err)
	}
	_, _ = pinnedHost(t, addr, f)
	for i := 0; i < 20; i++ {
		if err := c.reconnectPinnedTransport(context.Background(), c.readConn()); err == nil {
			t.Fatal("observer retried")
		}
	}
	if f.checks.Load() != 1 || f.requests.Load() != 0 {
		t.Fatal("observer work/replay")
	}
	out, err := c.StreamChat(context.Background(), "conversation", "new demand", "")
	if err != nil {
		t.Fatal(err)
	}
	for range out {
	}
	if f.requests.Load() != 1 {
		t.Fatalf("requests=%d", f.requests.Load())
	}
}

func TestPinnedConcurrentFailedDemandSharesRefusal(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
	f := &pinnedFake{host: "A", validationStarted: make(chan struct{}), validationRelease: make(chan struct{}), validationCode: codes.Unauthenticated}
	f.state.Store(int32(proto.RetainedSessionState_RETAINED_SESSION_STATE_RETAINED))
	_, addr := pinnedHost(t, "127.0.0.1:0", f)
	c, err := DialPinnedSession(context.Background(), pinAt(addr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.readConn().Close()
	select {
	case <-f.validationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("no validation")
	}
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func() { _, err := c.StreamChat(context.Background(), "conversation", "new demand", ""); results <- err }()
	}
	close(f.validationRelease)
	for i := 0; i < 20; i++ {
		select {
		case err := <-results:
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("lost shared refusal: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("blocked demand")
		}
	}
	if f.checks.Load() != 2 || f.requests.Load() != 0 {
		t.Fatalf("checks=%d requests=%d", f.checks.Load(), f.requests.Load())
	}
}
