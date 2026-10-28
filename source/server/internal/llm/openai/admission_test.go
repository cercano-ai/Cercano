package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/modelpolicy"
	"cercano/source/server/internal/secrets"
)

type chatMemoryAuthority struct {
	payload, revision             string
	snapshots                     int
	snapshotErr, admissionErr     error
	afterSnapshot, afterAdmission func()
}

func (a *chatMemoryAuthority) CredentialSnapshot(context.Context, string) (secrets.CredentialSnapshot, error) {
	a.snapshots++
	s := secrets.CredentialSnapshot{Payload: a.payload, Revision: a.revision}
	if a.afterSnapshot != nil {
		a.afterSnapshot()
	}
	return s, a.snapshotErr
}
func (a *chatMemoryAuthority) AdmitAttempt(_ context.Context, _ string, revision string) error {
	if a.admissionErr != nil {
		return a.admissionErr
	}
	if a.payload == "" || revision != a.revision {
		return errors.New("stale fixture")
	}
	if a.afterAdmission != nil {
		a.afterAdmission()
	}
	return nil
}

type chatFixtureWire func(*http.Request) (*http.Response, error)

func (f chatFixtureWire) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Deterministic boundary interleavings use only an in-memory authority, never
// native storage, sockets, subprocesses, or a transport patched after construction.
func TestCoordinatedChatMemoryBoundaries(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, boundary := range []string{"rotation", "snapshot-logout", "admitted-logout"} {
			name := "Chat/"
			if stream {
				name = "Stream/"
			}
			t.Run(name+boundary, func(t *testing.T) {
				credentialtest.EnableHermetic(t)
				deny := http.DefaultTransport.(*credentialtest.ForbiddenTransport)
				a := &chatMemoryAuthority{payload: "first", revision: "1"}
				calls := 0
				expected := "first"
				wire := chatFixtureWire(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Header.Get("Authorization") != "Bearer "+expected || r.GetBody != nil {
						t.Fatal("wrong bearer or replay body")
					}
					body := `{"id":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
					content := "application/json"
					if stream {
						body = "data: [DONE]\n\n"
						content = "text/event-stream"
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {content}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				c := NewClient(Config{Credentials: a, CredentialProfile: "fixture", APIKey: "must-not-fallback", Model: "fixture", BaseTransport: wire})
				call := func() error {
					if !stream {
						_, err := c.Chat(context.Background(), llm.ChatRequest{})
						return err
					}
					r, err := c.StreamChat(context.Background(), llm.ChatRequest{})
					if err == nil {
						r.Close()
					}
					return err
				}
				logout := func() { a.payload = ""; a.revision = "2" }
				switch boundary {
				case "snapshot-logout":
					a.afterSnapshot = logout
				case "admitted-logout":
					a.afterAdmission = logout
				case "rotation":
					a.payload = "rotated"
					a.revision = "2"
					expected = "rotated"
				}
				err := call()
				if boundary == "snapshot-logout" {
					if err == nil || calls != 0 {
						t.Fatal("stale attempt dispatched")
					}
				} else if err != nil || calls != 1 {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
				logout()
				a.afterSnapshot = nil
				a.afterAdmission = nil
				before := calls
				if call() == nil || calls != before {
					t.Fatal("retained client dispatched after logout")
				}
				if deny.Calls.Load() != 0 {
					t.Fatal("default transport reached")
				}
			})
		}
	}
}

func TestCoordinatedChatSingleDispatchErrors(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, status := range []int{0, 401, 429, 500, 307} {
			t.Run(fmt.Sprintf("stream=%v/status=%d", stream, status), func(t *testing.T) {
				credentialtest.EnableHermetic(t)
				deny := http.DefaultTransport.(*credentialtest.ForbiddenTransport)
				a := &chatMemoryAuthority{payload: "fixture", revision: "1"}
				calls := 0
				c := NewClient(Config{Credentials: a, CredentialProfile: "fixture", Model: "fixture", BaseTransport: chatFixtureWire(func(r *http.Request) (*http.Response, error) {
					calls++
					if status == 0 {
						return nil, errors.New("fixture transport failure")
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}, "Location": {"https://redirect.invalid"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"fixture","type":"fixture"}}`))}, nil
				})})
				var callErr error
				if stream {
					r, err := c.StreamChat(context.Background(), llm.ChatRequest{})
					callErr = err
					if err == nil {
						r.Close()
					}
				} else {
					_, callErr = c.Chat(context.Background(), llm.ChatRequest{})
				}
				if callErr == nil {
					t.Fatal("expected failed attempt")
				}
				if calls != 1 || a.snapshots != 1 || deny.Calls.Load() != 0 {
					t.Fatalf("wire=%d snapshots=%d default=%d", calls, a.snapshots, deny.Calls.Load())
				}
			})
		}
	}
}

