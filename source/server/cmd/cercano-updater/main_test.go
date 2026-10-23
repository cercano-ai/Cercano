package main

import (
	"bytes"
	"testing"
)

func TestBoundedInvocation(t *testing.T) {
	for _, args := range [][]string{nil, {"execute", "--input", "/tmp/input"}, {"version", "extra"}, {"--help"}, {"execute"}} {
		var out, diagnostic bytes.Buffer
		if code := run(args, &out, &diagnostic); code == 0 || out.Len() != 0 || diagnostic.Len() == 0 {
			t.Fatalf("%q: code=%d out=%q diagnostic=%q", args, code, out.String(), diagnostic.String())
		}
	}
	var out bytes.Buffer
	if code := run([]string{"version"}, &out, &out); code != 0 || out.String() != "cercano-updater v"+version+"\n" {
		t.Fatal(code, out.String())
	}
}
