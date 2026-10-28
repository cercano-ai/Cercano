package tools

import (
	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/capabilities/agentadapter"
	"cercano/source/server/internal/capabilities/builtins"
	"cercano/source/server/internal/dispatch"
	"cercano/source/server/internal/inference"
	"testing"
)

func TestRunCommandLegacyDispatchGrant(t *testing.T) {
	caps := capabilities.NewRegistry(capabilities.Services{})
	builtins.Register(caps)
	svc := &Service{toolRegistry: agentadapter.BuildAgentRegistry(caps, builtins.AgentAliases(), builtins.CapabilitySynonyms())}
	for _, name := range []string{"RunCommand", "Bash", "mcp__oc__Bash"} {
		reg, granted, ignored, err := svc.GrantedRegistry([]string{name})
		if err != nil || len(ignored) != 0 || len(granted) != 1 || granted[0] != "RunCommand" {
			t.Fatalf("%s: %v %v %v", name, granted, ignored, err)
		}
		for _, lookup := range []string{"Bash", "RunCommand"} {
			tool, ok := reg.Get(lookup)
			if !ok || tool.Permission() != agenttools.PermW {
				t.Fatalf("lost legacy permission: %s", lookup)
			}
		}
		if _, ok := reg.Get("Write"); ok {
			t.Fatal("ungranted tool leaked")
		}
	}
}

func TestDispatchGrantCannotDelegateAgain(t *testing.T) {
	caps := capabilities.NewRegistry(capabilities.Services{})
	builtins.Register(caps)
	svc := &Service{toolRegistry: agentadapter.BuildAgentRegistry(caps, builtins.AgentAliases(), builtins.CapabilitySynonyms())}
	for _, name := range []string{"dispatch", "workflow", "mcp__oc__workflow"} {
		if _, _, _, err := svc.GrantedRegistry([]string{"Read", name}); err == nil {
			t.Errorf("nested delegation grant accepted: %s", name)
		}
	}
	reg, _, _, err := svc.GrantedRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dispatch", "workflow"} {
		if _, ok := reg.Get(name); ok {
			t.Errorf("default grant contains %s", name)
		}
	}
}

func TestReconnaissanceRejectsWriteGrantBeforeProvider(t *testing.T) {
	caps := capabilities.NewRegistry(capabilities.Services{})
	builtins.Register(caps)
	svc := &Service{toolRegistry: agentadapter.BuildAgentRegistry(caps, builtins.AgentAliases(), builtins.CapabilitySynonyms())}
	_, err := svc.RunAgenticDispatch(t.Context(), dispatch.Spec{RoutingTask: "reconnaissance", Tools: []string{"Edit"}}, inference.Selection{}, "")
	if err == nil {
		t.Fatal("write grant accepted")
	}
}
