//go:build realcloudacceptance

package responses

// Run-specific instrumentation, not a production budgeting facility. The run
// owner exclusively creates a private ledger before starting any host. No
// prompts, responses, headers, credentials or account identifiers are persisted.
import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cercano/source/server/internal/filelock"
)

type acceptanceLedger struct {
	Started  time.Time         `json:"started"`
	Attempts int               `json:"attempts"`
	Input    int64             `json:"input_tokens"`
	Output   int64             `json:"output_tokens"`
	Halted   bool              `json:"halted"`
	Active   map[int]time.Time `json:"active"`
}

var errAcceptance = errors.New("real-cloud acceptance limit or validation refusal")

func acceptanceUpdate(path string, change func(*acceptanceLedger) error) error {
	if !filepath.IsAbs(path) {
		return errAcceptance
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	unlock, err := filelock.Lock(ctx, path+".lock")
	if err != nil {
		return err
	}
	defer unlock()
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errAcceptance
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var l acceptanceLedger
	if json.Unmarshal(data, &l) != nil || l.Started.IsZero() || l.Attempts < 0 || l.Input < 0 || l.Output < 0 {
		return errAcceptance
	}
	if err = change(&l); err != nil {
		return err
	}
	data, err = json.Marshal(l)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".usage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

type acceptanceTransport struct {
	cfg  Config
	next http.RoundTripper
	path string
	now  func() time.Time
}

func acceptanceHTTPClient(cfg Config, _ *http.Client) *http.Client {
	next := cfg.BaseTransport
	if next == nil {
		next = &http.Transport{DisableKeepAlives: true, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}}
	}
	return &http.Client{Transport: &acceptanceTransport{cfg: cfg, next: next, path: os.Getenv("CERCANO_ACCEPTANCE_LEDGER"), now: time.Now}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (t *acceptanceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.cfg.Route != RouteChatGPT || t.cfg.AccountingProfile != "openai-responses" || t.cfg.Model != "gpt-6-astra" || r.URL.String() != CodexBaseURL+"/responses" || r.Method != "POST" || r.Body == nil || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
		return nil, errAcceptance
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	_ = r.Body.Close()
	if err != nil {
		return nil, err
	}
	var body struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Model != "gpt-6-astra" {
		return nil, errAcceptance
	}
	var id int
	var deadline time.Time
	err = acceptanceUpdate(t.path, func(l *acceptanceLedger) error {
		now := t.now()
		if now.Before(l.Started) || !now.Before(l.Started.Add(30*time.Minute)) || l.Halted || l.Attempts >= 40 || l.Input+l.Output >= 300000 || l.Output >= 30000 {
			return errAcceptance
		}
		for _, d := range l.Active {
			if !now.Before(d) {
				return errAcceptance
			}
		}
		deadline = now.Add(120 * time.Second)
		if end := l.Started.Add(30 * time.Minute); end.Before(deadline) {
			deadline = end
		}
		l.Attempts++
		id = l.Attempts
		if l.Active == nil {
			l.Active = map[int]time.Time{}
		}
		l.Active[id] = deadline
		return nil
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	req := r.Clone(ctx)
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.GetBody = nil
	req.ContentLength = int64(len(raw))
	finish := func(input, output int64, known bool) {
		cancel()
		// A failed ledger write leaves an outstanding deadline; no restart clears it.
		_ = acceptanceUpdate(t.path, func(l *acceptanceLedger) error {
			delete(l.Active, id)
			if !known {
				l.Halted = true
			} else {
				l.Input += input
				l.Output += output
			}
			return nil
		})
	}
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		finish(0, 0, false)
		return nil, err
	}
	resp.Body = &acceptanceBody{ReadCloser: resp.Body, stream: body.Stream, finish: finish}
	return resp, nil
}

type acceptanceBody struct {
	io.ReadCloser
	stream        bool
	pending       []byte
	input, output int64
	known, bad    bool
	once          sync.Once
	finish        func(int64, int64, bool)
}

func (b *acceptanceBody) inspect(data []byte) {
	var v struct {
		Type  string `json:"type"`
		Usage *struct {
			Input  *int64 `json:"input_tokens"`
			Output *int64 `json:"output_tokens"`
		} `json:"usage"`
		Response *struct {
			Usage *struct {
				Input  *int64 `json:"input_tokens"`
				Output *int64 `json:"output_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if json.Unmarshal(data, &v) != nil {
		return
	}
	u := v.Usage
	if b.stream {
		if v.Type != "response.completed" || v.Response == nil {
			return
		}
		u = v.Response.Usage
	}
	if u == nil || u.Input == nil || u.Output == nil || *u.Input < 0 || *u.Output < 0 {
		return
	}
	if b.known {
		b.bad = true
		return
	}
	b.input = *u.Input
	b.output = *u.Output
	b.known = true
}
func (b *acceptanceBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.pending = append(b.pending, p[:n]...)
	if b.stream {
		for {
			index := bytes.IndexByte(b.pending, '\n')
			if index < 0 {
				break
			}
			line := bytes.TrimSpace(b.pending[:index])
			if bytes.HasPrefix(line, []byte("data:")) {
				b.inspect(bytes.TrimSpace(line[5:]))
			}
			b.pending = b.pending[index+1:]
		}
	}
	if len(b.pending) > 16<<20 {
		b.bad = true
		b.pending = nil
	}
	if err == io.EOF {
		if b.stream {
			line := bytes.TrimSpace(b.pending)
			if bytes.HasPrefix(line, []byte("data:")) {
				b.inspect(bytes.TrimSpace(line[5:]))
			}
		} else {
			b.inspect(b.pending)
		}
		b.done()
	} else if err != nil {
		b.bad = true
		b.done()
	}
	return n, err
}
func (b *acceptanceBody) done() { b.once.Do(func() { b.finish(b.input, b.output, b.known && !b.bad) }) }
func (b *acceptanceBody) Close() error {
	err := b.ReadCloser.Close()
	if err != nil {
		b.bad = true
	}
	b.done()
	return err
}
