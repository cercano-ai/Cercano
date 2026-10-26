package modelhost

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"cercano/source/server/internal/localruntime"
)

var errRetryStop = errors.New("stop failed while still live")

type retryStopManager struct {
	protectionManager
	failures int
}

func (m *retryStopManager) Stop(context.Context, localruntime.StopRequest) error {
	m.stops++
	if m.failures > 0 {
		m.failures--
		return errRetryStop
	}
	m.instances = nil
	return nil
}

func TestReleaseFailureRetainsResponsibility(t *testing.T) {
	for _, protected := range []bool{false, true} {
		for _, disconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("protected=%t/disconnect=%t", protected, disconnect), func(t *testing.T) {
				ctx := context.Background()
				m := &retryStopManager{failures: 1}
				m.instances = []localruntime.InstanceRecord{{ID: "i", Runtime: "fake", ModelID: "m"}}
				s := NewServer(m, ServerOptions{CoexistenceProtection: protected})
				st := s.addStream("a")
				s.holds.add("a", "i")
				if err := s.stop(ctx, "a", localruntime.StopRequest{InstanceID: "i"}); !errors.Is(err, errRetryStop) {
					t.Fatalf("first Stop: %v", err)
				}
				if !s.holds.has("a", "i") || len(m.instances) != 1 {
					t.Fatal("lost cleanup responsibility")
				}
				if disconnect {
					if !s.removeStream("a", st) {
						t.Fatal("still attached")
					}
					// A failed disconnect cleanup must also retain responsibility for retry.
					m.failures = 1
					s.releaseClient("a")
					if !s.holds.has("a", "i") {
						t.Fatal("disconnect lost cleanup responsibility")
					}
					s.releaseClient("a")
				} else if err := s.stop(ctx, "a", localruntime.StopRequest{InstanceID: "i"}); err != nil {
					t.Fatal(err)
				}
				want := 2
				if disconnect {
					want = 3
				}
				if m.stops != want || len(m.instances) != 0 || s.holds.count("i") != 0 {
					t.Fatalf("cleanup: stops=%d holds=%d", m.stops, s.holds.count("i"))
				}
			})
		}
		t.Run(fmt.Sprintf("protected=%t/sibling", protected), func(t *testing.T) {
			m := &retryStopManager{failures: 1}
			s := NewServer(m, ServerOptions{CoexistenceProtection: protected})
			s.holds.add("a", "i")
			req := localruntime.StopRequest{InstanceID: "i"}
			if err := s.stop(context.Background(), "a", req); !errors.Is(err, errRetryStop) {
				t.Fatal(err)
			}
			s.holds.add("b", "i")
			for i := 0; i < 2; i++ {
				if err := s.stop(context.Background(), "a", req); err != nil {
					t.Fatal(err)
				}
			}
			s.releaseClient("a")
			if m.stops != 1 || !s.holds.has("b", "i") {
				t.Fatal("A unloaded B's instance")
			}
		})
	}
}
