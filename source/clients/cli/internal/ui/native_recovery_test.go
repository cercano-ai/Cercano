//go:build darwin

package ui

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestFullNativeRecoveryAcceptance(t *testing.T) {
	binary := os.Getenv("CERCANO_RESUME_PROBE_BINARY")
	if binary == "" {
		t.Skip("requires disposable server test binary")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("CERCANO_UI_CONFIG", filepath.Join(root, "ui.yaml"))
	type child struct {
		input  io.WriteCloser
		done   chan error
		output *bytes.Buffer
	}
	start := func(role string) (*child, DevHostSelection) {
		t.Helper()
		dir := filepath.Join(root, role)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{\"test\":true}"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		executable := filepath.Join(dir, "cercano")
		if err = os.WriteFile(executable, data, 0700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestNativeRecoveryBridge$", "-test.timeout=90s", "--", "--host", "native-recovery-"+role)
		cmd.Env = []string{"HOME=" + root, "XDG_CONFIG_HOME=" + root, "TMPDIR=" + root, "PATH=/usr/bin:/bin", "CERCANO_DISABLE_CREDENTIALS=1", "CERCANO_NATIVE_RECOVERY_ROOT=" + root, "CERCANO_NATIVE_RECOVERY_ROLE=" + role}
		input, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		c := &child{input: input, done: make(chan error, 1), output: new(bytes.Buffer)}
		cmd.Stdout = c.output
		cmd.Stderr = c.output
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { c.done <- cmd.Wait(); close(c.done) }()
		t.Cleanup(func() {
			input.Close()
			select {
			case <-c.done:
			case <-time.After(5 * time.Second):
				cmd.Process.Kill()
				select {
				case <-c.done:
				case <-time.After(5 * time.Second):
					t.Errorf("%s child did not reap after cleanup kill", role)
				}
			}
		})
		manifest := filepath.Join(root, role+".json")
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(manifest); err == nil {
				s, e := ReadDevHostSelection(manifest)
				if e != nil {
					t.Fatal(e)
				}
				return c, s
			}
			select {
			case err := <-c.done:
				t.Fatalf("%s exited: %v\n%s", role, err, c.output.String())
			default:
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s startup timeout", role)
		return nil, DevHostSelection{}
	}
	source, _ := start("source")
	destination, selected := start("destination")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m := New(nil, false).WithDevHost(selected, "native-test")
	var oldEpoch string
	for _, key := range []string{"enter", "y"} {
		_, err := reopenDevHost(ctx, selected, "native-history")
		offer, ok := err.(*forcedResumeOffer)
		if !ok {
			t.Fatalf("expected containment offer: %T %v", err, err)
		}
		if oldEpoch != "" && oldEpoch != offer.original.Operation.Owner.Epoch {
			t.Fatal("default No changed source epoch")
		}
		oldEpoch = offer.original.Operation.Owner.Epoch
		offered, _ := m.offerForcedResume(offer)
		m = offered.(Model)
		if m.pendingConfirm == nil {
			t.Fatal("missing real confirmation")
		}
		resolved, cmd := m.resolveConfirmKey(key)
		m = resolved
		if key == "enter" {
			if cmd != nil {
				t.Fatal("default No initiated recovery")
			}
			select {
			case e := <-source.done:
				t.Fatalf("No terminated source: %v", e)
			default:
			}
			continue
		}
		if cmd == nil {
			t.Fatal("yes did not start recovery")
		}
		result := cmd().(devHostResult)
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.selection.Epoch == oldEpoch || result.selection.Epoch == "" || result.selection.Identity.Host != selected.Identity.Host {
			t.Fatal("wrong recovered binding")
		}
		applied, _ := m.applyDevHost(result)
		m = applied.(Model)
		defer m.agent.Close()
	}
	select {
	case err := <-source.done:
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("source did not exit by native signal: %v", err)
		}
		status, ok := exit.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || (status.Signal() != syscall.SIGTERM && status.Signal() != syscall.SIGKILL) {
			t.Fatalf("unexpected source termination: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("source did not actually exit")
	}
	hydrated, cmd := m.Update(devHostHydrateMsg{})
	m = hydrated.(Model)
	if cmd == nil {
		t.Fatal("missing hydration command")
	}
	// Drive the real startup history and metadata commands, not a synthetic
	// transcript. Ignore unrelated recurring status/banner commands.
	batch := cmd().(tea.BatchMsg)
	for len(batch) > 0 {
		initial := batch[0]
		batch = batch[1:]
		msg := initial()
		if nested, ok := msg.(tea.BatchMsg); ok {
			batch = append(batch, nested...)
			continue
		}
		for {
			updated, _ := m.Update(msg)
			m = updated.(Model)
			stream, ok := msg.(resumeViewportStreamMsg)
			if !ok || stream.done || stream.err != nil || stream.ch == nil {
				break
			}
			msg = waitProgressiveResumeCmd(stream.ch)()
		}
	}
	if m.convID != "native-history" || m.resumeHydrating || m.resumeBackfilling || m.errMsg != "" {
		t.Fatalf("hydration failed: conv=%s hydrating=%v error=%s", m.convID, m.resumeHydrating, m.errMsg)
	}
	found := false
	for _, entry := range m.mainChat().entries {
		if strings.Contains(entry.Content, "committed native recovery history") {
			found = true
		}
	}
	if !found {
		t.Fatalf("committed history missing from restored UI: %+v", m.mainChat().entries)
	}
	again, err := reopenDevHost(ctx, selected, "native-history")
	if err != nil || again.Epoch != m.devHost.selection.Epoch {
		t.Fatalf("reconnect changed exact epoch: %+v %v", again, err)
	}
	t.Logf("native source exited; restored UI/history at exact destination epoch %s", again.Epoch)
	if _, err := io.WriteString(destination.input, again.Epoch); err != nil {
		t.Fatal(err)
	}
	destination.input.Close()
	select {
	case err := <-destination.done:
		if err != nil {
			t.Fatalf("destination: %v\n%s", err, destination.output.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("destination cleanup timeout")
	}
}
