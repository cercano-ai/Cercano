package builtins

import (
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/invocationapproval"
	"context"
	"encoding/json"
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
