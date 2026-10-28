package builtins

import (
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/invocationapproval"
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

type planExperimentCap struct{}

func PlanExperiment() capabilities.Capability            { return planExperimentCap{} }
func (planExperimentCap) Name() string                   { return "plan_experiment" }
func (planExperimentCap) Tier() capabilities.Tier        { return capabilities.TierX }
func (planExperimentCap) Surfaces() capabilities.Surface { return capabilities.SurfaceAgent }
func (planExperimentCap) Description() string {
	return "Request approval for exactly one bounded Go test invocation while staying in planning. Runs go test with fixed count=1 and timeout=60s, one local package and one named test. Tests execute repository code and may have side effects; this is not a sandbox. No shell, flags, environment overrides, or reusable approval. Every invocation requires fresh confirmation."
}
func (planExperimentCap) Schema() capabilities.Schema {
	return capabilities.Schema(`{"type":"object","additionalProperties":false,"required":["package","test"],"properties":{"package":{"type":"string","description":"One local package: . or ./path (no recursive wildcard)"},"test":{"type":"string","description":"Exact Go test function name, beginning Test"}}}`)
}

type planExperimentArgs struct {
	Package string `json:"package"`
	Test    string `json:"test"`
}

var experimentPackage = regexp.MustCompile(`^(\.|\./[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*)$`)
var experimentTest = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

func (c planExperimentCap) Execute(ctx context.Context, call *capabilities.Call) (*capabilities.Result, error) {
	if !invocationapproval.Consume(ctx, c.Name(), call.Args) {
		return nil, errors.New("plan_experiment: exact single-invocation approval required")
	}
	var a planExperimentArgs
	if err := decodeDeclaredArguments(call.Args, c, &a); err != nil {
		return nil, err
	}
	if !experimentPackage.MatchString(a.Package) || !experimentTest.MatchString(a.Test) {
		return nil, errors.New("plan_experiment: requires one local package and one exact Test name")
	}
	args, _ := json.Marshal(runCommandArgs{Cmd: []string{"go", "test", "-count=1", "-timeout=60s", "-run=^" + a.Test + "$", a.Package}, TimeoutSeconds: 90, Env: map[string]string{"GOFLAGS": "", "GOENV": "off"}})
	child := *call
	child.Args = args
	return (runCommandCap{}).Execute(ctx, &child)
}
