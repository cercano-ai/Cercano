package broker

// ForceCloseOwner fences an exact ownership epoch and revokes all terminal
// references, including idle/stale streams. It does not pretend that execution
// or persistence has completed: the caller cancels producers and waits for
// their existing tickets before committing an ownership transfer.
func (a *Admission) ForceCloseOwner(o Owner) (*Closure, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessionLocked(o)
	if s == nil {
		return nil, ErrAdmissionStale
	}
	if a.retired || s.closed {
		return nil, ErrAdmissionClosed
	}
	s.closed = true
	close(s.cutover)
	for t := range s.attachments {
		if t.terminal != "" {
			s.revoked[t.terminal] = true
		}
		close(t.revoked)
		delete(s.attachments, t)
	}
	s.terminals = make(map[string]int)
	s.activeTerminal = ""
	c := &Closure{a: a, s: s}
	s.closure = c
	return c, nil
}

// Pending reports actual ticket accounting, not attachment or owner presence.
// An unresolved ticket is an uncertain durable outcome, not proof of execution.
func (c *Closure) Pending() (work, unresolved int, err error) {
	c.a.mu.Lock()
	defer c.a.mu.Unlock()
	if c.done || c.a.sessionLocked(c.s.owner) != c.s || c.s.closure != c {
		return 0, 0, ErrAdmissionStale
	}
	for t := range c.s.work {
		work++
		if t.unresolved {
			unresolved++
		}
	}
	return work, unresolved, nil
}

// CutoverSignal is epoch-bound and does not infer activity from retention.
func (a *Admission) CutoverSignal(o Owner) (<-chan struct{}, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessionLocked(o)
	if s == nil {
		return nil, ErrAdmissionStale
	}
	return s.cutover, nil
}
