package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"cercano/source/server/pkg/agentclient"
)

// Finish each harness before inspecting its decision queue: absence assertions
// then have a worker/connection completion barrier rather than a timing window.
func extraFinish(t *testing.T, h *harness, b *fakeBackend) {
	t.Helper()
	if err := h.in.Close(); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, h.done); err != nil {
		t.Fatal(err)
	}
	select {
	case d := <-b.decisions:
		t.Fatalf("unexpected extra permission decision: %+v", d)
	default:
	}
}

func extraPermission(t *testing.T, h *harness, b *fakeBackend, sid string) map[string]any {
	t.Helper()
	b.events[sid] <- Event{Kind: "permission", ToolID: "shared-tool", ToolName: "Write"}
	f := h.next()
	if f["method"] != "session/request_permission" || f["params"].(map[string]any)["sessionId"] != sid {
		t.Fatalf("unexpected permission: %v", f)
	}
	return f
}

func extraDecision(t *testing.T, b *fakeBackend, sid string, allow bool) {
	t.Helper()
	if d := recv(t, b.decisions); d.sid != sid || d.tool != "shared-tool" || d.allow != allow {
		t.Fatalf("decision=%+v, want sid=%s allow=%v", d, sid, allow)
	}
}

func extraPromptResult(t *testing.T, h *harness, id int, stop string) {
	t.Helper()
	f := h.next()
	if f["id"] != float64(id) || f["error"] != nil {
		t.Fatalf("unexpected prompt response: %v", f)
	}
	if f["result"].(map[string]any)["stopReason"] != stop {
		t.Fatal(f)
	}
}

func TestExtraPermissionFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   any
		rpcError bool
		valid    bool
	}{
		{name: "unknown_option", result: map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "unadvertised"}}},
		{name: "unknown_outcome", result: map[string]any{"outcome": map[string]any{"outcome": "approved", "optionId": "allow_once"}}},
		{name: "malformed_result", result: "allow_once"},
		{name: "missing_outcome", result: map[string]any{}},
		{name: "null_result", result: nil},
		{name: "rpc_error_overrides_allow", result: map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow_once"}}, rpcError: true},
		{name: "cancelled_outcome", result: map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newFake()
			h := start(t, b)
			h.init()
			sid := h.newSession()
			h.prompt(3, sid)
			recv(t, b.started)
			p := extraPermission(t, h, b, sid)
			reply := map[string]any{"jsonrpc": "2.0", "id": p["id"], "result": tc.result}
			if tc.rpcError {
				reply["error"] = map[string]any{"code": -32603, "message": "client permission UI failed"}
			}
			h.send(reply)
			extraDecision(t, b, sid, false)
			if tc.valid {
				b.events[sid] <- Event{Kind: "done"}
				extraPromptResult(t, h, 3, "end_turn")
			} else {
				f := h.next()
				e, ok := f["error"].(map[string]any)
				if f["id"] != float64(3) || !ok || e["code"] != float64(-32603) || !strings.Contains(e["message"].(string), "invalid permission response") {
					t.Fatal(f)
				}
			}
			extraFinish(t, h, b)
		})
	}
}

func TestExtraPermissionSameToolAcrossSessions(t *testing.T) {
	b := newFake()
	h := start(t, b)
	h.init()
	a := h.newSession()
	z := h.newSession()
	h.prompt(10, a)
	recv(t, b.started)
	h.prompt(11, z)
	recv(t, b.started)
	pa := extraPermission(t, h, b, a)
	pz := extraPermission(t, h, b, z)
	if pa["id"] == pz["id"] {
		t.Fatal("permission request IDs reused across sessions")
	}
	// Resolve in reverse request order with opposite decisions for the same tool.
	h.send(permissionReply(pz["id"], "allow_once"))
	extraDecision(t, b, z, true)
	h.send(permissionReply(pa["id"], "reject_once"))
	extraDecision(t, b, a, false)
	b.events[z] <- Event{Kind: "done"}
	extraPromptResult(t, h, 11, "end_turn")
	b.events[a] <- Event{Kind: "done"}
	extraPromptResult(t, h, 10, "end_turn")
	extraFinish(t, h, b)
}

