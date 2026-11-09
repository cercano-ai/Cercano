package broker

import (
	"errors"
	"testing"
)

func TestTakeoverPreservesWorkAndFencesOldClient(t *testing.T) {
	a := NewAdmission("h")
	o := Owner{"c", "h", "e"}
	mustOK(t, a.RegisterOwner(o))
	old, err := a.AttachTerminal(o, "old")
	mustOK(t, err)
	old2, err := a.AttachTerminal(o, "old")
	mustOK(t, err)
	work, err := a.AdmitWork(o, TaskExecutionWork)
	mustOK(t, err)
	if _, err = a.AttachTerminal(o, "new"); err == nil {
		t.Fatal("implicit takeover")
	}
	if _, err = a.TakeoverTerminal(o, "new", "wrong"); err == nil {
		t.Fatal("stale confirmation")
	}
	replacement, err := a.TakeoverTerminal(o, "new", "old")
	mustOK(t, err)
	for _, ref := range []*Attachment{old, old2} {
		select {
		case <-ref.Revoked():
		default:
			t.Fatal("observer not revoked")
		}
		ref.Detach()
	}
	mustOK(t, a.ValidateTerminal(o, "new"))
	for _, action := range []func() error{
		func() error { _, e := a.AttachTerminal(o, "old"); return e },
		func() error { _, e := a.TakeoverTerminal(o, "old", "new"); return e },
		func() error { return a.CheckTerminal(o, "old") },
	} {
		if !errors.Is(action(), ErrTerminalRevoked) {
			t.Fatal("revoked client admitted")
		}
	}
	if _, err = a.CloseTerminal(o, "new"); !errors.Is(err, ErrAdmissionBusy) {
		t.Fatalf("work lost: %v", err)
	}
	mustOK(t, work.Advance(PersistenceWork))
	mustOK(t, work.Complete())
	replacement.Detach()
	fresh, err := a.AttachTerminal(o, "fresh")
	mustOK(t, err)
	fresh.Detach()
	if _, retained, open := a.CheckRetained(o); !retained || !open {
		t.Fatal("epoch lost")
	}
}

// An abandoned outgoing request may have already closed the same epoch. The
// takeover fence is epoch-bound, not owned by that request's operation ID.
func TestTakeoverQuiescesAlreadyClosedEpoch(t *testing.T) {
	a := NewAdmission("h")
	o := Owner{"c", "h", "e"}
	mustOK(t, a.RegisterOwner(o))
	ref, e := a.AttachTerminal(o, "old")
	mustOK(t, e)
	work, e := a.AdmitWork(o, PersistenceWork)
	mustOK(t, e)
	closed, e := a.ForceCloseOwner(o)
	mustOK(t, e)
	c, e := a.QuiesceOwner(o)
	mustOK(t, e)
	if c != closed {
		t.Fatal("lost existing closure")
	}
	select {
	case <-ref.Revoked():
	default:
		t.Fatal("reference not revoked")
	}
	if _, e = a.QuiesceOwner(Owner{"c", "h", "stale"}); !errors.Is(e, ErrAdmissionStale) {
		t.Fatal(e)
	}
	w, u, e := c.Pending()
	mustOK(t, e)
	if w != 1 || u != 0 {
		t.Fatalf("invented completion: %d %d", w, u)
	}
	mustOK(t, work.PersistenceFailed())
	w, u, e = c.Pending()
	mustOK(t, e)
	if w != 1 || u != 1 {
		t.Fatalf("uncertainty lost: %d %d", w, u)
	}
	if e = c.Relinquished(); !errors.Is(e, ErrAdmissionBusy) {
		t.Fatal("uncertain persistence released", e)
	}
}
