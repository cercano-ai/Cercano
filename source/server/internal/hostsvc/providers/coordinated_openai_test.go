package providers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/internal/credentialtest"
	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/inference"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
)

type hostChatAuthority struct{ *hostCredentialFake }

func (a hostChatAuthority) CredentialSnapshot(context.Context, string) (secrets.CredentialSnapshot, error) {
	if a.deleted {
		return secrets.CredentialSnapshot{}, errors.New("logged out")
	}
	return secrets.CredentialSnapshot{Payload: "fixture-key", Revision: "revision"}, nil
}
func (a hostChatAuthority) AdmitAttempt(ctx context.Context, p, r string) error {
	if r != "revision" {
		return errors.New("stale")
	}
	_, err := a.CredentialSnapshot(ctx, p)
	return err
}
func TestCoordinatedOpenAIHostGraph(t *testing.T) {
	credentialtest.EnableHermetic(t)
	deny := http.DefaultTransport.(*credentialtest.ForbiddenTransport)
	authority := hostChatAuthority{&hostCredentialFake{Store: secrets.NewMemory()}}
	if err := authority.Store.Set("fixture", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	s := &service{cfgSvc: cfgsvc.New("", config.Defaults(), authority), router: &recordingRouter{}}
	calls := 0
	s.SetCloudBuilder(func(p config.CloudProfile, key string, opts ...cloudfactory.Options) (inference.Provider, error) {
		if len(opts) != 1 || opts[0].Credentials == nil || opts[0].TokenSource != nil || opts[0].AnthropicTokenSource != nil {
			t.Fatal("unexpected capability")
		}
		opts[0].OpenAIBaseTransport = hostAnthropicWire(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Header.Get("Authorization") != "Bearer fixture-key" || r.GetBody != nil {
				t.Fatal("bad wire request")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))}, nil
		})
		return cloudfactory.BuildCloudProvider(p, key, opts...)
	})
	p, err := s.buildProfile(config.CloudProfile{Name: "fixture", Flavor: cloudfactory.FlavorChatCompletions, Model: "fixture", ModelPinned: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Chat(context.Background(), llm.ChatRequest{}); err != nil {
		t.Fatal(err)
	}
	authority.Delete("fixture")
	if _, err = p.Chat(context.Background(), llm.ChatRequest{}); err == nil {
		t.Fatal("retained client bypassed logout")
	}
	if calls != 1 || deny.Calls.Load() != 0 {
		t.Fatalf("wire=%d default=%d", calls, deny.Calls.Load())
	}
}
