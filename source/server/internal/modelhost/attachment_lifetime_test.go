package modelhost

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cercano/source/server/internal/localruntime"
)

// The provider barrier puts disconnect cleanup strictly before hold acquisition,
// without depending on scheduling delays or canceling the independent Start RPC.
func TestDisconnectDuringStartReleasesLateHold(t *testing.T) {
	h := newHarness(t)
	st := h.srv.addStream("departing")
	entered, resume := make(chan struct{}), make(chan struct{})
	h.provider.OnStart = func(localruntime.StartRequest) { close(entered); <-resume }
	done := make(chan error, 1)
	go func() {
		_, err := h.srv.start(context.Background(), "departing", localruntime.StartRequest{Runtime: "fake", ModelID: "m1"})
		done <- err
	}()
	<-entered
	if !h.srv.removeStream("departing", st) {
		t.Fatal("client still attached")
	}
	h.srv.releaseClient("departing")
	close(resume)
	<-done
	if held := h.srv.holds.heldBy("departing"); len(held) != 0 {
		t.Errorf("disconnected client retained holds: %v", held)
	}
	if calls := h.provider.StopCalls(); calls != 1 {
		t.Errorf("StopCalls = %d, want 1 for abandoned launch", calls)
	}
}

func TestUnattachedAcquisitionRejected(t *testing.T) {
	h := newHarness(t)
	for _, method := range []string{"Start", "Restart"} {
		err := h.call("unattached", method, localruntime.StartRequest{Runtime: "fake", ModelID: "m1"}, nil)
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("%s: %v", method, err)
		}
	}
	if h.provider.StartCalls() != 0 {
		t.Fatal("unattached call reached provider")
	}
}

func TestReconnectInvalidatesInFlightAcquisition(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprint(restart), func(t *testing.T) {
			h := newHarness(t)
			old := h.srv.addStream("A")
			entered, resume := make(chan struct{}), make(chan struct{})
			h.provider.OnStart = func(localruntime.StartRequest) { close(entered); <-resume }
			done := make(chan error, 1)
			go func() {
				var err error
				if restart {
					_, err = h.srv.restart(context.Background(), "A", localruntime.RestartRequest{Runtime: "fake", ModelID: "m1"})
				} else {
					_, err = h.srv.start(context.Background(), "A", localruntime.StartRequest{Runtime: "fake", ModelID: "m1"})
				}
				done <- err
			}()
			<-entered
			h.srv.removeStream("A", old)
			h.srv.releaseClient("A")
			h.srv.addStream("A")
			close(resume)
			if err := <-done; status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("stale acquisition: %v", err)
			}
			if len(h.srv.holds.heldBy("A")) != 0 || h.provider.StopCalls() != 1 {
				t.Fatal("old generation acquired a hold")
			}
		})
	}
}

type instancesBarrier struct {
	localruntime.Manager
	entered, resume chan struct{}
}

func (m *instancesBarrier) Instances(ctx context.Context) ([]localruntime.InstanceRecord, error) {
	close(m.entered)
	<-m.resume
	return m.Manager.Instances(ctx)
}

func TestDisconnectedReusePreservesOtherHolder(t *testing.T) {
	h := newHarness(t)
	h.start("B", "m1")
	old := h.srv.addStream("A")
	m := &instancesBarrier{Manager: h.mgr, entered: make(chan struct{}), resume: make(chan struct{})}
	h.srv.mgr = m
	done := make(chan error, 1)
	go func() {
		_, err := h.srv.start(context.Background(), "A", localruntime.StartRequest{Runtime: "fake", ModelID: "m1"})
		done <- err
	}()
	<-m.entered
	h.srv.removeStream("A", old)
	h.srv.releaseClient("A")
	h.srv.addStream("A")
	close(m.resume)
	if err := <-done; status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("reuse: %v", err)
	}
	if h.provider.StopCalls() != 0 || h.srv.holds.count("fake-m1") != 1 {
		t.Fatal("surviving holder lost model")
	}
	// Restore before stream cleanup invokes Instances again.
	h.srv.mgr = h.mgr
}

// The child intentionally never closes its Remote. Only the owning parent kills
// this exact process, after a stdout handshake confirms successful acquisition.
func TestAbruptClientProcessHelper(t *testing.T) {
	dir := os.Getenv("CERCANO_DEATH_TEST_DIR")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := NewRemote(ctx, remoteOpts(dir, func(context.Context) error { return fmt.Errorf("test host must already exist") }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Start(ctx, localruntime.StartRequest{Runtime: "fake", ModelID: "present"}); err != nil {
		t.Fatal(err)
	}
	fmt.Println("ACQUIRED")
	for {
		time.Sleep(time.Hour)
	}
}

func TestAbruptClientProcessDeath(t *testing.T) {
	for _, second := range []bool{false, true} {
		t.Run(fmt.Sprint(second), func(t *testing.T) {
			dir := t.TempDir()
			host := startTestHost(t, "death-test-token")
			host.publish(t, dir)
			var survivor *Remote
			if second {
				survivor = newTestRemote(t, remoteOpts(dir, nil))
				if _, err := survivor.Start(context.Background(), localruntime.StartRequest{Runtime: "fake", ModelID: "present"}); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestAbruptClientProcessHelper$", "-test.timeout=30s")
			cmd.Env = append(os.Environ(), "CERCANO_DEATH_TEST_DIR="+dir)
			cmd.Stderr = os.Stderr
			pipe, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			ready := make(chan bool, 1)
			go func() {
				scan := bufio.NewScanner(pipe)
				for scan.Scan() {
					if scan.Text() == "ACQUIRED" {
						ready <- true
						return
					}
				}
				ready <- false
			}()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatal("child exited without readiness")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("child readiness timeout")
			}
			want := 1
			if second {
				want = 2
			}
			if host.srv.holds.count("fake-present") != want {
				t.Fatal("readiness did not establish holds")
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			waited = true
			if err == nil {
				t.Fatal("child did not die abruptly")
			}
			if second {
				waitUntil(t, "child hold released", func() bool { return host.srv.holds.count("fake-present") == 1 })
				if host.provider.StopCalls() != 0 {
					t.Fatal("death stopped surviving holder")
				}
				survivor.Close()
			}
			waitUntil(t, "last-holder model stopped", func() bool { return host.provider.StopCalls() == 1 })
			// A new authenticated connection proves the host remained alive.
			probe := newTestRemote(t, remoteOpts(dir, func(context.Context) error { return fmt.Errorf("host unexpectedly exited") }))
			if _, err := probe.Instances(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDisconnectedRestartPreservesOtherHolder(t *testing.T) {
	h := newHarness(t)
	h.provider.UniqueIDs = true
	h.start("B", "m1")
	old := h.srv.addStream("A")
	entered, resume := make(chan struct{}), make(chan struct{})
	h.provider.OnStart = func(localruntime.StartRequest) { close(entered); <-resume }
	done := make(chan error, 1)
	go func() {
		_, err := h.srv.restart(context.Background(), "A", localruntime.RestartRequest{Runtime: "fake", ModelID: "m1"})
		done <- err
	}()
	<-entered
	h.srv.removeStream("A", old)
	h.srv.releaseClient("A")
	close(resume)
	if err := <-done; status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("restart: %v", err)
	}
	held := h.srv.holds.heldBy("B")
	if len(held) != 1 || held[0] != "fake-m1-2" {
		t.Fatalf("survivor holds: %v", held)
	}
	if h.provider.StopCalls() != 1 {
		t.Fatal("abandoned restart stopped survivor's replacement")
	}
}
