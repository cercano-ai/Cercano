package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"google.golang.org/grpc"
	"path/filepath"
	"sort"
	"sync"

	"cercano/source/server/internal/broker"
	"cercano/source/server/internal/conversation"
)

// CoexistenceOptions enables only the lifecycle control surface, not normal RPC
// handling. StorePath must be an explicitly prepared fixture. Build and Host are
// immutable identities, not endpoints. CloseParticipation MUST only release this
// client's runtime participation, never Shutdown, StopAll, or restart a backend.
type CoexistenceOptions struct {
	StorePath, Build, Host string
	// Controls remain disabled unless startup explicitly supplies authenticated authorization and peer dialing.
	ControlAuthorize   func(context.Context) error
	ControlDial        func(context.Context, string) (*grpc.ClientConn, error)
	Contract           conversation.CoexistenceDeclaration
	CloseParticipation func(context.Context) error
	// Reload must inspect durable pending outcomes without replaying live work.
	Reload func(context.Context, conversation.Store, *conversation.Ownership) error
	// Cleanup finishes source callbacks after durable relinquishment and unlock.
	Cleanup func(context.Context, broker.Owner) error
}

type coexistenceLifecycle struct {
	mu        sync.Mutex
	options   CoexistenceOptions
	store     conversation.Store
	admission *broker.Admission
	owners    map[string]*conversation.Ownership
	incoming  map[broker.IncomingHandover]*IncomingAcceptance
}

// NewCoexistenceServer constructs a lifecycle-only server. Do not serve ordinary
// requests with it until every work path supplies admission tickets/capabilities.
func NewCoexistenceServer(ctx context.Context, o CoexistenceOptions) (*Server, error) {
	if o.Build == "" || o.Host == "" || o.CloseParticipation == nil || o.Reload == nil {
		return nil, fmt.Errorf("build, exact host, reload and safe participation hook required")
	}
	path, err := filepath.Abs(o.StorePath)
	if err != nil {
		return nil, err
	}
	o.StorePath = path
	store, err := conversation.OpenCoexistence(ctx, o.StorePath, o.Contract)
	if err != nil {
		return nil, err
	}
	return &Server{coexistence: &coexistenceLifecycle{options: o, store: store, admission: broker.NewAdmission(o.Host), owners: make(map[string]*conversation.Ownership), incoming: make(map[broker.IncomingHandover]*IncomingAcceptance)}}, nil
}
func (s *Server) lifecycle() (*coexistenceLifecycle, error) {
	if s.coexistence == nil {
		return nil, fmt.Errorf("coexistence lifecycle not enabled")
	}
	return s.coexistence, nil
}
func ownerOf(o *conversation.Ownership) broker.Owner {
	return broker.Owner{Conversation: o.ConversationID(), Host: o.HostInstance(), Epoch: o.Epoch()}
}

// Retain is explicit resume/acquisition, never a reconnect operation.
func (s *Server) Retain(ctx context.Context, id string) (broker.Owner, error) {
	l, e := s.lifecycle()
	if e != nil {
		return broker.Owner{}, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.acquire(ctx, id, nil)
}
func (l *coexistenceLifecycle) acquire(ctx context.Context, id string, accepted *IncomingAcceptance) (broker.Owner, error) {
	t, e := l.admission.AdmitHostWork(broker.PersistenceWork)
	if e != nil {
		return broker.Owner{}, e
	}
	var o *conversation.Ownership
	if accepted == nil {
		o, e = conversation.AcquireOwnership(ctx, l.store, id, l.options.Host)
	} else {
		o, e = conversation.ClaimHandover(ctx, l.store, id, l.options.Host, accepted.h.HandoverID)
	}
	if o != nil {
		l.owners[id] = o
	}
	if e != nil {
		if o != nil {
			_ = t.PersistenceFailed()
		} else {
			_ = t.Complete()
		}
		return broker.Owner{}, e
	}
	identity := ownerOf(o)
	if e = l.options.Reload(conversation.WithOwnership(ctx, o), l.store, o); e != nil {
		_ = t.PersistenceFailed()
		return broker.Owner{}, fmt.Errorf("acquired but reload unresolved: %w", e)
	}
	if accepted == nil {
		e = l.admission.RegisterOwner(identity)
	} else {
		e = l.admission.AcquiredIncoming(accepted.h, identity)
	}
	if e != nil {
		_ = t.PersistenceFailed()
		return broker.Owner{}, e
	}
	_ = t.Complete()
	return identity, nil
}

func (s *Server) ListRetained() ([]broker.Owner, error) {
	l, e := s.lifecycle()
	if e != nil {
		return nil, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]broker.Owner, 0, len(l.owners))
	for _, o := range l.owners {
		out = append(out, ownerOf(o))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Conversation < out[j].Conversation })
	return out, nil
}

// AttachRetained never acquires, launches, or resumes. Detach the returned handle
// for transport cleanup; ownership survives disconnect.
func (s *Server) AttachRetained(o broker.Owner) (*broker.Attachment, error) {
	l, e := s.lifecycle()
	if e != nil {
		return nil, e
	}
	return l.admission.AttachOwner(o)
}
func (s *Server) AdmitRetainedWork(o broker.Owner, k broker.WorkKind) (*broker.WorkTicket, error) {
	l, e := s.lifecycle()
	if e != nil {
		return nil, e
	}
	return l.admission.AdmitWork(o, k)
}

