package broker

import (
	"errors"
	"sync"
)

var (
	ErrAdmissionClosed       = errors.New("admission closed")
	ErrAdmissionBusy         = errors.New("retained sessions or work prevent transition")
	ErrAdmissionStale        = errors.New("unknown or stale ownership or ticket")
	ErrAttachmentEligibility = errors.New("requester is not the sole attachment")
)

// Owner is a durable ownership identity, never a Broker turn generation.
type Owner struct{ Conversation, Host, Epoch string }

// WorkKind includes accepted tasks, even when they are not currently executing.
type WorkKind uint8

const (
	ForegroundWork WorkKind = iota + 1
	TaskAcceptanceWork
	TaskExecutionWork
	CallbackWork
	PersistenceWork
)

func validWork(k WorkKind) bool { return k >= ForegroundWork && k <= PersistenceWork }

type admissionSession struct {
	owner       Owner
	closed      bool
	closure     *Closure
	attachments map[*Attachment]bool
	work        map[*WorkTicket]bool
}

// Admission is local accounting adjacent to Broker, not an ownership authority.
// Existing Broker methods do not implicitly participate: callers must hold tickets
// across all work, including accepted tasks and continuations. No method cancels
// work, invokes callbacks, or acquires/releases an OS lock.
//
// Lock ordering: mu is a leaf lock. No store, Broker, transport, or caller code is
// invoked while held. Perform durable transitions and OS-lock operations outside
// these methods. The server/store layer must validate durable epochs and retain
// the source OS lock throughout a closed-admission transition.
// The zero value is not usable; construct one per exact host instance.
type Admission struct {
	mu       sync.Mutex
	host     string
	retired  bool
	sessions map[string]*admissionSession
	seen     map[Owner]bool
	hostWork map[*WorkTicket]bool
	incoming map[IncomingHandover]bool
}

func NewAdmission(host string) *Admission {
	if host == "" {
		panic("empty admission host")
	}
	return &Admission{host: host, sessions: make(map[string]*admissionSession), seen: make(map[Owner]bool), hostWork: make(map[*WorkTicket]bool), incoming: make(map[IncomingHandover]bool)}
}

// RegisterOwner records a successfully committed acquisition. Acquisition must
// first be protected from retirement with a host work ticket; keep it until this
// registration completes. Epoch freshness is also enforced by the durable store.
func (a *Admission) RegisterOwner(o Owner) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.registerLocked(o)
}
func (a *Admission) registerLocked(o Owner) error {
	if a.retired {
		return ErrAdmissionClosed
	}
	if o.Host != a.host || o.Conversation == "" || o.Epoch == "" || a.seen[o] {
		return ErrAdmissionStale
	}
	if a.sessions[o.Conversation] != nil {
		return ErrAdmissionBusy
	}
	a.sessions[o.Conversation] = &admissionSession{owner: o, attachments: make(map[*Attachment]bool), work: make(map[*WorkTicket]bool)}
	a.seen[o] = true
	return nil
}
func (a *Admission) sessionLocked(o Owner) *admissionSession {
	s := a.sessions[o.Conversation]
	if s == nil || s.owner != o {
		return nil
	}
	return s
}

// Attachment is epoch-bound. Detach is transport cleanup, never session release.
type Attachment struct {
	a *Admission
	s *admissionSession
}

func (a *Admission) AttachOwner(o Owner) (*Attachment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessionLocked(o)
	if s == nil {
		return nil, ErrAdmissionStale
	}
	if a.retired || s.closed {
		return nil, ErrAdmissionClosed
	}
	t := &Attachment{a: a, s: s}
	s.attachments[t] = true
	return t, nil
}
func (t *Attachment) Detach() {
	t.a.mu.Lock()
	defer t.a.mu.Unlock()
	delete(t.s.attachments, t)
}

// WorkTicket remains live until Complete succeeds. Do not complete on transport
// timeout or context cancellation while execution, effects or writeback remain.
// Advance bridges acceptance/execution/callback/persistence without an idle gap.
// Tickets must not be copied.
type WorkTicket struct {
	a          *Admission
	s          *admissionSession
	kind       WorkKind
	unresolved bool
	done       bool
}

func (a *Admission) AdmitWork(o Owner, k WorkKind) (*WorkTicket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessionLocked(o)
	if s == nil {
		return nil, ErrAdmissionStale
	}
	if a.retired || s.closed {
		return nil, ErrAdmissionClosed
	}
	return a.ticketLocked(s, k)
}

// AdmitHostWork fences acquisition and other non-session lifecycle activity.
// For release/handover, callers must acquire a lifecycle ticket before starting
// the transition and hold it through the durable release/reservation commit,
// Closure.Relinquished, OS unlock, and all cleanup. Only then call Complete:
// removing the session alone does not mean the host is safe to retire.
func (a *Admission) AdmitHostWork(k WorkKind) (*WorkTicket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.retired {
		return nil, ErrAdmissionClosed
	}
	return a.ticketLocked(nil, k)
}
func (a *Admission) ticketLocked(s *admissionSession, k WorkKind) (*WorkTicket, error) {
	if !validWork(k) {
		return nil, ErrAdmissionStale
	}
	t := &WorkTicket{a: a, s: s, kind: k}
	if s == nil {
		a.hostWork[t] = true
	} else {
		s.work[t] = true
	}
	return t, nil
}
func (t *WorkTicket) Advance(k WorkKind) error {
	t.a.mu.Lock()
	defer t.a.mu.Unlock()
	if t.done || !validWork(k) {
		return ErrAdmissionStale
	}
	if t.unresolved {
		return ErrAdmissionBusy
	}
	t.kind = k
	return nil
}

