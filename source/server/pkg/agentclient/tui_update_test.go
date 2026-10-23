package agentclient

import (
	"context"
	"io"
	"testing"

	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type tuiEventFixture struct {
	grpc.ClientStream
	events []*proto.ClientEvent
}

func (s *tuiEventFixture) Recv() (*proto.ClientEvent, error) {
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, nil
}
func TestTUIUpdateEventDecodingKeepsOtherEvents(t *testing.T) {
	stream := &tuiEventFixture{events: []*proto.ClientEvent{
		{}, // An unknown forward-compatible event remains ignorable.
		{Event: &proto.ClientEvent_TuiUpdateInstalled{TuiUpdateInstalled: &proto.TuiUpdateInstalled{Version: "1.2.3", OperationId: 7}}},
		{Event: &proto.ClientEvent_PermissionModeChanged{PermissionModeChanged: &proto.PermissionModeChanged{Mode: "strict"}}},
	}}
	out := make(chan AgentEvent, 4)
	c := &Client{}
	c.drainSubscribeEvents(context.Background(), stream, out)
	first, ok := <-out
	if !ok || first.TUIUpdate == nil || first.TUIUpdate.Version != "1.2.3" || first.TUIUpdate.OperationID != 7 || first.Err != nil {
		t.Fatal("update event lost", first)
	}
	second, ok := <-out
	if !ok || second.Mode != "strict" || second.TUIUpdate != nil {
		t.Fatal("other event lost", second)
	}
	if cleared := <-out; cleared.UpdateReadiness == nil || cleared.UpdateReadiness.OperationToken != "" {
		t.Fatal("missing live-state withdrawal")
	}
	if _, ok = <-out; ok {
		t.Fatal("stream not closed")
	}
}
