package server

import (
	"cercano/source/server/internal/agent"
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/capabilities/agentadapter"
	"cercano/source/server/internal/capabilities/builtins"
	"cercano/source/server/internal/llm"
	"testing"
)

func TestReconAvailableInPlanningReadOnlyDispatch(t *testing.T) {
	caps := capabilities.NewRegistry(capabilities.Services{})
	builtins.Register(caps)
	tools := agenttools.NewRegistry()
	for _, name := range []string{"git_diff", "conversation_search", "conversation_retrieve"} {
		c, ok := caps.Get(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		tool := agentadapter.AsTool(c, "", caps.Services())
		if !agent.PlanProfile().Allows(llm.Permission(tool.Permission()), name) {
			t.Fatalf("planning blocked %s", name)
		}
		tools.MustRegister(tool)
	}
	srv := NewServer(nil, nil, nil, nil, nil)
	srv.SetToolRegistry(tools)
	reg, granted, ignored, e := srv.grantedRegistry(nil)
	if e != nil || len(granted) != 3 || len(ignored) != 0 {
		t.Fatalf("%v %v %v", granted, ignored, e)
	}
	for _, name := range granted {
		if !hasToolNamed(reg, name) {
			t.Fatalf("missing %s", name)
		}
	}
}
