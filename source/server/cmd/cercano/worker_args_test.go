package main

import "testing"

func TestWorkerArgs(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		prepared bool
	}{
		{[]string{"--socket", "/tmp/worker.sock"}, false},
		{[]string{"--socket", "/tmp/worker.sock", "--prepared"}, true},
		{[]string{"--prepared", "--socket", "/tmp/worker.sock"}, true},
	} {
		socket, prepared, err := parseWorkerArgs(tc.args)
		if err != nil || socket != "/tmp/worker.sock" || prepared != tc.prepared {
			t.Fatalf("args=%v socket=%q prepared=%v err=%v", tc.args, socket, prepared, err)
		}
	}
	for _, args := range [][]string{
		nil, {"--socket"}, {"--socket", ""}, {"--prepared"},
		{"--socket", "--prepared"}, {"--socket", "/tmp/s", "extra"},
		{"--socket", "/tmp/s", "--prepard"}, {"--socket", "/tmp/s", "--prepared=false"},
		{"--socket", "/tmp/s", "--prepared", "--prepared"},
		{"--socket", "/tmp/s", "--socket", "/tmp/t"},
	} {
		if _, _, err := parseWorkerArgs(args); err == nil {
			t.Errorf("accepted invalid args %v", args)
		}
	}
}
