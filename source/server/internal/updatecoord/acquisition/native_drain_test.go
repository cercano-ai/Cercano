package acquisition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/launch"
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"cercano/source/server/internal/updatecoord/selection"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/config"
)

type nativeDrainInstall struct{ root, id, stateRoot, stateDir, updater string }

// nativeUpdaterDrain deliberately uses only public process/RPC APIs. The input
// supplies identities, not authority: execute independently reinspects all three.
func nativeUpdaterDrain(t *testing.T, client *agentclient.Client, agentPID int, cliImage string, cliArgs, env []string, home, denyProfile, port string, install nativeDrainInstall, blocked <-chan struct{}, unblock func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	defer unblock()
	before, err := os.ReadFile(filepath.Join(install.stateDir, selection.FileName))
	if err != nil {
		t.Fatal(err)
	}
	waitFile := func(path string) []byte {
		t.Helper()
		for ctx.Err() == nil {
			if b, e := os.ReadFile(path); e == nil && len(b) > 0 {
				return b
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("waiting for %s: %v", path, ctx.Err())
		return nil
	}
	type terminal struct {
		control  string
		identity nativeprocess.Identity
		done     chan error
		log      string
	}
	var deferCleanup []func()
	defer func() {
		for i := len(deferCleanup) - 1; i >= 0; i-- {
			deferCleanup[i]()
		}
	}()
	startCLI := func(cliPort string) terminal {
		t.Helper()
		control := filepath.Join(home, fmt.Sprintf("pty-%d", time.Now().UnixNano()))
		python, e := exec.LookPath("python3")
		if e != nil {
			t.Fatal(e)
		}
		profile := denyProfile + fmt.Sprintf(`(allow network-outbound (remote ip "localhost:%s"))(deny process-exec (regex #"/cercano$"))`, cliPort)
		argv := []string{"-c", nativePTYHelper, "Inert native controlled PTY; do not use tools.", "/usr/bin/sandbox-exec", "-p", profile, cliImage}
		argv = append(argv, cliArgs...)
		cmd := exec.Command(python, argv...)
		cmd.Env = append(append([]string{}, env...), "TERM=xterm-256color", "NATIVE_PTY_CONTROL="+control)
		cmd.Dir = home
		log, e := os.Create(control + ".log")
		if e != nil {
			t.Fatal(e)
		}
		cmd.Stdout, cmd.Stderr = log, log
		if e = cmd.Start(); e != nil {
			log.Close()
			t.Fatal(e)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait(); log.Close(); close(done) }()
		var identity nativeprocess.Identity
		deferCleanup = append(deferCleanup, func() {
			_ = os.WriteFile(control+".abort", []byte("cleanup, not voluntary exit"), 0600)
			select {
			case <-done:
			case <-time.After(8 * time.Second):
				// The helper owns the CLI and its finally block kills/reaps it;
				// do not kill the helper first and orphan the actual terminal.
				if identity.PID == 0 {
					t.Error("PTY cleanup identity unknown; refusing to signal")
				} else if id, e := nativeprocess.Inspect(identity.PID); e != nil || !id.SameProcess(identity) {
					t.Errorf("PTY cleanup lifetime unverified; refusing to signal: %v", e)
				} else {
					p, e := os.FindProcess(identity.PID)
					if e != nil {
						t.Error(e)
					} else if e = p.Kill(); e != nil {
						t.Error(e)
					}
				}
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("PTY helper did not reap child")
				}
			}
		})
		t.Cleanup(func() {
			if t.Failed() {
				b, _ := os.ReadFile(control + ".log")
				t.Logf("PTY log: %s", b)
			}
		})
		waitFile(control + ".ready")
		pid, e := strconv.Atoi(string(waitFile(control + ".pid")))
		if e != nil {
			t.Fatal(e)
		}
		identity, e = nativeprocess.Inspect(pid)
		if e != nil {
			t.Fatal(e)
		}
		if identity.Executable != cliImage {
			t.Fatalf("CLI image: %+v", identity)
		}
		return terminal{control, identity, done, control + ".log"}
	}
	quit := func(c terminal) {
		t.Helper()
		if err := os.WriteFile(c.control+".quit", []byte("voluntary user /quit"), 0600); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-c.done:
			if err != nil {
				b, _ := os.ReadFile(c.log)
				t.Fatalf("CLI exit: %v %s", err, b)
			}
		case <-ctx.Done():
			t.Fatal("CLI exit deadline")
		}
	}
	events, err := client.SubscribeEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	awaitReadiness := func(token string, released bool) {
		t.Helper()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for {
			select {
			case event, ok := <-events:
				if !ok {
					t.Fatal("SubscribeEvents closed before acknowledgment")
				}
				n := event.UpdateReadiness
				if n == nil || n.OperationToken != token {
					continue
				}
				if (!released && (n.Phase == "DRAINING" || n.Phase == "IDLE")) || (released && (n.Phase == "RELEASED" || n.Phase == "CANCELLED")) {
					t.Logf("SubscribeEvents token=%s phase=%s (release is diagnostic, not admission proof)", token, n.Phase)
					return
				}
			case <-timer.C:
				t.Fatal("SubscribeEvents acknowledgment timeout; no voluntary CLI exit authorized")
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	first := startCLI(port)
	// A transparent harness-owned TCP relay permits a real completed CLI chat,
	// then closes EVERY transport and its listener. No agent hook or fake client.
	relay, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, relayPort, _ := net.SplitHostPort(relay.Addr().String())
	var mu sync.Mutex
	var connections []net.Conn
	closed := false
	disconnect := func() {
		mu.Lock()
		defer mu.Unlock()
		closed = true
		relay.Close()
		for _, c := range connections {
			c.Close()
		}
	}
	defer disconnect()
	go func() {
		for {
			c, e := relay.Accept()
			if e != nil {
				return
			}
			target, e := net.Dial("tcp4", "127.0.0.1:"+port)
			if e != nil {
				c.Close()
				return
			}
			mu.Lock()
			if closed {
				c.Close()
				target.Close()
				mu.Unlock()
				return
			}
			connections = append(connections, c, target)
			mu.Unlock()
			go func() { io.Copy(target, c) }()
			go func() { io.Copy(c, target) }()
		}
	}()
	configPath := filepath.Join(home, ".config/cercano/config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Port = relayPort
	if err = config.Save(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	second := startCLI(relayPort)
	cfg.Port = port
	if err = config.Save(cfg, configPath); err != nil {
		t.Fatal(err)
	}
	disconnect()
	observed, err := nativeprocess.Inspect(second.identity.PID)
	if err != nil || !observed.SameProcess(second.identity) {
		t.Fatalf("disconnected CLI not live: %v", err)
	}

	stream, err := client.StreamChat(ctx, fmt.Sprintf("blocked-%d", time.Now().UnixNano()), "native deliberately blocked; do not use tools.", home)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal("fixture was not admitted")
	}
	agent, err := nativeprocess.Inspect(agentPID)
	if err != nil {
		t.Fatal(err)
	}
	type participant struct {
		Role     string                 `json:"role"`
		Identity nativeprocess.Identity `json:"identity"`
	}
	participants := []participant{{"agent", agent}, {"client", first.identity}, {"client", second.identity}}
	for attempt := 0; attempt < 2; attempt++ {
		dir, err := os.MkdirTemp(home, "operation-")
		if err != nil {
			t.Fatal(err)
		}
		token := fmt.Sprintf("%032x", time.Now().UnixNano())
		input := map[string]any{"install_root": install.root, "install_id": install.id, "state_root": install.stateRoot, "operation_token": token, "agent_endpoint": "127.0.0.1:" + port, "participants": participants, "status_file": "status.json"}
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		inputPath := filepath.Join(dir, "operation.json")
		if err = os.WriteFile(inputPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		logPath := filepath.Join(dir, "updater.log")
		// sandbox-exec execs the acquired image; fixed argv, regular log, no parent lock.
		profile := denyProfile + fmt.Sprintf(`(allow network-outbound (remote ip "localhost:%s"))`, port)
		owner, err := launch.Launch(launch.Options{Executable: "/usr/bin/sandbox-exec", Argv: []string{"-p", profile, install.updater, "execute", "--input", inputPath}, Env: env, Dir: home, StdoutPath: logPath, StderrPath: logPath})
		if err != nil {
			t.Fatal(err)
		}
		reaped := false
		defer func() {
			if !reaped {
				select {
				case <-owner.Done():
				default:
					p, _ := os.FindProcess(owner.Pid())
					p.Kill()
				}
				cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				owner.WaitForCompletion(cleanup)
			}
			if t.Failed() {
				b, _ := os.ReadFile(logPath)
				t.Logf("actual updater log: %s", b)
			}
		}()
		statusPath := filepath.Join(dir, "status.json")
		var status struct {
			Token        string `json:"operation_token"`
			State        string `json:"state"`
			Participants int    `json:"participants"`
		}
		for {
			b, e := os.ReadFile(statusPath)
			if e == nil && json.Unmarshal(b, &status) == nil {
				break
			}
			select {
			case <-owner.Done():
				b, _ := os.ReadFile(logPath)
				t.Fatalf("updater exited before status: %s", b)
			case <-ctx.Done():
				t.Fatal("status deadline")
			default:
				time.Sleep(25 * time.Millisecond)
			}
		}
		info, err := os.Lstat(statusPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			t.Fatalf("unprotected status: %v", err)
		}
		if status.Token != token || status.State != "owner_evidence_sealed_incomplete" || status.Participants != len(participants) {
			t.Fatalf("status: %+v", status)
		}
		t.Log("owner-evidence status recorded; status alone does not authorize voluntary exit")
		awaitReadiness(token, false)
		if attempt == 0 {
			waitFile(first.control + ".notice")
			select {
			case <-first.done:
				t.Fatal("initiating CLI exited before acknowledged voluntary exit")
			default:
			}
		}
		updaterID, err := nativeprocess.Inspect(owner.Pid())
		if err != nil || updaterID.Executable != install.updater {
			t.Fatalf("actual acquired updater identity: %+v %v", updaterID, err)
		}
		waitNative := nativePinUpdater(t, updaterID)
		lockCtx, stop := context.WithTimeout(ctx, 150*time.Millisecond)
		lease, e := exclusion.Acquire(lockCtx, install.stateDir, exclusion.Launch)
		stop()
		if e == nil {
			lease.Close()
			t.Fatal("updater did not own exclusion")
		}
		reject, err := client.StreamChat(ctx, fmt.Sprintf("rejected-%d", attempt), "must not be admitted", home)
		rejected := err != nil && strings.Contains(err.Error(), "Unavailable")
		if err == nil {
			for m := range reject {
				if m.Type == agentclient.TypeError && strings.Contains(fmt.Sprint(m.Err), "Unavailable") {
					rejected = true
				}
			}
		}
		if !rejected {
			t.Fatal("new root was not rejected by sealed agent")
		}
		if attempt == 0 {
			quit(first)
			still, err := nativeprocess.Inspect(owner.Pid())
			if err != nil || !still.SameProcess(updaterID) {
				t.Fatal("updater did not survive voluntary initiating CLI exit")
			}
			select {
			case <-owner.Done():
				t.Fatal("updater exited with CLI")
			default:
			}
			// The provider handler is still blocked across the seal; now permit its
			// admitted continuation to complete, rather than inferring IDLE from time.
			unblock()
			completed := false
			for m := range stream {
				if m.Type == agentclient.TypeError {
					t.Fatal(m.Err)
				}
				if m.Type == agentclient.TypeDone {
					completed = strings.Contains(m.Final, "native fixture response")
				}
			}
			if !completed {
				t.Fatal("admitted blocked chat did not complete across seal")
			}
		}
		process, err := os.FindProcess(owner.Pid())
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			err = process.Signal(os.Interrupt)
		} else {
			err = process.Kill()
		}
		if err != nil {
			t.Fatal(err)
		}
		completion, err := owner.WaitForCompletion(ctx)
		if err != nil || !completion.For(owner) {
			t.Fatalf("owned updater completion: %v", err)
		}
		reaped = true
		waitNative(ctx)
		code, known := completion.ExitCode()
		if !known || code == 0 {
			t.Fatal("incomplete updater reported success")
		}
		if attempt == 0 {
			b, _ := os.ReadFile(logPath)
			if !strings.Contains(string(b), "updater: incomplete;") {
				t.Fatalf("missing incomplete result: %s", b)
			}
		}
		lockCtx, stop = context.WithTimeout(ctx, 3*time.Second)
		lease, err = exclusion.Acquire(lockCtx, install.stateDir, exclusion.Launch)
		stop()
		if err != nil {
			t.Fatal("owner exit leaked barrier", err)
		}
		lease.Close()
		awaitReadiness(token, true)
		// A completed real chat, not Health (which is exempt), proves admission opens.
		resumed, err := client.StreamChat(ctx, fmt.Sprintf("resumed-%d", attempt), "Reply with native fixture response; do not use tools.", home)
		if err != nil {
			t.Fatal(err)
		}
		completed := false
		for m := range resumed {
			if m.Type == agentclient.TypeError {
				t.Fatal(m.Err)
			}
			if m.Type == agentclient.TypeDone {
				completed = true
			}
		}
		if !completed {
			t.Fatal("owner exit leaked admission")
		}
		after, err := os.ReadFile(filepath.Join(install.stateDir, selection.FileName))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("selection changed")
		}
		db, err := os.ReadFile(filepath.Join(install.stateDir, "state.db"))
		if err != nil || string(db) != "not SQL" {
			t.Fatal("coordination touched SQL")
		}
		participants = []participant{{"agent", agent}, {"client", second.identity}}
	}
	quit(second)
	t.Log("actual acquired old updater: cancellation and crash release barrier/admission, exact owned completion, unchanged selection/corrupt SQL; no stop/publication. Workers/runtime owner set, initiating local action and activation remain unconnected")
}
