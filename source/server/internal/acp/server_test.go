package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type decision struct {
	sid, tool string
	allow     bool
}
type fakeBackend struct {
	mu        sync.Mutex
	next      int
	started   chan string
	decisions chan decision
	events    map[string]chan Event
	stopped   chan string
	texts     chan string
	closed    chan struct{}
}

func newFake() *fakeBackend {
	return &fakeBackend{started: make(chan string, 10), texts: make(chan string, 10), closed: make(chan struct{}), decisions: make(chan decision, 10), events: map[string]chan Event{}, stopped: make(chan string, 10)}
}
func (b *fakeBackend) NewSession(context.Context, string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	sid := fmt.Sprintf("session-%d", b.next)
	b.events[sid] = make(chan Event, 10)
	return sid, nil
}
func (b *fakeBackend) Prompt(ctx context.Context, sid, cwd, text string) (<-chan Event, error) {
	b.texts <- text
	b.started <- sid
	go func() { <-ctx.Done(); b.stopped <- sid }()
	return b.events[sid], nil
}
func (b *fakeBackend) Permission(_ context.Context, sid, tool string, allow bool) error {
	b.decisions <- decision{sid, tool, allow}
	return nil
}
func (b *fakeBackend) Close() error { close(b.closed); return nil }

type harness struct {
	t      *testing.T
	in     *io.PipeWriter
	frames chan map[string]any
	done   chan error
}

