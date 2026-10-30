package agentclient

import (
	"cercano/source/server/pkg/approvaldisclosure"
	"cercano/source/server/pkg/proto"
	"encoding/json"
	"testing"
)

func TestExperimentDisclosureAdapter(t *testing.T) {
	args := `{"package":".","test":"TestSafe"}`
	warning := approvaldisclosure.ForTool("plan_experiment", json.RawMessage(args), t.TempDir())
	msg := decodeStreamResponse(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_PermissionRequired{PermissionRequired: &proto.PermissionRequired{ToolUseId: "one", ToolName: "plan_experiment", ArgsJson: args, Tier: "X", Disclosure: warning}}})
	if msg == nil || msg.Type != TypePermissionRequired || msg.Disclosure != warning || msg.ArgsJSON != args {
		t.Fatalf("lost disclosure or exact args: %+v", msg)
	}
}
