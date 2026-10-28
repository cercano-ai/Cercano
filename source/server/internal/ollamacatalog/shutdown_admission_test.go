package ollamacatalog

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type shutdownTransport func(*http.Request) (*http.Response, error)

func (f shutdownTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestShutdownAdmissionActiveCatalogJoin(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "refresh", true: "warm"}[warm], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			m := NewManager(&Fetcher{HTTPClient: &http.Client{Transport: shutdownTransport(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				close(entered)
				<-release
				return nil, errors.New("fixture")
			})}}, filepath.Join(t.TempDir(), "cache"))
			m.cache = &Cache{Models: []Model{{Name: "fixture"}, {Name: "never"}}}
			done := make(chan struct{})
			go func() {
				defer close(done)
				if warm {
					m.WarmMissingEstimates(context.Background(), nil)
				} else {
					_ = m.Refresh(context.Background())
				}
			}()
			<-entered
			m.Fence()
			if err := m.Refresh(context.Background()); !errors.Is(err, context.Canceled) {
				t.Fatalf("fenced refresh: %v", err)
			}
			m.WarmMissingEstimates(context.Background(), nil)
			joined := make(chan struct{})
			go func() { m.Stop(); close(joined) }()
			select {
			case <-joined:
				t.Fatal("joined active callback")
			default:
			}
			close(release)
			<-done
			<-joined
			if calls.Load() != 1 {
				t.Fatalf("fetch calls=%d", calls.Load())
			}
		})
	}
}

func TestShutdownAdmissionCatalogFenceDoesNotWaitForCache(t *testing.T) {
	m := NewManager(&Fetcher{}, "")
	// Cache persistence holds mu; fencing must not acquire that work lock.
	m.mu.Lock()
	m.Fence()
	m.mu.Unlock()
	m.Start(context.Background()) // must not resurrect a fenced schedule
	m.Stop()
}
