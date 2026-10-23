package main

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"cercano/source/server/pkg/paths"
)

// The dispatch log creates the profile's logs/ directory (and the profile
// directory when missing) private, like every other profile directory.
func TestDispatchLogCreatesPrivateDirs(t *testing.T) {
	tempHome(t)
	t.Setenv("CERCANO_PROFILE", "work")
	prevOut, prevFlags := log.Writer(), log.Flags()
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })

	setupDispatchLogFile()
	for _, dir := range []string{paths.ProfileDir("work"), filepath.Join(paths.ProfileDir("work"), "logs")} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %v, want 0700", dir, fi.Mode().Perm())
		}
	}
}