func TestCoordinatedChatDiagnosticRefusal(t *testing.T) {
	credentialtest.EnableHermetic(t)
	a := &chatMemoryAuthority{payload: "fixture", revision: "1"}
	wire := &credentialtest.ForbiddenTransport{}
	c := NewClient(Config{Credentials: a, CredentialProfile: "fixture", Model: "fixture", BaseTransport: wire})
	ctx := context.WithValue(context.Background(), reasoningDiagnosticKey{}, &ReasoningDiagnostic{})
	if _, err := c.Chat(ctx, llm.ChatRequest{}); err == nil {
		t.Fatal("diagnostic allowed")
	}
	if r, err := c.StreamChat(ctx, llm.ChatRequest{}); err == nil {
		r.Close()
		t.Fatal("stream diagnostic allowed")
	}
	if a.snapshots != 0 || wire.Calls.Load() != 0 || http.DefaultTransport.(*credentialtest.ForbiddenTransport).Calls.Load() != 0 {
		t.Fatal("diagnostic touched credential or wire")
	}
}

func TestCoordinatedChatPolicyBeforeStrip(t *testing.T) {
	credentialtest.EnableHermetic(t)
	wire := &credentialtest.ForbiddenTransport{}
	a := &chatMemoryAuthority{payload: "fixture", revision: "1"}
	c := NewClient(Config{Credentials: a, CredentialProfile: "fixture", Model: "fixture", BaseTransport: wire})
	checked := false
	ctx := modelpolicy.WithAuthority(context.Background(), modelpolicy.AuthorizeFunc(func(_ context.Context, a modelpolicy.Attempt) error {
		checked = true
		return modelpolicy.Deny(a, "fixture denial")
	}))
	_, err := c.Chat(ctx, llm.ChatRequest{})
	if !checked || !modelpolicy.IsDenial(err) || wire.Calls.Load() != 0 {
		t.Fatalf("checked=%v err=%v wire=%d", checked, err, wire.Calls.Load())
	}
}

func TestCoordinatedChatPolicyApproved(t *testing.T) {
	credentialtest.EnableHermetic(t)
	a := &chatMemoryAuthority{payload: "fixture", revision: "1"}
	checked, calls := 0, 0
	ctx := modelpolicy.WithAuthority(context.Background(), modelpolicy.AuthorizeFunc(func(_ context.Context, a modelpolicy.Attempt) error {
		checked++
		if a.Model != "approved" || a.Provider != "openai" || a.Endpoint != "https://fixture.invalid/v1" {
			t.Fatalf("unexpected policy attempt: %+v", a)
		}
		return nil
	}))
	c := NewClient(Config{Credentials: a, CredentialProfile: "fixture", Model: "approved", BaseURL: "https://fixture.invalid/v1", BaseTransport: chatFixtureWire(func(r *http.Request) (*http.Response, error) {
		calls++
		if checked != calls || r.GetBody != nil || r.URL.String() != "https://fixture.invalid/v1/chat/completions" {
			t.Fatal("policy order, replay body, or endpoint changed")
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "approved" || len(body.Messages) != 1 || body.Messages[0].Content != "preserved" {
			t.Fatalf("body changed: %+v, %v", body, err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))}, nil
	})})
	_, err := c.Chat(ctx, llm.ChatRequest{Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: "preserved"}}}}})
	if err != nil || checked != 1 || calls != 1 {
		t.Fatalf("checked=%d calls=%d err=%v", checked, calls, err)
	}
}

