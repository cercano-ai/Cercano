package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestEOFBlockedSynchronousReply(t *testing.T) {
	for _, method := range []string{"initialize", "session/new"} {
		t.Run(method, func(t *testing.T) {
			b := newFake()
			r, w := io.Pipe()
			defer w.Close()
			out, ow := io.Pipe()
			defer out.Close()
			output := observedOutput{ow, make(chan struct{}, 4)}
			done := make(chan error, 1)
			go func() {
				done <- Serve(context.Background(), r, output, func(context.Context) (Backend, error) { return b, nil })
			}()
			enc := json.NewEncoder(w)
			send := func(method string, p any) {
				t.Helper()
				if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": p}); err != nil {
					t.Fatal(err)
				}
			}
			if method == "session/new" {
				send("initialize", map[string]any{"protocolVersion": 1})
				var f frame
				if err := json.NewDecoder(out).Decode(&f); err != nil {
					t.Fatal(err)
				}
				recv(t, output.writing)
			}
			p := map[string]any{"protocolVersion": 1}
			if method == "session/new" {
				p = map[string]any{"cwd": "/workspace", "mcpServers": []any{}}
			}
			send(method, p)
			recv(t, output.writing) // Actual Write has begun, with no output reader.
			w.Close()
			select {
			case err := <-done:
				if !errors.Is(err, errOutputDrain) {
					t.Fatalf("undrained reply returned %v", err)
				}
			case <-time.After(time.Second):
				out.Close()
				recv(t, done)
				t.Fatal("EOF stranded synchronous reply")
			}
		})
	}
}

func TestSessionCancelBlockedOutput(t *testing.T) {
	for _, kind := range []string{"text", "permission"} {
		t.Run(kind, func(t *testing.T) {
			b := newFake()
			r, w := io.Pipe()
			defer w.Close()
			out, ow := io.Pipe()
			defer out.Close()
			output := observedOutput{ow, make(chan struct{}, 8)}
			done := make(chan error, 1)
			srv := newServer(context.Background(), func(context.Context) (Backend, error) { return b, nil })
			go func() { done <- srv.serve(r, output) }()
			enc, dec := json.NewEncoder(w), json.NewDecoder(out)
			send := func(id any, method string, p any) {
				t.Helper()
				if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": p}); err != nil {
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
			recv(t, output.writing)
			send(2, "session/new", map[string]any{"cwd": "/workspace", "mcpServers": []any{}})
			sid := read()["result"].(map[string]any)["sessionId"].(string)
			recv(t, output.writing)
			send(3, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "hello"}}})
			recv(t, b.started)
			b.events[sid] <- Event{Kind: kind, Text: "blocked", ToolID: "blocked-tool"}
			recv(t, output.writing)
			send(30, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{}})
			send(nil, "session/cancel", map[string]any{"sessionId": sid})
			if kind == "permission" {
				if d := recv(t, b.decisions); d.allow || d.tool != "blocked-tool" {
					t.Fatal(d)
				}
			}
			// Backend context ends only after the prompt worker has completed cleanup.
			recv(t, b.stopped)
			cleaned := make(chan struct{})
			go func() { srv.wg.Wait(); close(cleaned) }()
			recv(t, cleaned)
			srv.mu.Lock()
			pending, active := len(srv.waits), srv.sessions[sid].cancel != nil
			srv.mu.Unlock()
			if pending != 0 || active {
				t.Fatalf("prompt cleanup incomplete: waits=%d active=%v", pending, active)
			}
			// Cleanup clears the active prompt, but terminal capacity stays reserved
			// until the cancellation result drains; the connection remains healthy.
			srv.mu.Lock()
			reserved := srv.sessions[sid].pending
			srv.mu.Unlock()
			if !reserved || srv.ctx.Err() != nil {
				t.Fatal("lost reservation or cancelled healthy connection")
			}
			// Drain the old in-flight frame, then the preserved cancellation result.
			read()
			f := read()
			other := read()
			if f["id"] == float64(30) {
				f, other = other, f
			}
			if other["id"] != float64(30) || other["error"] == nil {
				t.Fatal(other)
			}
			if f["id"] != float64(3) || f["result"].(map[string]any)["stopReason"] != "cancelled" {
				t.Fatal(f)
			}
			send(4, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{map[string]any{"type": "text", "text": "again"}}})
			recv(t, b.started)
			b.events[sid] <- Event{Kind: "done"}
			read()
			w.Close()
			recv(t, done)
		})
	}
}

