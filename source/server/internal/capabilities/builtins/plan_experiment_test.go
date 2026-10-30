package builtins

import (
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/invocationapproval"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanExperimentRejectsUnapprovedOrUnbounded(t *testing.T) {
	for _, args := range []string{`{"package":".","test":"TestOne"}`, `{"package":"./...","test":"TestOne"}`, `{"package":".","test":"Test.*"}`, `{"package":".","test":"TestOne","cmd":["sh","-c","evil"]}`, `{"package":"../outside","test":"TestOne"}`, `{"package":"-exec=sh","test":"TestOne"}`} {
		call := &capabilities.Call{Args: json.RawMessage(args)}
		if _, err := PlanExperiment().Execute(context.Background(), call); err == nil {
			t.Fatal("unapproved call accepted")
		}
		if args == `{"package":".","test":"TestOne"}` {
			continue
		}
		ctx := invocationapproval.With(context.Background(), "plan_experiment", call.Args)
		if _, err := PlanExperiment().Execute(ctx, call); err == nil {
			t.Fatalf("accepted %s", args)
		}
		if invocationapproval.Consume(ctx, "plan_experiment", call.Args) {
			t.Fatal("token not consumed")
		}
	}
}

func TestPlanExperimentApprovedInvocationAndReplay(t *testing.T) {
	dir := t.TempDir()
	bin := t.TempDir()
	// Safe executable fixture records the exact process contract, without running repository code.
	script := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"GOFLAGS=$GOFLAGS\" \"GOENV=$GOENV\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOFLAGS", "-dangerous-inherited-flag")
	t.Setenv("GOENV", "inherited")
	args := json.RawMessage(`{"package":"./fixture","test":"TestSafe"}`)
	ctx := invocationapproval.With(t.Context(), "plan_experiment", args)
	call := &capabilities.Call{Args: args, WorkDir: dir}
	result, err := PlanExperiment().Execute(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	want := "GOFLAGS=\nGOENV=off\ntest\n-count=1\n-timeout=60s\n-run=^TestSafe$\n./fixture"
	if !strings.Contains(result.Text, want) || !strings.Contains(result.Text, filepath.Base(dir)) {
		t.Fatalf("unexpected invocation: %+v", result)
	}
	if _, err := PlanExperiment().Execute(ctx, call); err == nil {
		t.Fatal("replayed invocation")
	}
}