// These structural checks do not qualify actual net/http retries, TLS, or native credentials.
func TestCoordinatedChatTransportConstraints(t *testing.T) {
	credentialtest.EnableHermetic(t)
	transport := chatAttemptTransport(nil).(chatNonReplay).next.(*http.Transport)
	defer transport.CloseIdleConnections()
	if !transport.DisableKeepAlives || transport.TLSNextProto == nil || len(transport.TLSNextProto) != 0 {
		t.Fatal("persistent connections or HTTP/2 enabled")
	}
	for _, kind := range []string{"get", "nil-body", "zero-length", "unknown-length", "idempotency", "x-idempotency", "valid"} {
		t.Run(kind, func(t *testing.T) {
			r, _ := http.NewRequest("POST", "https://fixture.invalid", strings.NewReader("body"))
			switch kind {
			case "get":
				r.Method = "GET"
			case "nil-body":
				r.Body = nil
			case "zero-length":
				r.ContentLength = 0
			case "unknown-length":
				r.ContentLength = -1
			case "idempotency":
				r.Header["iDeMpOtEnCy-KeY"] = []string{""}
			case "x-idempotency":
				r.Header["x-idempotency-key"] = []string{"fixture"}
			}
			calls := 0
			_, err := chatAttemptTransport(chatFixtureWire(func(w *http.Request) (*http.Response, error) {
				calls++
				if w.GetBody != nil {
					t.Fatal("replay retained")
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			})).RoundTrip(r)
			if kind == "valid" {
				if err != nil || calls != 1 || r.GetBody == nil {
					t.Fatal("valid request changed or refused")
				}
			} else if err == nil || calls != 0 {
				t.Fatal("malformed request dispatched")
			}
		})
	}
}

type chatFixtureDoer func(*http.Request) (*http.Response, error)

func (f chatFixtureDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestCoordinatedChatAuthorityFailuresAndClone(t *testing.T) {
	credentialtest.EnableHermetic(t)
	for _, kind := range []string{"snapshot-error", "admission-error", "empty-key", "valid"} {
		t.Run(kind, func(t *testing.T) {
			a := &chatMemoryAuthority{payload: "fixture", revision: "1"}
			sentinel := errors.New("fixture authority failure")
			switch kind {
			case "snapshot-error":
				a.snapshotErr = sentinel
			case "admission-error":
				a.admissionErr = sentinel
			case "empty-key":
				a.payload = ""
			}
			calls := 0
			d := chatAttemptDoer{cfg: Config{Credentials: a, CredentialProfile: "fixture", APIKey: "no-fallback"}, next: chatFixtureDoer(func(w *http.Request) (*http.Response, error) {
				calls++
				if w.Header.Get("Authorization") != "Bearer fixture" {
					t.Fatal("wrong bearer")
				}
				return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
			})}
			r, _ := http.NewRequest("POST", "https://fixture.invalid", strings.NewReader("body"))
			r.Header.Set("Authorization", "original")
			_, err := d.Do(r)
			if r.Header.Get("Authorization") != "original" {
				t.Fatal("original authorization mutated")
			}
			if kind == "valid" {
				if err != nil || calls != 1 {
					t.Fatal("valid attempt refused")
				}
			} else {
				if err == nil || calls != 0 {
					t.Fatal("authority failure dispatched")
				}
				if kind != "empty-key" && !errors.Is(err, sentinel) {
					t.Fatalf("lost authority error: %v", err)
				}
			}
			if a.snapshots != 1 {
				t.Fatal("unexpected snapshot count")
			}
		})
	}
}