func TestFiniteInitializeDrains(t *testing.T) {
	for i := 0; i < 100; i++ {
		var out strings.Builder
		if err := Serve(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`), &out, nil); err != nil {
			t.Fatal(err)
		}
		var f frame
		if err := json.Unmarshal([]byte(out.String()), &f); err != nil || string(f.ID) != "1" {
			t.Fatalf("lost final reply: %q, %v", out.String(), err)
		}
	}
}

// A delivered permission request can be answered while a subsequent rejection
// is blocked on output. Exhausting response admission must instead fail closed.
func TestBlockedReplyPermissionDispatch(t *testing.T) {
	for _, action := range []string{"response", "saturation"} {
		t.Run(action, func(t *testing.T) {
			b := newFake()
			r, w := io.Pipe()
			defer w.Close()
			out, ow := io.Pipe()
			defer out.Close()
			output := observedOutput{ow, make(chan struct{}, 8)}
			srv := newServer(context.Background(), func(context.Context) (Backend, error) { return b, nil })
			defer srv.cancel()
			done := make(chan error, 1)
			go func() { done <- srv.serve(r, output) }()
			enc, dec := json.NewEncoder(w), json.NewDecoder(out)
			send := func(v any) {
				t.Helper()
				if err := enc.Encode(v); err != nil {
					t.Fatal(err)
				}
			}
			request := func(id int, method string, params any) {
				send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
			}
			read := func() map[string]any {
				t.Helper()
				var f map[string]any
				if err := dec.Decode(&f); err != nil {
					t.Fatal(err)
				}
				recv(t, output.writing)
				return f
			}
			request(1, "initialize", map[string]any{"protocolVersion": 1})
			read()
			request(2, "session/new", map[string]any{"cwd": "/workspace", "mcpServers": []any{}})
			sid := read()["result"].(map[string]any)["sessionId"].(string)
			request(3, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{}})
			recv(t, b.started)
			b.events[sid] <- Event{Kind: "permission", ToolID: "tool"}
			permissionID := read()["id"]
			request(4, "session/prompt", map[string]any{"sessionId": sid, "prompt": []any{}})
			recv(t, output.writing) // rejection is now blocked; stdin stays open
			if action == "response" {
				send(map[string]any{"jsonrpc": "2.0", "id": permissionID, "result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow_once"}}})
				if d := recv(t, b.decisions); !d.allow || d.tool != "tool" {
					t.Fatal(d)
				}
				if srv.ctx.Err() != nil {
					t.Fatal("healthy connection cancelled")
				}
				srv.cancel()
			} else {
				for i := 0; i <= maxPendingReplies; i++ {
					request(100+i, "unknown", nil)
				}
				if d := recv(t, b.decisions); d.allow || d.tool != "tool" {
					t.Fatal(d)
				}
			}
			recv(t, b.stopped)
			err := recv(t, done)
			if action == "saturation" && !errors.Is(err, errReplyQueueFull) {
				t.Fatalf("queue overflow: %v", err)
			}
			srv.mu.Lock()
			pending := len(srv.waits)
			srv.mu.Unlock()
			if pending != 0 {
				t.Fatalf("leaked %d permission waits", pending)
			}
		})
	}
}

func TestFiniteRepliesDrainInOrder(t *testing.T) {
	var input, output strings.Builder
	enc := json.NewEncoder(&input)
	for i := 0; i < maxPendingReplies; i++ {
		if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": i, "method": "unknown"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := Serve(context.Background(), strings.NewReader(input.String()), &output, nil); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(output.String()))
	for i := 0; i < maxPendingReplies; i++ {
		var response struct {
			ID    int
			Error *Error
		}
		if err := dec.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if response.ID != i || response.Error == nil {
			t.Fatalf("reply %d: %+v", i, response)
		}
	}
	var extra frame
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("unexpected extra response: %+v, %v", extra, err)
	}
}
