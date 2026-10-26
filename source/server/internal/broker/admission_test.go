package broker

import (
	"errors"
	"testing"
)

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func wantErr(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
func fixtureAdmission(t *testing.T) (*Admission, Owner) {
	t.Helper()
	a := NewAdmission("A")
	o := Owner{"one", "A", "epoch-1"}
	mustOK(t, a.RegisterOwner(o))
	return a, o
}

func TestAdmissionClosureRace(t *testing.T) {
	// Both contenders cross an explicit start barrier. Either serialization is
	// valid, but it is impossible for a work ticket and closure both to succeed.
	for i := 0; i < 100; i++ {
		a, o := fixtureAdmission(t)
		start := make(chan struct{})
		ready := make(chan struct{}, 2)
		type workResult struct {
			ticket *WorkTicket
			err    error
		}
		type closeResult struct {
			closure *Closure
			err     error
		}
		work := make(chan workResult, 1)
		closed := make(chan closeResult, 1)
		go func() { ready <- struct{}{}; <-start; w, e := a.AdmitWork(o, ForegroundWork); work <- workResult{w, e} }()
		go func() { ready <- struct{}{}; <-start; c, e := a.CloseOwner(o, nil); closed <- closeResult{c, e} }()
		<-ready
		<-ready
		close(start)
		w, c := <-work, <-closed
		if w.err == nil {
			wantErr(t, c.err, ErrAdmissionBusy)
			mustOK(t, w.ticket.Complete())
		} else {
			wantErr(t, w.err, ErrAdmissionClosed)
			mustOK(t, c.err)
			mustOK(t, c.closure.Reopen())
		}
		ticket, e := a.AdmitWork(o, CallbackWork)
		mustOK(t, e)
		mustOK(t, ticket.Complete())
	}
}

func TestAdmissionBusyKindsAndUnresolvedPersistence(t *testing.T) {
	a, o := fixtureAdmission(t)
	for _, kind := range []WorkKind{ForegroundWork, TaskAcceptanceWork, TaskExecutionWork, CallbackWork, PersistenceWork} {
		w, e := a.AdmitWork(o, kind)
		mustOK(t, e)
		_, e = a.CloseOwner(o, nil)
		wantErr(t, e, ErrAdmissionBusy)
		extra, e := a.AdmitWork(o, ForegroundWork)
		mustOK(t, e)
		mustOK(t, extra.Complete())
		mustOK(t, w.Complete())
	}
	w, e := a.AdmitWork(o, TaskAcceptanceWork)
	mustOK(t, e)
	for _, kind := range []WorkKind{TaskExecutionWork, CallbackWork, PersistenceWork} {
		mustOK(t, w.Advance(kind))
	}
	mustOK(t, w.PersistenceFailed())
	wantErr(t, w.Complete(), ErrAdmissionBusy)
	wantErr(t, w.Advance(CallbackWork), ErrAdmissionBusy)
	_, e = a.CloseOwner(o, nil)
	wantErr(t, e, ErrAdmissionBusy)
	mustOK(t, w.ResolvePersistence())
	mustOK(t, w.Complete())
	c, e := a.CloseOwner(o, nil)
	mustOK(t, e)
	// Uncertain durability is represented by retaining the closure, not reopening.
	_, e = a.AdmitWork(o, ForegroundWork)
	wantErr(t, e, ErrAdmissionClosed)
	mustOK(t, c.Relinquished())
	wantErr(t, c.Reopen(), ErrAdmissionStale)
	_, e = a.AttachOwner(o)
	wantErr(t, e, ErrAdmissionStale)
	wantErr(t, a.RegisterOwner(o), ErrAdmissionStale)
}

func TestAdmissionAttachmentsAndSiblingIsolation(t *testing.T) {
	a, o := fixtureAdmission(t)
	sibling := Owner{"two", "A", "epoch-2"}
	mustOK(t, a.RegisterOwner(sibling))
	sw, e := a.AdmitWork(sibling, TaskExecutionWork)
	mustOK(t, e)
	first, e := a.AttachOwner(o)
	mustOK(t, e)
	second, e := a.AttachOwner(o)
	mustOK(t, e)
	_, e = a.CloseOwner(o, first)
	wantErr(t, e, ErrAttachmentEligibility)
	second.Detach()
	_, e = a.CloseOwner(o, second)
	wantErr(t, e, ErrAttachmentEligibility)
	_, e = a.CloseOwner(o, nil)
	wantErr(t, e, ErrAttachmentEligibility)
	c, e := a.CloseOwner(o, first)
	mustOK(t, e)
	_, e = a.AttachOwner(o)
	wantErr(t, e, ErrAdmissionClosed)
	sa, e := a.AttachOwner(sibling)
	mustOK(t, e)
	sa.Detach()
	mustOK(t, c.Relinquished())
	first.Detach()
	wantErr(t, a.Retire(), ErrAdmissionBusy)
	fresh := Owner{o.Conversation, o.Host, "fresh"}
	mustOK(t, a.RegisterOwner(fresh))
	_, e = a.CloseOwner(fresh, first)
	wantErr(t, e, ErrAttachmentEligibility)
	c, e = a.CloseOwner(fresh, nil)
	mustOK(t, e)
	mustOK(t, c.Relinquished())
	mustOK(t, sw.Complete())
	c, e = a.CloseOwner(sibling, nil)
	mustOK(t, e)
	mustOK(t, c.Relinquished())
	mustOK(t, a.Retire())
	_, e = a.AdmitHostWork(CallbackWork)
	wantErr(t, e, ErrAdmissionClosed)
	wantErr(t, a.RegisterOwner(Owner{"three", "A", "3"}), ErrAdmissionClosed)
}

func TestRetirementDestinationRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		a := NewAdmission("B")
		h := IncomingHandover{"one", "A", "epoch", "handover", "B"}
		start := make(chan struct{})
		ready := make(chan struct{}, 2)
		accepted := make(chan error, 1)
		retired := make(chan error, 1)
		go func() { ready <- struct{}{}; <-start; accepted <- a.AcceptIncoming(h) }()
		go func() { ready <- struct{}{}; <-start; retired <- a.Retire() }()
		<-ready
		<-ready
		close(start)
		ae, re := <-accepted, <-retired
		if ae == nil {
			wantErr(t, re, ErrAdmissionBusy)
			mustOK(t, a.AcceptIncoming(h)) // retry does not duplicate or clear participation
			mismatch := h
			mismatch.HandoverID = "other"
			wantErr(t, a.ResolveIncomingDurably(mismatch), ErrAdmissionStale)
			wantErr(t, a.Retire(), ErrAdmissionBusy)
			mustOK(t, a.AcquiredIncoming(h, Owner{"one", "B", "fresh"}))
			wantErr(t, a.Retire(), ErrAdmissionBusy)
			c, e := a.CloseOwner(Owner{"one", "B", "fresh"}, nil)
			mustOK(t, e)
			mustOK(t, c.Relinquished())
			mustOK(t, a.Retire())
		} else {
			wantErr(t, ae, ErrAdmissionClosed)
			mustOK(t, re)
		}
	}
}

