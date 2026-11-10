package ui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDestinationResumeAfterActualProcessExit(t *testing.T) {
	binary := os.Getenv("CERCANO_RESUME_PROBE_BINARY")
	if binary == "" {
		t.Skip("requires explicit disposable server test binary")
	}
	root := t.TempDir()
	manifest := filepath.Join(root, "destination.json")
	cmd := exec.Command(binary, "-test.run=^TestDestinationResumeProcessBridge$", "-test.timeout=40s", "-test.v")
	cmd.Env = []string{"HOME=" + root, "XDG_CONFIG_HOME=" + root, "TMPDIR=/tmp", "PATH=/usr/bin:/bin", "CERCANO_DISABLE_CREDENTIALS=1", "CERCANO_DESTINATION_RESUME_MANIFEST=" + manifest}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		input.Close()
		if err := cmd.Wait(); err != nil {
			t.Errorf("process probe: %v\n%s", err, output.String())
		} else {
			t.Log(output.String())
		}
	}()
	deadline := time.Now().Add(25 * time.Second)
	for {
		if _, err = os.Stat(manifest); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("destination manifest timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	destination, err := ReadDevHostSelection(manifest)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// The source has already exited and the bridge verifies Wait's exact child
	// death under launch exclusion. No live-host interruption consent is needed;
	// the deleted takeover protocol's confirmation is not part of this route.
	first, err := reopenDevHost(ctx, destination, "resume-probe")
	if err != nil {
		t.Fatalf("death-proven recovery: %v", err)
	}
	if first.Identity.Host != destination.Identity.Host || first.Epoch == "" {
		t.Fatal("missing recovered ownership binding")
	}
	// Ordinary reopen and fresh pinned client after response/binding loss must
	// stay on B and reuse the recovered epoch, not rediscover the absent A.
	selected, err := reopenDevHost(ctx, destination, "resume-probe")
	if err != nil {
		t.Fatal(err)
	}
	client, err := selected.Dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if selected.Identity.Host != destination.Identity.Host || selected.Epoch != first.Epoch {
		t.Fatal("stale reconnect binding")
	}
	if _, err = input.Write([]byte("persist next turn\n")); err != nil {
		t.Fatal(err)
	}
}
