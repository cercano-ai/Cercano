package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"cercano/source/server/pkg/agentclient"
)

func TestChatGPTLoginModalTransitions(t *testing.T) {
	mo := newChatGPTLoginModal("chatgpt", "gpt-5.3-codex")
	if mo.state != chatgptLoginWaiting {
		t.Fatalf("initial state = %d, want waiting", mo.state)
	}
	mo.setCode("https://auth.openai.com/codex/device", "ABCD-1234")
	if mo.userCode != "ABCD-1234" || mo.verificationURL == "" {
		t.Errorf("setCode didn't record code/url: %+v", mo)
	}
	mo.setDone("acct-9")
	if mo.state != chatgptLoginDone || mo.accountID != "acct-9" {
		t.Errorf("setDone: state=%d acct=%q", mo.state, mo.accountID)
	}
	if mo.cancel != nil {
		t.Error("setDone must clear cancel")
	}
}

func TestChatGPTLoginModalSetFailed(t *testing.T) {
	mo := newChatGPTLoginModal("chatgpt", "")
	mo.setFailed("declined by user")
	if mo.state != chatgptLoginFailed || mo.errMsg != "declined by user" {
		t.Errorf("setFailed: state=%d err=%q", mo.state, mo.errMsg)
	}
}

// TestDrainChatGPTLoginClosedChannel verifies a channel that closes without a
// terminal frame yields a synthetic failure frame so the modal never hangs.
func TestDrainChatGPTLoginClosedChannel(t *testing.T) {
	ch := make(chan agentclient.ChatGPTLoginMsg)
	close(ch)
	msg := drainChatGPTLoginCmd(ch)()
	frame, ok := msg.(chatgptLoginFrameMsg)
	if !ok {
		t.Fatalf("want chatgptLoginFrameMsg, got %T", msg)
	}
	if !frame.frame.Done || frame.frame.Ok {
		t.Errorf("closed channel should yield done+failed frame: %+v", frame.frame)
	}
}

// TestDrainChatGPTLoginDeliversFrame verifies a normal frame is passed through
// with the channel retained for re-draining.
func TestDrainChatGPTLoginDeliversFrame(t *testing.T) {
	ch := make(chan agentclient.ChatGPTLoginMsg, 1)
	ch <- agentclient.ChatGPTLoginMsg{UserCode: "WXYZ-9", VerificationURL: "u"}
	msg := drainChatGPTLoginCmd(ch)()
	frame := msg.(chatgptLoginFrameMsg)
	if frame.frame.UserCode != "WXYZ-9" {
		t.Errorf("frame not passed through: %+v", frame.frame)
	}
	if frame.ch == nil {
		t.Error("channel should be retained for re-drain on a non-closed stream")
	}
}

func waitingChatGPTModalWithCode() *chatgptLoginModal {
	mo := newChatGPTLoginModal("chatgpt", "")
	mo.setCode("https://auth.openai.com/codex/device", "ABCD-1234")
	return mo
}

// Over SSH the opener would run on the remote host: it fails silently or starts
// a text-mode browser in this terminal. The printed URL and code cover it.
func TestChatGPTLoginAutoOpenSkippedWhenHeadless(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "1 2 3 4")
	t.Setenv("SSH_TTY", "/dev/pts/1")
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	mo := waitingChatGPTModalWithCode()
	if cmd := mo.autoOpenBrowser(); cmd != nil || mo.browserOpened {
		t.Fatal("auto-open attempted over headless SSH")
	}
}

func TestChatGPTLoginAutoOpenOnceWithDisplay(t *testing.T) {
	for _, k := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(k, "")
	}
	t.Setenv("DISPLAY", ":0")
	mo := waitingChatGPTModalWithCode()
	if cmd := mo.autoOpenBrowser(); cmd == nil || !mo.browserOpened {
		t.Fatal("expected one auto-open where a browser can show up")
	}
	if cmd := mo.autoOpenBrowser(); cmd != nil {
		t.Fatal("auto-open fired twice")
	}
}

