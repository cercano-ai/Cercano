package agentclient

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/hostsvc/persistence"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type suggestionFixtureServer struct {
	proto.UnimplementedAgentServer
	text string
}

func (s *suggestionFixtureServer) SuggestNextPrompt(_ context.Context, req *proto.SuggestNextPromptRequest) (*proto.SuggestNextPromptResponse, error) {
	return &proto.SuggestNextPromptResponse{Suggestion: persistence.SanitizeSuggestion(s.text)}, nil
}

// Exercise normalization, actual protobuf/gRPC encoding, and the client wrapper.
func TestSuggestNextPromptPreservesCompleteTextOverGRPC(t *testing.T) {
	suggestion := strings.Repeat("x", 79) + "界 finish the focused verification before checkpointing."
	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { listener.Close() })
	transport := grpc.NewServer()
	proto.RegisterAgentServer(transport, &suggestionFixtureServer{text: suggestion})
	go transport.Serve(listener)
	t.Cleanup(transport.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	client := &Client{conn: conn, agent: proto.NewAgentClient(conn)}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	got, err := client.SuggestNextPrompt(ctx, "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if got != suggestion {
		t.Fatalf("got %q, want complete %q", got, suggestion)
	}
}
