package agentadapter_test

import (
	"testing"

	"cercano/source/server/internal/agenttools"
	"cercano/source/server/internal/capabilities"
	"cercano/source/server/internal/capabilities/agentadapter"
	"cercano/source/server/internal/capabilities/builtins"
)

func TestDeepResearchMapsToWritePermission(t *testing.T) {
	tool := agentadapter.AsTool(builtins.DeepResearch(), "DeepResearch", capabilities.Services{})
	if got := tool.Permission(); got != agenttools.PermW {
		t.Fatalf("DeepResearch permission = %v, want W", got)
	}
}