func TestHostWorkRetirementAndExactIncomingResolution(t *testing.T) {
	a := NewAdmission("B")
	w, e := a.AdmitHostWork(PersistenceWork)
	mustOK(t, e)
	mustOK(t, w.PersistenceFailed())
	wantErr(t, a.Retire(), ErrAdmissionBusy)
	wantErr(t, w.Complete(), ErrAdmissionBusy)
	mustOK(t, w.ResolvePersistence())
	mustOK(t, w.Complete())
	h := IncomingHandover{"one", "A", "epoch", "id", "B"}
	mustOK(t, a.AcceptIncoming(h))
	for _, change := range []func(*IncomingHandover){
		func(h *IncomingHandover) { h.Conversation = "other" }, func(h *IncomingHandover) { h.SourceHost = "other" },
		func(h *IncomingHandover) { h.SourceEpoch = "other" }, func(h *IncomingHandover) { h.HandoverID = "other" }, func(h *IncomingHandover) { h.DestinationHost = "other" },
	} {
		wrong := h
		change(&wrong)
		wantErr(t, a.ResolveIncomingDurably(wrong), ErrAdmissionStale)
		wantErr(t, a.Retire(), ErrAdmissionBusy)
	}
	mustOK(t, a.ResolveIncomingDurably(h))
	mustOK(t, a.Retire())
}

