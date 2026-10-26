package server

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/chatgptauth"
	"cercano/source/server/internal/cloudfactory"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pasteLoginStream plays a headless client: it never touches the loopback and
// instead submits pasted codes under the login id from the first frame.
type pasteLoginStream struct {
	proto.Agent_ReauthenticateCloudServer
	ctx     context.Context
	server  *Server
	pastes  func(state string) []string
	events  []*proto.CloudLoginEvent
	replies chan *proto.SubmitClaudeLoginCodeResponse
}

func (s *pasteLoginStream) Context() context.Context { return s.ctx }
func (s *pasteLoginStream) Send(e *proto.CloudLoginEvent) error {
	s.events = append(s.events, e)
	if e.LoginId == "" {
		return nil
	}
	manual, err := url.Parse(e.ManualAuthorizeUrl)
	if err != nil {
		return err
	}
	pastes := s.pastes(manual.Query().Get("state"))
	s.replies = make(chan *proto.SubmitClaudeLoginCodeResponse, len(pastes))
	go func() {
		defer close(s.replies)
		for _, code := range pastes {
			reply, err := s.server.SubmitClaudeLoginCode(s.ctx, &proto.SubmitClaudeLoginCodeRequest{LoginId: e.LoginId, Code: code})
			if err != nil {
				reply = &proto.SubmitClaudeLoginCodeResponse{Error: "rpc: " + err.Error()}
			}
			s.replies <- reply
		}
	}()
	return nil
}

func TestClaudeLoginCompletesFromPastedCode(t *testing.T) {
	credentialtest.Enable(t)
	var exchanged map[string]string
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&exchanged)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"pasted-access","refresh_token":"pasted-refresh","expires_in":3600}`))
	}))
	defer endpoint.Close()

	cfg := config.Defaults()
	cfg.CloudProfiles = []config.CloudProfile{{Name: "named", Flavor: cloudfactory.FlavorMessages, Route: cloudfactory.RouteSubscription}}
	server := &Server{cfgSvc: cfgsvc.New("", cfg, secrets.NewMemory())}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stream := &pasteLoginStream{ctx: ctx, server: server, pastes: func(state string) []string {
		return []string{"typo#not-this-attempt", "pasted-code#" + state}
	}}
	err := server.runCloudReauthentication(&proto.CloudReauthenticationRequest{ProfileName: "named", AttemptId: "attempt-1"}, stream, anthropicauth.Flow{TokenURL: endpoint.URL + "/token"}, chatgptauth.Flow{})
	if err != nil {
		t.Fatal(err)
	}

	var replies []*proto.SubmitClaudeLoginCodeResponse
	if stream.replies == nil {
		t.Fatalf("login never opened code inbox: events=%+v", stream.events)
	}
collect:
	for {
		select {
		case r, ok := <-stream.replies:
			if !ok {
				break collect
			}
			replies = append(replies, r)
		case <-ctx.Done():
			t.Fatalf("waiting for code replies: %v", ctx.Err())
		}
	}
	if len(replies) != 2 || replies[0].Accepted || !strings.Contains(replies[0].Error, "isn't a code for this sign-in") || !replies[1].Accepted {
		t.Fatalf("replies=%+v, want rejected typo then accepted code", replies)
	}
	if len(stream.events) != 2 || !stream.events[1].Done || !stream.events[1].Ok {
		t.Fatalf("events=%+v", stream.events)
	}
	if first := stream.events[0]; first.AuthorizeUrl == "" || first.ManualAuthorizeUrl == "" {
		t.Fatalf("first frame missing URLs: %+v", first)
	}
	if exchanged["code"] != "pasted-code" || exchanged["redirect_uri"] != anthropicauth.DefaultManualRedirectURL {
		t.Fatalf("exchange body=%+v, want pasted code against the manual redirect", exchanged)
	}
	raw, err := server.cfgSvc.Secrets().Get("named")
	if err != nil || !strings.Contains(raw, "pasted-access") {
		t.Fatalf("credential save failed: %v", err)
	}

	// The inbox is gone once the sign-in finishes.
	_, err = server.SubmitClaudeLoginCode(ctx, &proto.SubmitClaudeLoginCodeRequest{LoginId: stream.events[0].LoginId, Code: "late"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("submit after completion err=%v, want NotFound", err)
	}
}

func TestSubmitClaudeLoginCodeUnknownLogin(t *testing.T) {
	server := &Server{}
	_, err := server.SubmitClaudeLoginCode(context.Background(), &proto.SubmitClaudeLoginCodeRequest{LoginId: "nope", Code: "c#s"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("err=%v, want NotFound", err)
	}
}
