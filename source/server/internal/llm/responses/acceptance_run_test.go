//go:build realcloudacceptance

package responses

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type acceptanceWire func(*http.Request) (*http.Response, error)

func (f acceptanceWire) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func acceptanceFixture(t *testing.T) (string, *acceptanceTransport) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "usage.json")
	data, _ := json.Marshal(acceptanceLedger{Started: time.Now()})
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p, &acceptanceTransport{path: p, now: time.Now, cfg: Config{Route: RouteChatGPT, AccountingProfile: "openai-responses", Model: "gpt-6-astra"}, next: acceptanceWire(func(r *http.Request) (*http.Response, error) {
		if r.GetBody != nil {
			t.Error("replay enabled")
		}
		d, ok := r.Context().Deadline()
		if !ok || time.Until(d) > 120*time.Second {
			t.Error("deadline missing")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}}\n\n"))}, nil
	})}
}
func acceptanceRequest(t *testing.T, tr *acceptanceTransport) error {
	t.Helper()
	r, err := http.NewRequestWithContext(context.Background(), "POST", CodexBaseURL+"/responses", strings.NewReader(`{"model":"gpt-6-astra","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tr.RoundTrip(r)
	if err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func TestAcceptanceLimits(t *testing.T) {
	p, tr := acceptanceFixture(t)
	for i := 0; i < 40; i++ {
		if err := acceptanceRequest(t, tr); err != nil {
			t.Fatal(err)
		}
	}
	if acceptanceRequest(t, tr) == nil {
		t.Fatal("attempt 41 admitted")
	}
	data, _ := os.ReadFile(p)
	var l acceptanceLedger
	_ = json.Unmarshal(data, &l)
	if l.Attempts != 40 || l.Input != 440 || l.Output != 280 || len(l.Active) != 0 {
		t.Fatalf("bad totals: %+v", l)
	}
	for _, tc := range []struct {
		name   string
		change func(*acceptanceLedger)
	}{
		{"input", func(l *acceptanceLedger) { l.Input = 300000 }},
		{"output", func(l *acceptanceLedger) { l.Output = 30000 }},
		{"time", func(l *acceptanceLedger) { l.Started = time.Now().Add(-30 * time.Minute) }},
		{"missing usage", func(l *acceptanceLedger) { l.Halted = true }},
		{"orphan", func(l *acceptanceLedger) { l.Active = map[int]time.Time{1: time.Now().Add(-time.Second)} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, tr := acceptanceFixture(t)
			if err := acceptanceUpdate(p, func(l *acceptanceLedger) error { tc.change(l); return nil }); err != nil {
				t.Fatal(err)
			}
			if acceptanceRequest(t, tr) == nil {
				t.Fatal("admitted")
			}
		})
	}
}
func TestAcceptanceConcurrentAndUnknownUsage(t *testing.T) {
	p, tr := acceptanceFixture(t)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copy := *tr
			if acceptanceRequest(t, &copy) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 40 {
		t.Fatalf("admitted %d", successes.Load())
	}
	p, tr = acceptanceFixture(t)
	tr.next = acceptanceWire(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"error":"refused"}`))}, nil
	})
	_ = acceptanceRequest(t, tr)
	if acceptanceRequest(t, tr) == nil {
		t.Fatal("continued after missing usage")
	}
	data, _ := os.ReadFile(p)
	if strings.Contains(string(data), "refused") {
		t.Fatal("response leaked")
	}
}
func TestAcceptanceRouteAndTransport(t *testing.T) {
	_, tr := acceptanceFixture(t)
	tr.cfg.Model = "other"
	if acceptanceRequest(t, tr) == nil {
		t.Fatal("other model admitted")
	}
	c := acceptanceHTTPClient(Config{}, nil)
	wire := c.Transport.(*acceptanceTransport).next.(*http.Transport)
	if !wire.DisableKeepAlives || wire.ForceAttemptHTTP2 || wire.TLSNextProto == nil || wire.Proxy != nil {
		t.Fatal("unsafe default transport")
	}
	if c.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect enabled")
	}
}

func TestAcceptanceProcess(t *testing.T) {
	if p := os.Getenv("ACCEPTANCE_COUNTER_CHILD"); p != "" {
		for i := 0; i < 12; i++ {
			_ = acceptanceUpdate(p, func(l *acceptanceLedger) error {
				if l.Attempts >= 40 {
					return errAcceptance
				}
				l.Attempts++
				return nil
			})
		}
		return
	}
	p, _ := acceptanceFixture(t)
	var children []*exec.Cmd
	for i := 0; i < 4; i++ {
		c := exec.Command(os.Args[0], "-test.run=^TestAcceptanceProcess$")
		c.Env = append(os.Environ(), "ACCEPTANCE_COUNTER_CHILD="+p, "CERCANO_DISABLE_CREDENTIALS=1")
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, c)
	}
	for _, c := range children {
		if err := c.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(p)
	var l acceptanceLedger
	_ = json.Unmarshal(data, &l)
	if l.Attempts != 40 {
		t.Fatalf("cross-process attempts %d", l.Attempts)
	}
}
func TestAcceptanceDeadlineAndPartial(t *testing.T) {
	p, tr := acceptanceFixture(t)
	_ = acceptanceUpdate(p, func(l *acceptanceLedger) error { l.Started = time.Now().Add(-30*time.Minute + time.Second); return nil })
	tr.next = acceptanceWire(func(r *http.Request) (*http.Response, error) {
		d, ok := r.Context().Deadline()
		if !ok || time.Until(d) > time.Second {
			t.Error("overall deadline not propagated")
		}
		return nil, context.DeadlineExceeded
	})
	if acceptanceRequest(t, tr) == nil {
		t.Fatal("timeout success")
	}
	if acceptanceRequest(t, tr) == nil {
		t.Fatal("timeout did not halt")
	}
	_, tr = acceptanceFixture(t)
	r, _ := http.NewRequest("POST", CodexBaseURL+"/responses", strings.NewReader(`{"model":"gpt-6-astra","stream":true}`))
	resp, err := tr.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if acceptanceRequest(t, tr) == nil {
		t.Fatal("closed stream missing usage did not halt")
	}
}
