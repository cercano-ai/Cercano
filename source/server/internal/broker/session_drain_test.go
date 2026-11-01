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
	// Low-level anonymous holders remain in eligibility accounting.
	other, err := a.AttachOwner(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "unknown", "one"} {
		want := ErrAttachmentEligibility
		if id != "one" {
			want = ErrTerminalNotAttached
		}
		if _, err = a.DrainTerminal(o, id); !errors.Is(err, want) {
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

// Exercise competing goroutines, not just the two sequential orderings above.
func TestTerminalDrainNotificationConcurrentCompletion(t *testing.T) {
	for i := 0; i < 100; i++ {
		a := NewAdmission("host")
		o := Owner{"conversation", "host", "epoch"}
		mustOK(t, a.RegisterOwner(o))
		_, err := a.AttachTerminal(o, "pane")
		mustOK(t, err)
		work, err := a.AdmitWork(o, TaskAcceptanceWork)
		mustOK(t, err)
		closure, err := a.DrainTerminal(o, "pane")
		mustOK(t, err)
		if closure.Drained() {
			t.Fatal("work drained before completion")
		}
		start := make(chan struct{})
		ready := make(chan struct{}, 2)
		results := make(chan error, 2)
		notified := make(chan bool, 2)
		// Hold the accounting lock until both contenders reach the start barrier.
		a.mu.Lock()
		go func() { ready <- struct{}{}; <-start; results <- work.Complete() }()
		go func() {
			ready <- struct{}{}
			<-start
			results <- closure.WhenDrained(func() { notified <- closure.Drained() })
		}()
		<-ready
		<-ready
		close(start)
		a.mu.Unlock()
		for j := 0; j < 2; j++ {
			select {
			case err := <-results:
				mustOK(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("completion/registration blocked")
			}
		}
		select {
		case drained := <-notified:
			if !drained {
				t.Fatal("notification before drainage")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("lost drain notification")
		}
		if err := closure.WhenDrained(func() { notified <- false }); !errors.Is(err, ErrAdmissionStale) {
			t.Fatalf("duplicate registration: %v", err)
		}
		mustOK(t, closure.Relinquished())
		mustOK(t, a.Retire())
	}
}
