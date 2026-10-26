package server

import (
	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/pkg/proto"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestLoginStageFailureSafeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, stage string
		err         error
		want        string
	}{
		{"network", "authorization completion", &url.Error{Op: "Post", URL: "https://auth.invalid?code=secret", Err: &net.DNSError{Err: "secret", Name: "secret"}}, "could not reach the authentication service"},
		{"timeout", "authorization completion", &net.DNSError{Err: "secret", IsTimeout: true}, "network request timed out"},
		{"response shape", "authorization completion", &json.UnmarshalTypeError{Value: "secret"}, "unexpected response"},
		{"saved credentials", "provider activation", errors.New("secret"), "sign-in credentials were saved, but provider activation failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, provider := range []string{"ChatGPT", "Claude"} {
				got := loginStageFailure(provider, tc.stage, tc.err)
				if !strings.Contains(got, tc.want) || !strings.Contains(got, provider) || strings.Contains(got, "secret") || strings.Contains(got, "https://") {
					t.Fatalf("unexpected diagnostic: %s", got)
				}
			}
		})
	}
}

func TestChatGPTLoginReportsAuthorizationStartBoundary(t *testing.T) {
	credentialtest.Enable(t)
	s, _ := newTestServer()
	stream := &chatgptLoginTestStream{ctx: t.Context()}
	if err := s.runChatGPTLogin(&proto.StartChatGPTLoginRequest{}, stream, false, chatgptauth.Flow{Issuer: ":invalid"}); err != nil {
		t.Fatal(err)
	}
	last := stream.events[len(stream.events)-1]
	if !last.Done || last.Ok || !strings.Contains(last.Error, "ChatGPT authorization start:") {
		t.Fatalf("unexpected result: %+v", last)
	}
}