// IncomingAcceptance is an opaque in-process proof of exact destination
// participation. Future transport wiring must authenticate an equivalent receipt;
// a caller-supplied tuple alone is not proof. It cannot be canceled on timeout.
type IncomingAcceptance struct {
	destination *coexistenceLifecycle
	sourceBuild string
	receipt     string
	h           broker.IncomingHandover
}

func (s *Server) AcceptIncoming(source broker.Owner, id string, peerBuild string, contract conversation.CoexistenceDeclaration) (*IncomingAcceptance, error) {
	l, e := s.lifecycle()
	if e != nil {
		return nil, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if peerBuild == "" || contract != l.options.Contract || source.Host == l.options.Host {
		return nil, fmt.Errorf("incompatible or identical peer")
	}
	h := broker.IncomingHandover{Conversation: source.Conversation, SourceHost: source.Host, SourceEpoch: source.Epoch, HandoverID: id, DestinationHost: l.options.Host}
	var token [32]byte
	if _, e = rand.Read(token[:]); e != nil {
		return nil, e
	}
	if e = l.admission.AcceptIncoming(h); e != nil {
		return nil, e
	}
	if a := l.incoming[h]; a != nil {
		if a.sourceBuild != peerBuild {
			return nil, broker.ErrAdmissionStale
		}
		return a, nil
	}
	a := &IncomingAcceptance{receipt: hex.EncodeToString(token[:]), destination: l, sourceBuild: peerBuild, h: h}
	l.incoming[h] = a
	return a, nil
}
func (s *Server) ReleaseRetained(ctx context.Context, o broker.Owner, requester *broker.Attachment) error {
	return s.relinquish(ctx, o, requester, nil)
}

// HandoverRetained reports only pending reservation, never destination success.
func (s *Server) HandoverRetained(ctx context.Context, o broker.Owner, requester *broker.Attachment, a *IncomingAcceptance) error {
	if a == nil {
		return fmt.Errorf("preaccepted destination required")
	}
	return s.relinquish(ctx, o, requester, a)
}
func (s *Server) relinquish(ctx context.Context, o broker.Owner, requester *broker.Attachment, a *IncomingAcceptance) error {
	l, e := s.lifecycle()
	if e != nil {
		return e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cap := l.owners[o.Conversation]
	if cap == nil || ownerOf(cap) != o {
		return broker.ErrAdmissionStale
	}
	if a != nil && (a.destination == nil || a.destination.options.StorePath != l.options.StorePath || a.sourceBuild != l.options.Build || a.destination.options.Contract != l.options.Contract || a.h.SourceHost != o.Host || a.h.SourceEpoch != o.Epoch || a.h.Conversation != o.Conversation) {
		return broker.ErrAdmissionStale
	}
	var h *broker.IncomingHandover
	if a != nil {
		h = &a.h
	}
	return l.relinquishLocked(ctx, o, requester, nil, h)
}
func (l *coexistenceLifecycle) relinquishLocked(ctx context.Context, o broker.Owner, requester *broker.Attachment, terminal *string, h *broker.IncomingHandover) error {
	cap := l.owners[o.Conversation]
	if cap == nil || ownerOf(cap) != o {
		return broker.ErrAdmissionStale
	}
	t, e := l.admission.AdmitHostWork(broker.PersistenceWork)
	if e != nil {
		return e
	}
	var c *broker.Closure
	if terminal != nil {
		c, e = l.admission.CloseTerminal(o, *terminal)
	} else {
		c, e = l.admission.CloseOwner(o, requester)
	}
	if e != nil {
		_ = t.Complete()
		return e
	}
	if h == nil {
		e = cap.Release(ctx)
	} else {
		e = cap.ReserveHandover(ctx, h.DestinationHost, h.HandoverID)
	}
	// Conservatively leave closure and ticket unresolved on ANY persistence/unlock
	// error. No automatic reopen, unlock, timeout reassignment or recovery.
	if e != nil {
		_ = t.PersistenceFailed()
		return e
	}
	if e = c.Relinquished(); e != nil {
		_ = t.PersistenceFailed()
		return e
	}
	delete(l.owners, o.Conversation)
	if l.options.Cleanup != nil {
		if e = l.options.Cleanup(ctx, o); e != nil {
			_ = t.PersistenceFailed()
			return e
		}
	}
	return t.Complete()
}
func (s *Server) ClaimIncoming(ctx context.Context, a *IncomingAcceptance) (broker.Owner, error) {
	l, e := s.lifecycle()
	if e != nil {
		return broker.Owner{}, e
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if a == nil || a.destination != l || l.incoming[a.h] != a {
		return broker.Owner{}, broker.ErrAdmissionStale
	}
	if o := l.owners[a.h.Conversation]; o != nil {
		epoch, err := conversation.HandoverResult(ctx, l.store, a.h.Conversation, l.options.Host, a.h.HandoverID)
		if err != nil {
			return broker.Owner{}, err
		}
		return broker.Owner{}, &conversation.HandoverAlreadyClaimed{Epoch: epoch}
	}
	return l.acquire(ctx, a.h.Conversation, a)
}

// RetireCoexistence is a readiness reservation, NOT a shutdown probe. Hook errors
// leave admission retired/closed and are surfaced; normal Shutdown is never used.
func (s *Server) RetireCoexistence(ctx context.Context) error {
	l, e := s.lifecycle()
	if e != nil {
		return e
	}
	if e = l.admission.Retire(); e != nil {
		return e
	}
	// Admission is retired and has no outstanding task tickets. Close the
	// prepared manager before closing the shared agent/persistence store.
	s.closeTaskManager()
	if e = l.options.CloseParticipation(ctx); e != nil {
		return e
	}
	return l.store.Close()
}
