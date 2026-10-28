package responses

import (
	"cercano/source/server/internal/secrets"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type admissionFake struct {
	loggedOut  bool
	admissions int
}

func (f *admissionFake) CredentialSnapshot(context.Context, string) (secrets.CredentialSnapshot, error) {
	return secrets.CredentialSnapshot{Payload: "fixture", Revision: "revision"}, nil
}
func (f *admissionFake) AdmitAttempt(context.Context, string, string) error {
	if f.loggedOut {
		return errors.New("logged out")
	}
	f.admissions++
	return nil
}

type attemptDoer func(*http.Request) (*http.Response, error)

func (f attemptDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestCoordinatedAttemptLogout(t *testing.T) {
	f := &admissionFake{}
	c := NewClient(Config{Credentials: f, CredentialProfile: "fixture"})
	calls := 0
	c.http = attemptDoer(func(r *http.Request) (*http.Response, error) {
		calls++
		if f.admissions != calls {
			t.Fatal("attempt not admitted")
		}
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Fatal("wrong snapshot payload")
		}

		// A committed logout after admission does not cancel this attempt.
		f.loggedOut = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	resp, err := c.do(context.Background(), request{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if _, err = c.do(context.Background(), request{}); err == nil {
		t.Fatal("post-logout attempt allowed")
	}
	if calls != 1 {
		t.Fatalf("transport calls %d", calls)
	}
}

type attemptRoundTripper func(*http.Request) (*http.Response, error)

func (f attemptRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCoordinatedReplayRemovedAtWire(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://fixture.invalid", strings.NewReader("{}"))
	wire := nonReplayTransport{next: attemptRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.GetBody != nil {
			t.Fatal("wire replay enabled")
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}
	if _, err := wire.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if req.GetBody == nil {
		t.Fatal("policy body reader lost")
	}
}

func TestCoordinatedRedirectRefused(t *testing.T) {
	f := &admissionFake{}
	c := NewClient(Config{Credentials: f, CredentialProfile: "fixture"})
	client := c.http.(*http.Client)
	calls := 0
	client.Transport = attemptRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://fixture.invalid/redirect"}}, Body: http.NoBody, Request: r}, nil
	})
	resp, err := c.do(context.Background(), request{})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if calls != 1 || f.admissions != 1 || resp.StatusCode != 307 {
		t.Fatal("redirect followed")
	}
}
