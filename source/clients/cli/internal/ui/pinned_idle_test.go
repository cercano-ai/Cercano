package ui

import (
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"context"
	"google.golang.org/grpc"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type idleListener struct {
	net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func (l *idleListener) Accept() (net.Conn, error) {
	c, e := l.Listener.Accept()
	if e == nil {
		l.mu.Lock()
		l.conns = append(l.conns, c)
		l.mu.Unlock()
	}
	return c, e
}
func (l *idleListener) drop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.conns {
		c.Close()
	}
	l.conns = nil
}
func TestPinnedIdleDisconnectUI(t *testing.T) {
	for _, mode := range []string{"transport", "host"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
			t.Setenv("CERCANO_THEMES_DIR", filepath.Join(root, "themes"))
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", "0")
			raw, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			l := &idleListener{Listener: raw}
			h := &recoveryHost{}
			s := grpc.NewServer()
			proto.RegisterAgentServer(s, h)
			done := make(chan struct{})
			go func() { defer close(done); s.Serve(l) }()
			defer func() { s.Stop(); <-done }()
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			c, e := agentclient.DialPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: l.Addr().String(), HostInstance: "A", Contract: "contract", SchemaVersion: 2, ConversationID: "conversation", OwnershipEpoch: "epoch"})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			m := New(c, false)
			m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
			// Command creation registers the observer synchronously: no scheduler sleeps.
			cmd := subscribeConnStateCmd(c)
			if mode == "host" {
				s.Stop()
			} else {
				l.drop()
			}
			for i := 0; i < 2; i++ {
				out := make(chan tea.Msg, 1)
				go func(f tea.Cmd) { out <- f() }(cmd)
				var event tea.Msg
				select {
				case event = <-out:
				case <-ctx.Done():
					t.Fatal("timeout")
				}
				msg := event.(connStateChangedMsg)
				next, _ := m.Update(msg)
				m = next.(Model)
				cmd = msg.next
				if i == 0 && !strings.Contains(m.View().Content, "reconnecting") {
					t.Fatalf("missing persistent reconnect status: %s", m.View().Content)
				}
			}
			text := ""
			for _, entry := range m.mainChat().entries {
				text += entry.Content + "\n"
			}
			if strings.Count(text, "agent disconnected") != 1 {
				t.Fatalf("missing/duplicate idle notice: %q", text)
			}
			if mode == "transport" && (m.connState != agentclient.ConnStateConnected || h.checks.Load() != 2 || !strings.Contains(text, "agent reconnected")) {
				t.Fatal("transport recovery failed")
			}
			if mode == "host" && (m.connState != agentclient.ConnStateFailed || !strings.Contains(m.View().Content, "unreachable")) {
				t.Fatal("host detection failed")
			}
			if h.requests.Load() != 0 {
				t.Fatal("automatic submission")
			}
			if mode == "host" {
				// Same endpoint and owner return. The existing send action,
				// not an SDK-only reconnect command, must recover before send.
				raw2, err := net.Listen("tcp", l.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				restored := grpc.NewServer()
				proto.RegisterAgentServer(restored, &successfulIdleHost{recoveryHost: h})
				restoredDone := make(chan struct{})
				go func() { defer close(restoredDone); restored.Serve(raw2) }()
				defer func() { restored.Stop(); <-restoredDone }()
				d := &mainAgentDriver{agent: c, convID: "conversation"}
				wait, cancel, err := d.Submit(ctx, 1, "new demand", nil)
				if err != nil {
					t.Fatal(err)
				}
				defer cancel()
				m.turnGen = 1
				m.streaming = true
				m.cancelStream = cancel
				m.mainChat().AppendEntry(&Entry{Role: RoleUser, Content: "new demand"})
				m.mainChat().AppendEntry(&Entry{Role: RoleAssistant, Streaming: true})
				queue := []tea.Cmd{wait}
				for steps := 0; m.streaming && len(queue) > 0 && steps < 30; steps++ {
					nextCmd := queue[0]
					queue = queue[1:]
					if nextCmd == nil {
						continue
					}
					event := nextCmd()
					if batch, ok := event.(tea.BatchMsg); ok {
						queue = append(queue, batch...)
						continue
					}
					next, nextCmd := m.Update(event)
					m = next.(Model)
					queue = append(queue, nextCmd)
				}
				if m.streaming || h.requests.Load() != 1 || h.checks.Load() != 2 {
					t.Fatalf("send failed: streaming=%v requests=%d checks=%d", m.streaming, h.requests.Load(), h.checks.Load())
				}
				// Consume the real connection observer's recovery events too.
				for i := 0; i < 2; i++ {
					msg := cmd().(connStateChangedMsg)
					next, _ := m.Update(msg)
					m = next.(Model)
					cmd = msg.next
				}
				text = ""
				for _, entry := range m.mainChat().entries {
					text += entry.Content + "\n"
				}
				if m.connState != agentclient.ConnStateConnected || strings.Count(text, "agent disconnected") != 1 {
					t.Fatalf("recovery status/notice: %s", text)
				}
			}

		})
	}
}

type successfulIdleHost struct{ *recoveryHost }

func (h *successfulIdleHost) StreamProcessRequest(_ *proto.ProcessRequestRequest, stream grpc.ServerStreamingServer[proto.StreamProcessResponse]) error {
	h.requests.Add(1)
	return stream.Send(&proto.StreamProcessResponse{Payload: &proto.StreamProcessResponse_FinalResponse{FinalResponse: &proto.ProcessRequestResponse{Output: "recovered"}}})
}
