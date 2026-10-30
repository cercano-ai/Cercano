package anthropic

import (
	"cercano/source/server/internal/credentialtest"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type endpointWire func(*http.Request) (*http.Response, error)

func (f endpointWire) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// No sockets: a regression must not send the fixture bearer to a real endpoint.
func TestOrdinarySubscriptionHonorsExplicitEndpoint(t *testing.T) {
	credentialtest.EnableHermetic(t)
	calls := 0
	c := NewClient(Config{Route: RouteSubscription, BaseURL: "https://fixture.invalid", TokenSource: stubTokens{tok: "fixture"}, BaseTransport: endpointWire(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "fixture.invalid" {
			t.Errorf("explicit ordinary endpoint overridden: %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"fixture","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))}, nil
	})})
	if _, err := c.Chat(context.Background(), ChatRequest{Model: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
