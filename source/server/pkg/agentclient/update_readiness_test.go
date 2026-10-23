package agentclient

import (
	"cercano/source/server/pkg/proto"
	"context"
	"strings"
	"testing"
)

func TestUpdateReadinessEventMappingAndWithdrawal(t *testing.T) {
	token := strings.Repeat("a", 32)
	stream := &tuiEventFixture{events: []*proto.ClientEvent{
		{Event: &proto.ClientEvent_UpdateReadiness{UpdateReadiness: &proto.UpdateReadiness{OperationToken: token, Phase: proto.UpdateReadiness_DRAINING}}},
		{Event: &proto.ClientEvent_PermissionModeChanged{PermissionModeChanged: &proto.PermissionModeChanged{Mode: "strict"}}},
		{Event: &proto.ClientEvent_UpdateReadiness{UpdateReadiness: &proto.UpdateReadiness{OperationToken: token, Phase: proto.UpdateReadiness_RELEASED}}},
	}}
	out := make(chan AgentEvent, 8)
	c := &Client{}
	c.drainSubscribeEvents(context.Background(), stream, out)
	if n := (<-out).UpdateReadiness; n == nil || n.OperationToken != token || n.Phase != "DRAINING" {
		t.Fatal("missing drain", n)
	}
	if n := <-out; n.Mode != "strict" {
		t.Fatal("other event lost")
	}
	if n := (<-out).UpdateReadiness; n == nil || n.Phase != "RELEASED" {
		t.Fatal("missing release")
	}
	if n := (<-out).UpdateReadiness; n == nil || *n != (UpdateReadiness{}) {
		t.Fatal("missing empty state on stream loss")
	}
}
