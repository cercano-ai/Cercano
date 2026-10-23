//go:build unix

package agentclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Both initial Dial and reconnect use ensureServerLaunched -> autoLaunchServer.
// Exercise that shared spawn boundary twice without opening a listener.
func TestCredentialDisabledAutolaunchAndRespawnInheritance(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	t.Setenv("HOME", dir)
	t.Setenv("TMPDIR", dir)
	t.Setenv("PATH", dir)
	output := filepath.Join(dir, "child-env")
	t.Setenv("CERCANO_FIXTURE_OUTPUT", output)
	script := "#!/bin/sh\nprintf '%s:%s' \"$CERCANO_DISABLE_CREDENTIALS\" \"$CERCANO_AUTOLAUNCHED\" > \"$CERCANO_FIXTURE_OUTPUT\"\n"
	if err := os.WriteFile(filepath.Join(dir, "cercano"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"initial", "reconnect-respawn"} {
		t.Run(phase, func(t *testing.T) {
			os.Remove(output)
			if _, err := autoLaunchServer("unused-no-listener"); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				data, _ := os.ReadFile(output)
				if string(data) == "1:1" {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("child did not inherit credential-disabled setting")
		})
	}
}
