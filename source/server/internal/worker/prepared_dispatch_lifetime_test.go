package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	persistsvc "cercano/source/server/internal/hostsvc/persistence"
)

func TestPreparedDispatchLifetimeDrainsNestedProducers(t *testing.T) {
	d := &preparedDispatchLifetime{}
	child, finishChild, err := d.begin(context.Background(), "root", "child")
	if err != nil {
		t.Fatal(err)
	}
	joined := make(chan error, 1)
	go func() { joined <- d.join() }()
	// Observe the admission fence without relying on scheduling sleeps.
	deadline := time.After(time.Second)
	for {
		d.mu.Lock()
		fenced := d.closed
		d.mu.Unlock()
		if fenced {
			break
		}
		select {
		case <-deadline:
			t.Fatal("join never fenced root")
		default:
		}
	}
	if _, _, err := d.begin(context.Background(), "root", "late"); err == nil {
		t.Fatal("late root admitted")
	}
	// A still-running child can finish its own nested dispatch after the root returns.
	grandchild, finishGrandchild, err := d.begin(child, "child", "grandchild")
	if err != nil {
		t.Fatal(err)
	}
	if err := finishChild(nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.begin(child, "child", "late"); err == nil {
		t.Fatal("completed producer admitted work")
	}
	select {
	case <-joined:
		t.Fatal("joined before descendant callback")
	default:
	}
	failure := errors.New("descendant durable write failed")
	persistsvc.ReportWriteFailure(grandchild, failure)
	if err := finishGrandchild(nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := finishGrandchild(nil); !errors.Is(err, failure) {
		t.Fatal("completion not idempotent", err)
	}
	select {
	case err := <-joined:
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("join did not finish")
	}
	if _, _, err := d.begin(grandchild, "grandchild", "late"); err == nil {
		t.Fatal("closed lifetime admitted work")
	}
}