// PersistenceFailed retains an unresolved blocker even after execution ends.
func (t *WorkTicket) PersistenceFailed() error {
	t.a.mu.Lock()
	defer t.a.mu.Unlock()
	if t.done || t.kind != PersistenceWork {
		return ErrAdmissionStale
	}
	t.unresolved = true
	return nil
}

// ResolvePersistence is explicit acknowledgement of a reconciled durable outcome,
// not a retry timer. The caller must establish that outcome in the store first.
func (t *WorkTicket) ResolvePersistence() error {
	t.a.mu.Lock()
	defer t.a.mu.Unlock()
	if t.done || t.kind != PersistenceWork {
		return ErrAdmissionStale
	}
	t.unresolved = false
	return nil
}
func (t *WorkTicket) Complete() error {
	t.a.mu.Lock()
	defer t.a.mu.Unlock()
	if t.done {
		return ErrAdmissionStale
	}
	if t.unresolved {
		return ErrAdmissionBusy
	}
	t.done = true
	if t.s == nil {
		delete(t.a.hostWork, t)
	} else {
		delete(t.s.work, t)
	}
	return nil
}

// Closure reserves readiness for either explicit release or handover. It is not
// evidence of durable relinquishment. An uncertain commit must leave it closed.
// Only the exact active handle may finish the closure; copies are stale.
type Closure struct {
	a    *Admission
	s    *admissionSession
	done bool
}

// CloseOwner atomically closes admission, checks requester eligibility and all
// work. A nil requester requires a disconnected session; otherwise it must be
// the sole live attachment. Refusal reopens admission without cancelling work.
func (a *Admission) CloseOwner(o Owner, requester *Attachment) (*Closure, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.sessionLocked(o)
	if s == nil {
		return nil, ErrAdmissionStale
	}
	if s.closed || a.retired {
		return nil, ErrAdmissionClosed
	}
	s.closed = true
	eligible := len(s.attachments) == 0 && requester == nil
	if requester != nil {
		eligible = len(s.attachments) == 1 && s.attachments[requester]
	}
	if !eligible {
		s.closed = false
		return nil, ErrAttachmentEligibility
	}
	if len(s.work) != 0 {
		s.closed = false
		return nil, ErrAdmissionBusy
	}
	c := &Closure{a: a, s: s}
	s.closure = c
	return c, nil
}

// Reopen is allowed only after the caller confirms durable ownership is unchanged.
// Never call it after committed relinquishment or an uncertain commit result.
func (c *Closure) Reopen() error { return c.finish(false) }

// Relinquished records a confirmed conditional durable release/reservation commit.
// The caller, not this method, remains responsible for releasing the OS lock
// and must retain an AdmitHostWork ticket through that unlock and cleanup.
func (c *Closure) Relinquished() error { return c.finish(true) }
func (c *Closure) finish(relinquished bool) error {
	c.a.mu.Lock()
	defer c.a.mu.Unlock()
	if c.done || c.a.sessionLocked(c.s.owner) != c.s || c.s.closure != c {
		return ErrAdmissionStale
	}
	c.done = true
	c.s.closure = nil
	if relinquished {
		delete(c.a.sessions, c.s.owner.Conversation)
	} else {
		c.s.closed = false
	}
	return nil
}

// IncomingHandover is exact-instance participation, not an endpoint/build alias.
type IncomingHandover struct{ Conversation, SourceHost, SourceEpoch, HandoverID, DestinationHost string }

// AcceptIncoming must precede source reservation. Participation survives timeout,
// disconnect and retries; there is deliberately no transport-driven release API.
func (a *Admission) AcceptIncoming(h IncomingHandover) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.retired {
		return ErrAdmissionClosed
	}
	if h.Conversation == "" || h.SourceHost == "" || h.SourceEpoch == "" || h.HandoverID == "" || h.DestinationHost != a.host {
		return ErrAdmissionStale
	}
	a.incoming[h] = true
	return nil
}

// AcquiredIncoming atomically replaces participation with retained ownership,
// after the store has consumed this exact reservation with a fresh durable epoch.
func (a *Admission) AcquiredIncoming(h IncomingHandover, o Owner) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.incoming[h] || o.Conversation != h.Conversation {
		return ErrAdmissionStale
	}
	if err := a.registerLocked(o); err != nil {
		return err
	}
	delete(a.incoming, h)
	return nil
}

// ResolveIncomingDurably removes only the exact participation after the caller
// proves no pending reservation/claim/callback remains (including in-flight source
// negotiation). Timeout, disconnect, or a missing record alone is not such proof.
func (a *Admission) ResolveIncomingDurably(h IncomingHandover) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.incoming[h] {
		return ErrAdmissionStale
	}
	delete(a.incoming, h)
	return nil
}

// Retire atomically fences acquisition, attachment and destination acceptance,
// checks readiness and either commits local retirement or reversibly refuses.
// It performs no shutdown, cancellation, bulk release or backend operation.
func (a *Admission) Retire() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.retired {
		return ErrAdmissionClosed
	}
	a.retired = true
	if len(a.sessions) != 0 || len(a.hostWork) != 0 || len(a.incoming) != 0 {
		a.retired = false
		return ErrAdmissionBusy
	}
	return nil
}
