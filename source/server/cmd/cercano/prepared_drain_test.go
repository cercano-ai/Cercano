package main

import (
	"context"
	"google.golang.org/grpc"
	"testing"
	"time"
)

func TestPreparedDrainKeepsStatusUntilResourcesFinish(t *testing.T) {
	gate := newProductionAdmissionGate()
	rpcEntered, rpcRelease, rpcDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	resourcesEntered, resourcesRelease := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(rpcDone)
		_, _ = gate.unary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "ordinary"}, func(context.Context, any) (any, error) { close(rpcEntered); <-rpcRelease; return nil, nil })
	}()
	<-rpcEntered
	h := newPreparedShutdown(gate.close, func() error { gate.drain(); close(resourcesEntered); <-resourcesRelease; return nil })
	h.Request()
	if s := h.Control(true); !s.Accepted || s.Complete {
		t.Fatal(s)
	}
	select {
	case <-resourcesEntered:
		t.Fatal("disposed before RPC drain")
	default:
	}
	close(rpcRelease)
	<-rpcDone
	select {
	case <-resourcesEntered:
	case <-time.After(time.Second):
		t.Fatal("drain stuck")
	}
	if s := h.Control(true); !s.Accepted || s.Complete {
		t.Fatal(s)
	}
	close(resourcesRelease)
	if err := h.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !h.Control(true).Complete {
		t.Fatal("not complete")
	}
}
