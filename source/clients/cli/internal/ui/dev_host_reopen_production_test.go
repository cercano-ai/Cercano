package ui

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Build source/server/cmd/cercano with go test -c and explicitly provide that
// disposable test binary. The helper runs ordinary production A with synthetic
// backends, then lifecycle-only B; this does not assert B's UI/history hydration.
// The live case records the pre-simplification refusal, not the desired resume
// contract. Replace that expectation when live-owner routing is implemented.
func TestReopenProductionReleasedAndDisconnected(t *testing.T) {
	binary := os.Getenv("CERCANO_RESUME_PROBE_BINARY")
	if binary == "" {
		t.Skip("requires explicit disposable server test binary")
	}
	for _, mode := range []string{"released", "disconnected", "live"} {
		t.Run(mode, func(t *testing.T) {
			manifest := filepath.Join(t.TempDir(), "selection.json")
			cmd := exec.Command(binary, "-test.run=^TestProductionResumeBridge$", "-test.timeout=45s")
			cmd.Env = append(os.Environ(), "CERCANO_RESUME_PROBE_MANIFEST="+manifest, "CERCANO_RESUME_PROBE_MODE="+mode)
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
					t.Errorf("probe: %v\n%s", err, output.String())
				}
			}()
			deadline := time.Now().Add(20 * time.Second)
			for {
				if _, err = os.Stat(manifest); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("probe manifest timeout")
				}
				time.Sleep(10 * time.Millisecond)
			}
			destination, err := ReadDevHostSelection(manifest)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			selected, err := reopenDevHost(ctx, destination, "resume-probe")
			if mode == "released" {
				if err != nil || selected.Epoch == "" {
					t.Fatalf("released production reopen: %+v %v", selected, err)
				}
			} else if mode == "live" {
				if err == nil || !strings.Contains(err.Error(), "conversation remains on its original host") {
					t.Fatalf("expected existing live-owner cross-host refusal: %v", err)
				}
				t.Logf("live owner A, selected host B: %v", err)
			} else {
				if err == nil || !strings.Contains(err.Error(), "authoritative acquisition refused") {
					t.Fatalf("unexpected-disconnect reopen: %v", err)
				}
			}
		})
	}
}
