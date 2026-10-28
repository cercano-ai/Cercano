// Package workadmission linearizes invocation-local callback entry and shutdown.
package workadmission

import "sync"

// Gate's zero value admits work. Fence never waits for admitted callbacks.
// Wait must follow Fence; dependencies remain alive until Wait returns.
type Gate struct {
	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
}

func (g *Gate) Enter() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	g.active.Add(1)
	return true
}
func (g *Gate) Leave() { g.active.Done() }
func (g *Gate) Fence() { g.mu.Lock(); g.closed = true; g.mu.Unlock() }
func (g *Gate) Wait()  { g.active.Wait() }
