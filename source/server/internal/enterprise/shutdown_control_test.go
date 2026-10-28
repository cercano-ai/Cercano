package enterprise

import (
	"cercano/source/server/pkg/proto"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
)

func TestShutdownControlUsesPolicyAdmission(t *testing.T) {
	h := &Host{closed: true}
	called := false
	_, err := h.UnaryInterceptor()(context.Background(), &proto.ShutdownAgentRequest{StatusOnly: true}, &grpc.UnaryServerInfo{FullMethod: proto.Agent_ShutdownAgent_FullMethodName}, func(context.Context, any) (any, error) { called = true; return nil, nil })
	if called || status.Code(err) != codes.PermissionDenied || status.Convert(err).Message() != "shutdown control denied" {
		t.Fatalf("control bypassed policy: called=%v err=%v", called, err)
	}
}
