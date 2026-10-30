package main

import (
	"bytes"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

// Deliberately inject discovery: never run the new command on live inventory.
func TestPreflightBoundary(t *testing.T) {
	for _, tc := range []struct {
		args         []string
		discoveryErr error
		code, calls  int
	}{
		{nil, nil, 2, 0},
		{[]string{"--address", "localhost:8080"}, nil, 2, 0},
		{[]string{"--address", "127.0.0.1:0"}, nil, 2, 0},
		{[]string{"--address", "[::1%lo0]:8080"}, nil, 2, 0},
		{[]string{"--address", "127.0.0.1:8080", "extra"}, nil, 2, 0},
		{[]string{"--address", "127.0.0.1:8080", "--config", "anything"}, nil, 2, 0},
		{[]string{"--address", "[::1]:8080"}, nil, 0, 1},
		{[]string{"--address", "0.0.0.0:8080"}, nil, 2, 0},
		{[]string{"--address", "127.0.0.1:8080", "--database", "anything"}, nil, 2, 0},
		{[]string{"--address", "127.0.0.1:8080"}, nil, 0, 1},
		{[]string{"--address", "127.0.0.1:8080"}, errors.New("unresolved plausible writer"), 1, 1},
	} {
		var out bytes.Buffer
		calls := 0
		code := preflight(tc.args, &out, func(endpoint netip.AddrPort) error { calls++; return tc.discoveryErr })
		if code != tc.code || calls != tc.calls {
			t.Fatalf("code=%d calls=%d output=%s", code, calls, out.String())
		}
		if calls > 0 && !strings.Contains(out.String(), "discovery elapsed=") {
			t.Fatal(out.String())
		}
	}
}
