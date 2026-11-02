package broker

import (
	"errors"
	"testing"
)

func TestForcedCutoverStoppedSession(t *testing.T) {
	a := NewAdmission("A")
	o := Owner{"chat", "A", "epoch"}
	if err := a.RegisterOwner(o); err != nil {
		t.Fatal(err)
	}
	old, err := a.AttachTerminal(o, "stale-client")
	if err != nil {
		t.Fatal(err)
	}
	// No work exists. The ordinary management path nevertheless rejects this
	// stopped conversation solely because a transport reference remains.
	if _, err = a.CloseOwner(o, nil); !errors.Is(err, ErrAttachmentEligibility) {
		t.Fatalf("baseline: %v", err)
	}
	force, ok := any(a).(interface{ ForceCloseOwner(Owner) (*Closure, error) })
	if !ok {
		t.Fatal("forced cutover cannot revoke stale attachment references")
	}
	c, err := force.ForceCloseOwner(o)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-old.Revoked():
	default:
		t.Fatal("old client not revoked")
	}
	if _, err := a.AttachTerminal(o, "new-old-client"); !errors.Is(err, ErrAdmissionClosed) {
		t.Fatalf("old host admitted reconnect: %v", err)
	}
	if err := c.Relinquished(); err != nil {
		t.Fatal(err)
	}
	old.Detach() // delayed stream cleanup cannot alter the new ownership.
}

func TestForcedCutoverDoesNotInventCompletion(t *testing.T) {
	a := NewAdmission("A")
	o := Owner{"chat", "A", "epoch"}
	if err := a.RegisterOwner(o); err != nil {
		t.Fatal(err)
	}
	w, err := a.AdmitWork(o, PersistenceWork)
	if err != nil {
		t.Fatal(err)
	}
	force, ok := any(a).(interface{ ForceCloseOwner(Owner) (*Closure, error) })
	if !ok {
		t.Fatal("force closure missing")
	}
	c, err := force.ForceCloseOwner(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Relinquished(); !errors.Is(err, ErrAdmissionBusy) {
		t.Fatalf("live writer released: %v", err)
	}
	if err := w.PersistenceFailed(); err != nil {
		t.Fatal(err)
	}
	if err := w.Complete(); !errors.Is(err, ErrAdmissionBusy) {
		t.Fatalf("uncertain write discarded: %v", err)
	}
	if err := c.Relinquished(); !errors.Is(err, ErrAdmissionBusy) {
		t.Fatalf("uncertain writer released: %v", err)
	}
}