// The verification URL and code are shown for copying, often over SSH, so the
// terminal must own the mouse while they are on screen.
func TestChatGPTLoginModalReleasesMouseForSelection(t *testing.T) {
	m := New(nil, false)
	m.width, m.height = 140, 45
	m.chatgptLoginModal = newChatGPTLoginModal("chatgpt", "")
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("before the code arrives, mouse mode = %v, want cell motion", got)
	}
	m.chatgptLoginModal = waitingChatGPTModalWithCode()
	if got := m.View().MouseMode; got != tea.MouseModeNone {
		t.Fatalf("with the code showing, mouse mode = %v, want none", got)
	}
	m.chatgptLoginModal.setDone("acct")
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("after sign-in finished, mouse mode = %v, want cell motion", got)
	}
}

// Exercise the root update path, not just modal setters: terminal frames and
// dismissal must restore mouse reporting after the selectable code disappears.
func TestChatGPTLoginMouseRestoredAfterTerminalEvents(t *testing.T) {
	for _, event := range []string{"success", "failure", "stream error", "dismissal"} {
		t.Run(event, func(t *testing.T) {
			m := New(nil, false)
			m.width, m.height = 140, 45
			m.chatgptLoginModal = waitingChatGPTModalWithCode()
			canceled := false
			m.chatgptLoginModal.cancel = func() { canceled = true }
			if got := m.View().MouseMode; got != tea.MouseModeNone {
				t.Fatalf("waiting mouse mode = %v", got)
			}
			rendered := ansi.Strip(m.chatgptLoginModal.View(m.styles, m.palette, m.width, m.height))
			for _, text := range []string{"https://auth.openai.com/codex/device", "ABCD-1234"} {
				if !strings.Contains(rendered, text) {
					t.Fatalf("missing selectable %q in %s", text, rendered)
				}
			}
			var msg tea.Msg
			switch event {
			case "success":
				msg = chatgptLoginFrameMsg{frame: agentclient.ChatGPTLoginMsg{Done: true, Ok: true, AccountID: "acct"}}
			case "failure":
				msg = chatgptLoginFrameMsg{frame: agentclient.ChatGPTLoginMsg{Done: true, Error: "denied"}}
			case "stream error":
				msg = chatgptLoginFrameMsg{frame: agentclient.ChatGPTLoginMsg{Err: errors.New("disconnected")}}
			case "dismissal":
				msg = tea.KeyPressMsg{Code: tea.KeyEsc}
			}
			updated, _ := m.Update(msg)
			m = updated.(Model)
			if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
				t.Fatalf("after %s mouse mode = %v", event, got)
			}
			if event == "dismissal" {
				if m.chatgptLoginModal != nil || !canceled {
					t.Fatal("dismissal did not close and cancel sign-in")
				}
			} else {
				want := chatgptLoginFailed
				if event == "success" {
					want = chatgptLoginDone
				}
				if m.chatgptLoginModal.state != want {
					t.Fatalf("state = %v, want %v", m.chatgptLoginModal.state, want)
				}
				updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				m = updated.(Model)
				if m.chatgptLoginModal != nil || m.View().MouseMode != tea.MouseModeCellMotion {
					t.Fatal("settled dismissal did not preserve mouse reporting")
				}
			}
		})
	}
}

// Do not execute the returned browser command: testing scheduling requires no
// live browser or authentication. Repeated device frames must not launch twice.
func TestChatGPTLoginFrameBrowserScheduling(t *testing.T) {
	for _, ssh := range []bool{true, false} {
		name := "local"
		if ssh {
			name = "ssh"
		}
		t.Run(name, func(t *testing.T) {
			for _, k := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY", "DISPLAY", "WAYLAND_DISPLAY"} {
				t.Setenv(k, "")
			}
			if ssh {
				t.Setenv("SSH_CONNECTION", "1 2 3 4")
			} else {
				t.Setenv("DISPLAY", ":0")
			}
			m := New(nil, false)
			m.chatgptLoginModal = newChatGPTLoginModal("chatgpt", "")
			frame := chatgptLoginFrameMsg{frame: agentclient.ChatGPTLoginMsg{VerificationURL: "https://auth.openai.com/codex/device", UserCode: "ABCD-1234"}}
			updated, cmd := m.Update(frame)
			m = updated.(Model)
			if (cmd != nil) != !ssh || m.chatgptLoginModal.browserOpened != !ssh {
				t.Fatal("incorrect browser scheduling for", name)
			}
			updated, cmd = m.Update(frame)
			m = updated.(Model)
			if cmd != nil {
				t.Fatal("repeated frame scheduled browser")
			}
			if m.View().MouseMode != tea.MouseModeNone {
				t.Fatal("device code not selectable")
			}
		})
	}
}