func TestExtraStalePermissionResponses(t *testing.T) {
	for _, kind := range []string{"unknown", "duplicate", "late_after_cancel"} {
		t.Run(kind, func(t *testing.T) {
			b := newFake()
			h := start(t, b)
			h.init()
			sid := h.newSession()
			stale := any("acp-permission-unknown")
			if kind != "unknown" {
				h.prompt(3, sid)
				recv(t, b.started)
				p := extraPermission(t, h, b, sid)
				stale = p["id"]
				if kind == "late_after_cancel" {
					h.send(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": sid}})
					extraDecision(t, b, sid, false)
					extraPromptResult(t, h, 3, "cancelled")
				} else {
					h.send(permissionReply(stale, "reject_once"))
					extraDecision(t, b, sid, false)
					b.events[sid] <- Event{Kind: "done"}
					extraPromptResult(t, h, 3, "end_turn")
				}
			}
			h.prompt(4, sid)
			recv(t, b.started)
			current := extraPermission(t, h, b, sid)
			if current["id"] == stale {
				t.Fatal("permission ID reused")
			}
			h.send(permissionReply(stale, "allow_once"))
			// A synchronous response is a reader barrier after the unsolicited reply.
			h.req(90, "unsupported/barrier", nil)
			if f := h.next(); f["id"] != float64(90) || f["error"] == nil {
				t.Fatal(f)
			}
			h.send(permissionReply(current["id"], "reject_once"))
			extraDecision(t, b, sid, false)
			b.events[sid] <- Event{Kind: "done"}
			extraPromptResult(t, h, 4, "end_turn")
			extraFinish(t, h, b)
		})
	}
}

type extraFailBackend struct {
	*fakeBackend
	newErr, promptErr error
}

func (b *extraFailBackend) NewSession(ctx context.Context, cwd string) (string, error) {
	if b.newErr != nil {
		return "", b.newErr
	}
	return b.fakeBackend.NewSession(ctx, cwd)
}
func (b *extraFailBackend) Prompt(ctx context.Context, sid, cwd, text string) (<-chan Event, error) {
	if b.promptErr != nil {
		return nil, b.promptErr
	}
	return b.fakeBackend.Prompt(ctx, sid, cwd, text)
}

func TestExtraBackendCreationFailures(t *testing.T) {
	for _, stage := range []string{"connect", "new"} {
		t.Run(stage, func(t *testing.T) {
			b := &extraFailBackend{fakeBackend: newFake()}
			sentinel := errors.New("injected " + stage + " failure")
			if stage == "new" {
				b.newErr = sentinel
			}
			calls := 0
			input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}
{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/workspace","mcpServers":[]}}
{"jsonrpc":"2.0","id":3,"method":"session/new","params":{"cwd":"/workspace","mcpServers":[]}}
`
			var out bytes.Buffer
			err := Serve(context.Background(), strings.NewReader(input), &out, func(context.Context) (Backend, error) {
				calls++
				if stage == "connect" {
					return nil, sentinel
				}
				return b, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			dec := json.NewDecoder(&out)
			for id := 1; id <= 3; id++ {
				var f map[string]any
				if err := dec.Decode(&f); err != nil {
					t.Fatal(err)
				}
				if f["id"] != float64(id) {
					t.Fatal(f)
				}
				if id == 1 {
					if f["error"] != nil {
						t.Fatal(f)
					}
					continue
				}
				e, ok := f["error"].(map[string]any)
				if !ok || e["code"] != float64(-32603) || !strings.Contains(e["message"].(string), sentinel.Error()) {
					t.Fatal(f)
				}
			}
			var trailing any
			if err := dec.Decode(&trailing); err != io.EOF {
				t.Fatalf("extra output: %v (%v)", trailing, err)
			}
			want := 1
			if stage == "connect" {
				want = 2
			}
			if calls != want {
				t.Fatalf("connect calls=%d, want %d", calls, want)
			}
			if stage == "new" {
				recv(t, b.closed)
			}
		})
	}
}

func TestExtraBackendPromptFailureAndPrematureClose(t *testing.T) {
	for _, stage := range []string{"prompt_failure", "stream_close", "adapter_stream_close"} {
		t.Run(stage, func(t *testing.T) {
			b := newFake()
			var backend Backend = b
			var client *streamClient
			if stage == "prompt_failure" {
				backend = &extraFailBackend{fakeBackend: b, promptErr: errors.New("injected prompt failure")}
			}
			if stage == "adapter_stream_close" {
				client = &streamClient{messages: make(chan agentclient.StreamMsg)}
				backend = NewAgentBackend(context.Background(), client)
			}
			h := start(t, backend)
			h.init()
			sid := h.newSession()
			h.prompt(3, sid)
			if stage == "stream_close" {
				recv(t, b.started)
				close(b.events[sid])
			}
			if client != nil {
				close(client.messages)
			}
			f := h.next()
			e, ok := f["error"].(map[string]any)
			want := "without completion"
			if stage == "prompt_failure" {
				want = "injected prompt failure"
			}
			if f["id"] != float64(3) || !ok || e["code"] != float64(-32603) || !strings.Contains(e["message"].(string), want) {
				t.Fatal(f)
			}
			extraFinish(t, h, b)
		})
	}
}