func start(t *testing.T, b Backend) *harness {
	t.Helper()
	r, w := io.Pipe()
	or, ow := io.Pipe()
	h := &harness{t: t, in: w, frames: make(chan map[string]any, 50), done: make(chan error, 1)}
	go func() {
		d := json.NewDecoder(or)
		defer close(h.frames)
		for {
			var f map[string]any
			if d.Decode(&f) != nil {
				return
			}
			h.frames <- f
		}
	}()
	go func() {
		err := Serve(context.Background(), r, ow, func(context.Context) (Backend, error) { return b, nil })
		ow.Close()
		h.done <- err
	}()
	t.Cleanup(func() { w.Close(); or.Close() })
	return h
}
func (h *harness) send(v any) {
	h.t.Helper()
	if err := json.NewEncoder(h.in).Encode(v); err != nil {
		h.t.Fatal(err)
	}
}
func (h *harness) req(id int, method string, p any) {
	h.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": p})
}
func (h *harness) next() map[string]any {
	h.t.Helper()
	select {
	case f := <-h.frames:
		if f == nil {
			h.t.Fatal("connection closed")
		}
		return f
	case <-time.After(3 * time.Second):
		h.t.Fatal("no frame")
		return nil
	}
}
func (h *harness) init() {
	h.req(1, "initialize", map[string]any{"protocolVersion": 1})
	f := h.next()
	if f["error"] != nil {
		h.t.Fatal(f)
	}
}
func (h *harness) newSession() string {
	h.req(2, "session/new", map[string]any{"cwd": "/workspace", "mcpServers": []any{}})
	f := h.next()
	if f["error"] != nil {
		h.t.Fatal(f)
	}
	return f["result"].(map[string]any)["sessionId"].(string)
}
func (h *harness) prompt(id int, sid string) {
	h.req(id, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "hello"}}})
}
func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("channel timed out")
		var z T
		return z
	}
}
func permissionReply(id any, option string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": option}}}
}
func TestPermissionRoundTrip(t *testing.T) {
	for _, option := range []string{"allow_once", "reject_once", "allow_always", ""} {
		t.Run(option, func(t *testing.T) {
			b := newFake()
			h := start(t, b)
			h.init()
			sid := h.newSession()
			h.prompt(3, sid)
			recv(t, b.started)
			b.events[sid] <- Event{Kind: "permission", ToolID: "stable-tool", ToolName: "Write", Input: `{"path":"/x"}`}
			f := h.next()
			if f["method"] != "session/request_permission" {
				t.Fatal(f)
			}
			h.send(permissionReply(f["id"], option))
			d := recv(t, b.decisions)
			if d.sid != sid || d.tool != "stable-tool" || d.allow != (option == "allow_once") {
				t.Fatal(d)
			}
			if option == "allow_once" || option == "reject_once" {
				b.events[sid] <- Event{Kind: "done"}
			}
			f = h.next()
			if (f["error"] != nil) != (option == "allow_always" || option == "") {
				t.Fatal(f)
			}
		})
	}
}
func TestCancelAndDisconnectWhilePermissionPending(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(fmt.Sprint(disconnect), func(t *testing.T) {
			b := newFake()
			h := start(t, b)
			h.init()
			sid := h.newSession()
			h.prompt(3, sid)
			recv(t, b.started)
			b.events[sid] <- Event{Kind: "permission", ToolID: "wait"}
			_ = h.next()
			if disconnect {
				h.in.Close()
			} else {
				h.send(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": sid}})
			}
			if d := recv(t, b.decisions); d.allow || d.sid != sid {
				t.Fatal(d)
			}
			recv(t, b.stopped)
			if disconnect {
				if err := recv(t, h.done); err != nil {
					t.Fatal(err)
				}
			} else {
				f := h.next()
				if f["result"].(map[string]any)["stopReason"] != "cancelled" {
					t.Fatal(f)
				}
			}
		})
	}
}
func TestConcurrentSessionsAndToolCompletion(t *testing.T) {
	b := newFake()
	h := start(t, b)
	h.init()
	a := h.newSession()
	z := h.newSession()
	h.prompt(10, a)
	recv(t, b.started)
	h.prompt(11, z)
	recv(t, b.started)
	h.prompt(12, a)
	if f := h.next(); f["error"] == nil {
		t.Fatal(f)
	}
	b.events[a] <- Event{Kind: "tool", ToolID: "tool-1", ToolName: "Read"}
	b.events[a] <- Event{Kind: "tool_done", ToolID: "tool-1", Text: "contents", Failed: true}
	for _, kind := range []string{"tool_call", "tool_call_update"} {
		f := h.next()
		u := f["params"].(map[string]any)["update"].(map[string]any)
		if u["toolCallId"] != "tool-1" || u["sessionUpdate"] != kind {
			t.Fatal(f)
		}
		if kind == "tool_call_update" && u["status"] != "failed" {
			t.Fatal(f)
		}
	}
	b.events[z] <- Event{Kind: "text", Text: "response"}
	b.events[z] <- Event{Kind: "thought", Text: "thinking"}
	for _, kind := range []string{"agent_message_chunk", "agent_thought_chunk"} {
		f := h.next()
		if f["params"].(map[string]any)["update"].(map[string]any)["sessionUpdate"] != kind {
			t.Fatal(f)
		}
	}
	b.events[z] <- Event{Kind: "done", Text: "response"}
	if f := h.next(); f["id"] != float64(11) {
		t.Fatal(f)
	}
	b.events[a] <- Event{Kind: "done"}
	if f := h.next(); f["id"] != float64(10) {
		t.Fatal(f)
	}
}
func TestValidation(t *testing.T) {
	b := newFake()
	h := start(t, b)
	h.req(1, "session/new", map[string]any{})
	if h.next()["error"] == nil {
		t.Fatal("accepted before initialize")
	}
	h.init()
	for _, p := range []any{map[string]any{"cwd": "relative", "mcpServers": []any{}}, map[string]any{"cwd": "/x"}, map[string]any{"cwd": "/x", "mcpServers": []any{map[string]any{"name": "tool"}}}} {
		h.req(2, "session/new", p)
		if h.next()["error"] == nil {
			t.Fatal("accepted invalid new session")
		}
	}
	sid := h.newSession()
	for _, p := range []any{map[string]any{"sessionId": "foreign", "prompt": []any{}}, map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "image"}}}, map[string]any{"sessionId": sid}} {
		h.req(3, "session/prompt", p)
		if h.next()["error"] == nil {
			t.Fatal("accepted invalid prompt")
		}
	}
}
func TestAuthAndRolloverErrors(t *testing.T) {
	b := newFake()
	h := start(t, b)
	h.init()
	sid := h.newSession()
	h.prompt(3, sid)
	recv(t, b.started)
	b.events[sid] <- Event{Kind: "error", Err: fmt.Errorf("authenticate in CLI")}
	if f := h.next(); f["error"] == nil {
		t.Fatal(f)
	}
}

