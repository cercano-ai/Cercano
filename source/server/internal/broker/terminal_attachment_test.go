package broker

import (
	"sync"
	"testing"
)

func TestTerminalReferences(t *testing.T) {
	a := NewAdmission("h")
	o := Owner{"c", "h", "e"}
	if err := a.RegisterOwner(o); err != nil {
		t.Fatal(err)
	}
	x, _ := a.AttachTerminal(o, "x")
	y, _ := a.AttachTerminal(o, "x")
	if err := a.ValidateTerminal(o, "pane"); err != ErrAttachmentEligibility {
		t.Fatal(err)
	}
	z, _ := a.AttachTerminal(o, "z")
	if _, err := a.CloseTerminal(o, "x"); err != ErrAttachmentEligibility {
		t.Fatal(err)
	}
	z.Detach()
	c, err := a.CloseTerminal(o, "x")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Reopen(); err != nil {
		t.Fatal(err)
	}
	x.Detach()
	x.Detach()
	if err := a.ValidateTerminal(o, "x"); err != nil {
		t.Fatal(err)
	}
	y.Detach()
	if _, retained, open := a.CheckRetained(o); !retained || !open {
		t.Fatal("detach released ownership")
	}
	if _, err := a.CloseTerminal(o, "x"); err != ErrAttachmentEligibility {
		t.Fatal(err)
	}
	c, err = a.CloseTerminal(o, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Relinquished(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AttachTerminal(o, "x"); err != ErrAdmissionStale {
		t.Fatal(err)
	}
	if err := c.Reopen(); err != ErrAdmissionStale {
		t.Fatal(err)
	}
	o.Epoch = "new"
	if err := a.RegisterOwner(o); err != nil {
		t.Fatal(err)
	}
	if err := a.ValidateTerminal(o, "x"); err != ErrAttachmentEligibility {
		t.Fatal(err)
	}
}

func TestTerminalAttachClosureRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		a := NewAdmission("h")
		o := Owner{"c", "h", "e"}
		_ = a.RegisterOwner(o)
		x, _ := a.AttachTerminal(o, "x")
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var z *Attachment
		var attachErr, closeErr error
		var c *Closure
		go func() { defer wg.Done(); <-start; z, attachErr = a.AttachTerminal(o, "z") }()
		go func() { defer wg.Done(); <-start; c, closeErr = a.CloseTerminal(o, "x") }()
		close(start)
		wg.Wait()
		if closeErr == nil {
			if attachErr != ErrAdmissionClosed {
				t.Fatalf("attach escaped closure: %v", attachErr)
			}
			_ = c.Relinquished()
		} else {
			if closeErr != ErrAttachmentEligibility || attachErr != nil {
				t.Fatalf("%v %v", closeErr, attachErr)
			}
			z.Detach()
		}
		x.Detach()
	}
}
