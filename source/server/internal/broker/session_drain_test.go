package broker

import (
	"errors"
	"testing"
	"time"
)

func TestTerminalDrainNotification(t *testing.T) {
	for _, completeFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "registered-before-completion", true: "registered-after-completion"}[completeFirst], func(t *testing.T) {
			a := NewAdmission("host")
			o := Owner{"conversation", "host", "epoch"}
			if err := a.RegisterOwner(o); err != nil {
				t.Fatal(err)
			}
			attachment, err := a.AttachTerminal(o, "pane")
			if err != nil {
				t.Fatal(err)
			}
			parent, err := a.AdmitWork(o, TaskAcceptanceWork)
			if err != nil {
				t.Fatal(err)
			}
			c, err := a.DrainTerminal(o, "pane")
			if err != nil {
				t.Fatal(err)
			}
			if err = c.Relinquished(); !errors.Is(err, ErrAdmissionBusy) {
				t.Fatalf("early relinquishment: %v", err)
			}
			attachment.Detach()
			child, err := a.AdmitWork(o, CallbackWork)
			if err != nil {
				t.Fatal(err)
			}
			if err = parent.Complete(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			notify := func() {
				// Notification must not hold admission.mu.
				if !c.Drained() {
					t.Error("not drained")
				}
				close(done)
			}
			if completeFirst {
				if err = child.Complete(); err != nil {
					t.Fatal(err)
				}
			}
			if err = c.WhenDrained(notify); err != nil {
				t.Fatal(err)
			}
			if !completeFirst {
				if err = child.Complete(); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("notification blocked")
			}
			if err = c.WhenDrained(notify); !errors.Is(err, ErrAdmissionStale) {
				t.Fatalf("duplicate notification: %v", err)
			}
			if _, err = a.AdmitWork(o, PersistenceWork); !errors.Is(err, ErrAdmissionClosed) {
				t.Fatalf("restarted drained work: %v", err)
			}
			if err = c.Relinquished(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTerminalDrainEligibility(t *testing.T) {
	a := NewAdmission("host")
	o := Owner{"conversation", "host", "epoch"}
	if err := a.RegisterOwner(o); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AttachTerminal(o, "one"); err != nil {
		t.Fatal(err)
	}
	other, err := a.AttachTerminal(o, "two")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "unknown", "one"} {
		if _, err = a.DrainTerminal(o, id); !errors.Is(err, ErrAttachmentEligibility) {
			t.Fatalf("terminal %q: %v", id, err)
		}
	}
	other.Detach()
	if err = a.ValidateTerminal(o, "one"); err != nil {
		t.Fatalf("refusal closed admission: %v", err)
	}
	if _, err = a.DrainTerminal(o, "one"); err != nil {
		t.Fatal(err)
	}
}
