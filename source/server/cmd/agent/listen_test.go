package main

import "testing"

func TestAgentListensOnLoopbackOnly(t *testing.T) {
	if got := listenAddr("50052"); got != "127.0.0.1:50052" {
		t.Fatalf("listenAddr(50052) = %q, want 127.0.0.1:50052", got)
	}
}
