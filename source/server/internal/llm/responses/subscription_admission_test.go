package responses

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/secrets"
)

type subscriptionAdmissionFake struct {
	admissionFake
	snapshots       int
	rawCalls        int
	beforeAdmission func()
}

func (f *subscriptionAdmissionFake) CredentialProfile() string { return "fixture" }
func (f *subscriptionAdmissionFake) Token(context.Context) (string, string, error) {
	f.rawCalls++
	return "raw-must-not-be-used", "raw-account", nil
}
func (f *subscriptionAdmissionFake) SubscriptionSnapshot(context.Context) (secrets.CredentialSnapshot, error) {
	f.snapshots++
	if f.beforeAdmission != nil {
		f.beforeAdmission()
	}
	return secrets.CredentialSnapshot{Payload: `{"access":"atomic-access","account_id":"atomic-account"}`, Revision: "atomic-revision"}, nil
}
func (f *subscriptionAdmissionFake) AdmitAttempt(ctx context.Context, name, revision string) error {
	if revision != "atomic-revision" {
		return errors.New("unrelated revision")
	}
	return f.admissionFake.AdmitAttempt(ctx, name, revision)
}

func TestSubscriptionAtomicAdmission(t *testing.T) {
	credentialtest.Enable(t)
	f := &subscriptionAdmissionFake{}
	c := NewClient(Config{Route: RouteChatGPT, TokenSource: f, Credentials: f, CredentialProfile: "fixture", APIKey: "must-not-fallback"})
	calls := 0
	// Inject HTTP completely: no native credentials or network transport.
	c.http = &http.Client{Transport: attemptRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if f.admissions != calls || f.snapshots != calls || f.rawCalls != 0 {
			t.Fatal("attempt reused admission or legacy material")
		}
		if r.Header.Get("Authorization") != "Bearer atomic-access" || r.Header.Get("ChatGPT-Account-Id") != "atomic-account" {
			t.Fatal("headers not from atomic payload")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
	})}
	for i := 0; i < 2; i++ {
		resp, err := c.do(context.Background(), request{})
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	f.beforeAdmission = func() { f.loggedOut = true }
	if _, err := c.do(context.Background(), request{}); err == nil {
		t.Fatal("logout between snapshot and admission allowed")
	}
	if calls != 2 || f.rawCalls != 0 {
		t.Fatal("logout fell back or transmitted")
	}
}

func TestSubscriptionModelRetryAdmission(t *testing.T) {
	credentialtest.Enable(t)
	for _, logout := range []bool{false, true} {
		f := &subscriptionAdmissionFake{}
		c := NewClient(Config{Route: RouteChatGPT, TokenSource: f, Credentials: f, CredentialProfile: "fixture"})
		calls := 0
		c.http = &http.Client{Transport: attemptRoundTripper(func(r *http.Request) (*http.Response, error) {
			calls++
			if f.admissions != calls || f.snapshots != calls || f.rawCalls != 0 {
				t.Fatal("model retry reused admission or raw token")
			}
			status, body := http.StatusOK, "data: [DONE]\n\n"
			if calls == 1 {
				status, body = http.StatusBadRequest, `{"error":{"message":"Unsupported parameter: temperature"}}`
				if logout {
					f.loggedOut = true
				}
			}
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		})}
		temperature := 0.0
		rd, err := c.StreamChat(context.Background(), llm.ChatRequest{Model: "fixture", Temperature: &temperature})
		if logout {
			if err == nil || calls != 1 || f.snapshots != 2 || f.admissions != 1 {
				t.Fatal("model retry not fenced by logout")
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			rd.Close()
			if calls != 2 {
				t.Fatal("model retry missing")
			}
		}
	}
}

func TestSubscriptionNoLegacyFallback(t *testing.T) {
	credentialtest.Enable(t)
	f := &admissionFake{}
	c := NewClient(Config{Route: RouteChatGPT, Credentials: f, CredentialProfile: "fixture", APIKey: "must-not-fallback"})
	c.http = attemptDoer(func(*http.Request) (*http.Response, error) {
		t.Fatal("unsupported subscription transmitted")
		return nil, errors.New("unexpected send")
	})
	if _, err := c.do(context.Background(), request{}); err == nil {
		t.Fatal("subscription without atomic source accepted")
	}
}