func TestContextCancellationUnblocksReader(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, r, io.Discard, func(context.Context) (Backend, error) { return newFake(), nil })
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cancelled connection is stuck reading input")
	}
}

func TestResourceLinkPrompt(t *testing.T) {
	b := newFake()
	h := start(t, b)
	h.init()
	sid := h.newSession()
	h.req(3, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{
		map[string]any{"type": "text", "text": "Review this reference"},
		map[string]any{"type": "resource_link", "name": "private reference", "uri": "file:///not-to-be-read/secret"},
	}})
	select {
	case text := <-b.texts:
		for _, want := range []string{"Review this reference", "private reference", "file:///not-to-be-read/secret"} {
			if !strings.Contains(text, want) {
				t.Fatalf("missing %q in backend prompt %q", want, text)
			}
		}
	case f := <-h.frames:
		t.Fatalf("resource link rejected: %v", f)
	case <-time.After(time.Second):
		t.Fatal("prompt not forwarded")
	}
	b.events[sid] <- Event{Kind: "done"}
	h.next()
	for _, block := range []any{
		map[string]any{"type": "resource_link", "uri": "file:///x"},
		map[string]any{"type": "resource_link", "name": "x"},
		map[string]any{"type": "resource_link", "name": nil, "uri": "file:///x"},
		map[string]any{"type": "resource_link", "name": "x", "uri": 42},
		map[string]any{"type": "image", "data": "abc", "mimeType": "image/png"},
		map[string]any{"type": "resource", "resource": map[string]any{"uri": "file:///x", "text": "secret"}},
		map[string]any{"type": "audio", "data": "abc", "mimeType": "audio/wav"},
	} {
		h.req(4, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{block}})
		if f := h.next(); f["error"] == nil {
			t.Fatalf("accepted invalid content: %v", block)
		}
	}
}

type observedOutput struct {
	*io.PipeWriter
	writing chan struct{}
}

func (w observedOutput) Write(p []byte) (int, error) {
	w.writing <- struct{}{}
	return w.PipeWriter.Write(p)
}

// The peer reads exactly the handshake and new-session reply, then stops.
func TestBlockedOutputShutdown(t *testing.T) {
	for _, kind := range []string{"text", "permission"} {
		for _, shutdown := range []string{"cancel", "eof", "nonclosable-cancel", "nonclosable-eof"} {
			t.Run(kind+"/"+shutdown, func(t *testing.T) {
				b := newFake()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				r, w := io.Pipe()
				or, ow := io.Pipe()
				defer w.Close()
				defer or.Close()
				defer ow.Close()
				done := make(chan error, 1)
				output := observedOutput{ow, make(chan struct{}, 3)}
				var writer io.Writer = output
				if strings.HasPrefix(shutdown, "nonclosable-") {
					writer = struct{ io.Writer }{output}
				}
				go func() { done <- Serve(ctx, r, writer, func(context.Context) (Backend, error) { return b, nil }) }()
				enc, dec := json.NewEncoder(w), json.NewDecoder(or)
				send := func(id int, method string, params any) {
					t.Helper()
					if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
						t.Fatal(err)
					}
				}
				read := func() map[string]any {
					t.Helper()
					var f map[string]any
					if err := dec.Decode(&f); err != nil {
						t.Fatal(err)
					}
					return f
				}
				send(1, "initialize", map[string]any{"protocolVersion": 1})
				read()
				send(2, "session/new", map[string]any{"cwd": "/workspace", "mcpServers": []any{}})
				sid := read()["result"].(map[string]any)["sessionId"].(string)
				send(3, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "hello"}}})
				recv(t, b.started)
				b.events[sid] <- Event{Kind: kind, Text: "blocked", ToolID: "blocked-tool"}
				// Two replies and then the unread event frame have entered Write.
				for i := 0; i < 3; i++ {
					recv(t, output.writing)
				}
				if strings.HasSuffix(shutdown, "cancel") {
					cancel()
				} else {
					w.Close()
				}
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("Serve blocked by unread output")
					or.Close()
					recv(t, done)
				}
				recv(t, b.closed)
				recv(t, b.stopped)
				if kind == "permission" {
					if d := recv(t, b.decisions); d.allow || d.tool != "blocked-tool" {
						t.Fatal(d)
					}
				}
			})
		}
	}
}
