package broker

// QuiesceOwner fences the retained epoch, even if a different outgoing control
// already closed admission. It does not complete any producer or persistence
// ticket and must not be used as proof that writeback is finished.
func (a *Admission) QuiesceOwner(o Owner) (*Closure, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessionLocked(o)
	if s == nil {
		return nil, ErrAdmissionStale
	}
	if a.retired || s.draining {
		return nil, ErrAdmissionClosed
	}
	s.closed = true
	select {
	case <-s.cutover:
	default:
		close(s.cutover)
	}
	for t := range s.attachments {
		if t.terminal != "" {
			s.revoked[t.terminal] = true
		}
		close(t.revoked)
		delete(s.attachments, t)
	}
	s.terminals = make(map[string]int)
	s.activeTerminal = ""
	if s.closure == nil {
		s.closure = &Closure{a: a, s: s}
	}
	return s.closure, nil
}
