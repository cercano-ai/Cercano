package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

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