// A copied handle must not remove a reopened session with live work.
func TestAdmissionCopiedClosureAfterReopen(t *testing.T) {
	a, o := fixtureAdmission(t)
	c1, err := a.CloseOwner(o, nil)
	mustOK(t, err)
	copied := *c1
	mustOK(t, c1.Reopen())
	w, err := a.AdmitWork(o, ForegroundWork)
	mustOK(t, err)
	if err := copied.Relinquished(); !errors.Is(err, ErrAdmissionStale) {
		t.Errorf("copied closure relinquishment = %v, want %v", err, ErrAdmissionStale)
	}
	wantErr(t, a.Retire(), ErrAdmissionBusy)
	mustOK(t, w.Complete())
	c2, err := a.CloseOwner(o, nil)
	mustOK(t, err)
	mustOK(t, c2.Relinquished())
	mustOK(t, a.Retire())
}

func TestAdmissionClosureIdentity(t *testing.T) {
	for _, finish := range []struct {
		name string
		call func(*Closure) error
	}{
		{"reopen", (*Closure).Reopen},
		{"relinquish", (*Closure).Relinquished},
	} {
		t.Run(finish.name, func(t *testing.T) {
			a, o := fixtureAdmission(t)
			c1, err := a.CloseOwner(o, nil)
			mustOK(t, err)
			oldCopy := *c1
			// Even a copy of the currently active closure is not its identity.
			wantErr(t, finish.call(&oldCopy), ErrAdmissionStale)
			mustOK(t, c1.Reopen())
			c2, err := a.CloseOwner(o, nil)
			mustOK(t, err)
			wantErr(t, finish.call(c1), ErrAdmissionStale)
			wantErr(t, finish.call(&oldCopy), ErrAdmissionStale)
			currentCopy := *c2
			wantErr(t, finish.call(&currentCopy), ErrAdmissionStale)
			// Stale completion must neither reopen nor delete c2's closed session.
			_, err = a.AdmitWork(o, ForegroundWork)
			wantErr(t, err, ErrAdmissionClosed)
			wantErr(t, a.Retire(), ErrAdmissionBusy)
			mustOK(t, c2.Relinquished())
			wantErr(t, finish.call(&currentCopy), ErrAdmissionStale)
			mustOK(t, a.Retire())
		})
	}
}

func TestHostLifecycleTicketHeldThroughReleaseCleanup(t *testing.T) {
	a, o := fixtureAdmission(t)
	lifecycle, err := a.AdmitHostWork(PersistenceWork)
	mustOK(t, err)
	c, err := a.CloseOwner(o, nil)
	mustOK(t, err)
	// These checkpoints model caller-owned operations, not real store/OS work.
	// The caller must retain the ticket for the entire sequence, even after
	// Relinquished has removed the session from admission accounting.
	t.Run("durable release committed", func(t *testing.T) {
		wantErr(t, a.Retire(), ErrAdmissionBusy)
	})
	mustOK(t, c.Relinquished())
	t.Run("relinquished before OS unlock", func(t *testing.T) {
		wantErr(t, a.Retire(), ErrAdmissionBusy)
	})
	t.Run("OS unlocked before cleanup", func(t *testing.T) {
		wantErr(t, a.Retire(), ErrAdmissionBusy)
	})
	t.Run("cleanup finished before ticket completion", func(t *testing.T) {
		wantErr(t, a.Retire(), ErrAdmissionBusy)
	})
	mustOK(t, lifecycle.Complete())
	mustOK(t, a.Retire())
}
