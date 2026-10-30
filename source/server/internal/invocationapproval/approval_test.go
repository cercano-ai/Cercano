package invocationapproval

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestExactSingleUse(t *testing.T) {
	args := []byte(`{"test":"TestOne"}`)
	ctx := With(context.Background(), "plan_experiment", args)
	args[0] = '!'
	if Consume(ctx, "plan_experiment", args) {
		t.Fatal("mutable args authorized")
	}
	if Consume(ctx, "plan_experiment", []byte(`{"test":"TestOne"}`)) {
		t.Fatal("mismatch did not burn token")
	}
	ctx = With(context.Background(), "plan_experiment", args)
	var n atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if Consume(ctx, "plan_experiment", args) {
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	if n.Load() != 1 {
		t.Fatalf("consumed %d times", n.Load())
	}
	if Consume(context.Background(), "plan_experiment", args) {
		t.Fatal("unapproved")
	}
}
