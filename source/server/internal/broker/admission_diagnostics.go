package broker

// AdmissionSnapshot is a copy of local accounting, not durable ownership proof.
// HostWork is host-wide and cannot be attributed to the requested owner.
// No ticket, attachment, terminal identity, callback, or receipt is exposed.
type AdmissionSnapshot struct {
	HostRetainedSessions int
	Retired              bool
	Retained             bool
	Open                 bool
	Closed               bool
	Draining             bool
	Attachments          int
	Work                 map[WorkKind]int
	UnresolvedWork       int
	HostWork             map[WorkKind]int
	UnresolvedHostWork   int
}

// Snapshot only reads accounting under its leaf lock. A mismatched owner never
// exposes another epoch's session. Host-wide counters are intentionally separate.
func (a *Admission) Snapshot(o Owner) AdmissionSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := AdmissionSnapshot{HostRetainedSessions: len(a.sessions), Retired: a.retired, Work: make(map[WorkKind]int), HostWork: make(map[WorkKind]int)}
	for t := range a.hostWork {
		out.HostWork[t.kind]++
		if t.unresolved {
			out.UnresolvedHostWork++
		}
	}
	if s := a.sessionLocked(o); s != nil {
		out.Retained = true
		out.Open = !a.retired && !s.closed
		out.Closed = s.closed
		out.Draining = s.draining
		out.Attachments = len(s.attachments)
		for t := range s.work {
			out.Work[t.kind]++
			if t.unresolved {
				out.UnresolvedWork++
			}
		}
	}
	return out
}
