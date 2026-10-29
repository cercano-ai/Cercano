package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerArgs(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", t.TempDir())
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "failures.jsonl")
	for _, tc := range []struct {
		args     []string
		prepared bool
	}{
		{[]string{"--socket", "/tmp/worker.sock"}, false},
		{[]string{"--socket", "/tmp/worker.sock", "--prepared", "--failure-log", path, "--routing-log", path}, true},
		{[]string{"--failure-log", path, "--routing-log", path, "--prepared", "--socket", "/tmp/worker.sock"}, true},
	} {
		socket, prepared, failureLog, routingLog, err := parseWorkerArgs(tc.args)
		if err != nil || socket != "/tmp/worker.sock" || prepared != tc.prepared || (prepared && (failureLog != path || routingLog != path)) {
			t.Fatalf("args=%v socket=%q prepared=%v err=%v", tc.args, socket, prepared, err)
		}
	}
	for _, args := range [][]string{
		nil, {"--socket", "/tmp/s", "--prepared"},
		{"--socket", "/tmp/s", "--prepared", "--failure-log", path},
		{"--socket", "/tmp/s", "--prepared", "--routing-log", path},
		{"--socket", "/tmp/s", "--routing-log", path},
		{"--socket", "/tmp/s", "--prepared", "--failure-log", path, "--routing-log", "relative"},
		{"--socket", "/tmp/s", "--prepared", "--failure-log", path, "--routing-log", path, "--routing-log", path}, {"--socket", "/tmp/s", "--failure-log", path, "--routing-log", path}, {"--socket", "/tmp/s", "--prepared", "--failure-log", "relative"}, {"--socket"}, {"--socket", ""}, {"--prepared"},
		{"--socket", "--prepared"}, {"--socket", "/tmp/s", "extra"},
		{"--socket", "/tmp/s", "--prepard"}, {"--socket", "/tmp/s", "--prepared=false"},
		{"--socket", "/tmp/s", "--prepared", "--prepared"},
		{"--socket", "/tmp/s", "--socket", "/tmp/t"},
	} {
		if _, _, _, _, err := parseWorkerArgs(args); err == nil {
			t.Errorf("accepted invalid args %v", args)
		}
	}
}
